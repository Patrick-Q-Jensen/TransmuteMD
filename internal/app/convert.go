package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/analyze"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/document"
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

	// ErrOutput indicates that fully rendered content could not be committed
	// to its destination.
	ErrOutput = errors.New("output operation failed")
)

// Converter coordinates extraction, analysis, rendering, and output commit.
type Converter struct {
	extractor extract.Extractor
	analyzer  analyze.Analyzer
	renderer  render.Renderer
}

// Result describes a successfully committed conversion.
type Result struct {
	Diagnostics []document.Diagnostic
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
	_, err := converter.ConvertWithResult(ctx, source, destination)
	return err
}

// ConvertWithResult runs the conversion pipeline and returns non-fatal
// diagnostics after output has committed successfully.
func (converter *Converter) ConvertWithResult(
	ctx context.Context,
	source extract.Source,
	destination Destination,
) (Result, error) {
	if converter == nil || converter.extractor == nil {
		return Result{}, errNilExtractor
	}
	if converter.analyzer == nil {
		return Result{}, errNilAnalyzer
	}
	if converter.renderer == nil {
		return Result{}, errNilRenderer
	}
	if source == nil {
		return Result{}, errNilSource
	}
	if destination == nil {
		return Result{}, errNilDestination
	}
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("convert document: %w", err)
	}

	layout, err := converter.extractor.Extract(ctx, source)
	if err != nil {
		return Result{}, fmt.Errorf("extract with %s: %w", converter.extractor.Name(), err)
	}
	if layout == nil {
		return Result{}, fmt.Errorf("extract with %s: %w", converter.extractor.Name(), errNilLayout)
	}
	if err := layout.Validate(); err != nil {
		return Result{}, fmt.Errorf("validate extracted layout: %w", err)
	}

	semantic, err := converter.analyzer.Analyze(ctx, layout)
	if err != nil {
		return Result{}, fmt.Errorf("analyze document: %w", err)
	}
	if semantic == nil {
		return Result{}, errNilDocument
	}
	if err := semantic.Validate(); err != nil {
		return Result{}, fmt.Errorf("validate analyzed document: %w", err)
	}
	if len(semantic.Blocks) == 0 {
		return Result{}, ErrNoExtractableText
	}

	var rendered bytes.Buffer
	if err := converter.renderer.Render(ctx, semantic, &rendered); err != nil {
		return Result{}, fmt.Errorf("render document: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("commit output: %w", err)
	}
	if err := destination.Commit(ctx, rendered.Bytes()); err != nil {
		return Result{}, fmt.Errorf("commit output: %w: %w", ErrOutput, err)
	}
	return Result{
		Diagnostics: append([]document.Diagnostic(nil), semantic.Diagnostics...),
	}, nil
}
