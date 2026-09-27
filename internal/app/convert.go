package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/analyze"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/extract"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/render"
)

var (
	errNilExtractor   = errors.New("extractor must not be nil")
	errNilAnalyzer    = errors.New("analyzer must not be nil")
	errNilRenderer    = errors.New("renderer must not be nil")
	errNilSource      = errors.New("source must not be nil")
	errNilDestination = errors.New("destination must not be nil")
	errNilLayout      = errors.New("extractor returned a nil layout")
	errNilDocument    = errors.New("analyzer returned a nil document")

	// ErrNoExtractableText indicates that analysis found no semantic text.
	ErrNoExtractableText = errors.New(
		"document contains no extractable text; scanned documents require OCR",
	)
)

// Converter coordinates extraction, analysis, rendering, and output commit.
type Converter struct {
	extractor extract.Extractor
	analyzer  analyze.Analyzer
	renderer  render.Renderer
}

// NewConverter creates an engine-neutral conversion pipeline.
func NewConverter(
	extractor extract.Extractor,
	analyzer analyze.Analyzer,
	renderer render.Renderer,
) (*Converter, error) {
	if extractor == nil {
		return nil, errNilExtractor
	}
	if analyzer == nil {
		return nil, errNilAnalyzer
	}
	if renderer == nil {
		return nil, errNilRenderer
	}
	return &Converter{
		extractor: extractor,
		analyzer:  analyzer,
		renderer:  renderer,
	}, nil
}

// Convert runs the conversion pipeline and commits output only after every
// pipeline stage succeeds.
func (converter *Converter) Convert(
	ctx context.Context,
	source extract.Source,
	destination Destination,
) error {
	if converter == nil || converter.extractor == nil {
		return errNilExtractor
	}
	if converter.analyzer == nil {
		return errNilAnalyzer
	}
	if converter.renderer == nil {
		return errNilRenderer
	}
	if source == nil {
		return errNilSource
	}
	if destination == nil {
		return errNilDestination
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("convert document: %w", err)
	}

	layout, err := converter.extractor.Extract(ctx, source)
	if err != nil {
		return fmt.Errorf("extract with %s: %w", converter.extractor.Name(), err)
	}
	if layout == nil {
		return fmt.Errorf("extract with %s: %w", converter.extractor.Name(), errNilLayout)
	}
	if err := layout.Validate(); err != nil {
		return fmt.Errorf("validate extracted layout: %w", err)
	}

	semantic, err := converter.analyzer.Analyze(ctx, layout)
	if err != nil {
		return fmt.Errorf("analyze document: %w", err)
	}
	if semantic == nil {
		return errNilDocument
	}
	if err := semantic.Validate(); err != nil {
		return fmt.Errorf("validate analyzed document: %w", err)
	}
	if len(semantic.Blocks) == 0 {
		return ErrNoExtractableText
	}

	var rendered bytes.Buffer
	if err := converter.renderer.Render(ctx, semantic, &rendered); err != nil {
		return fmt.Errorf("render document: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("commit output: %w", err)
	}
	if err := destination.Commit(ctx, rendered.Bytes()); err != nil {
		return fmt.Errorf("commit output: %w", err)
	}
	return nil
}
