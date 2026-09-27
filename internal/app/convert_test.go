package app_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/app"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/document"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/extract"
)

func TestConverterCommitsCompletedPipelineOutput(t *testing.T) {
	t.Parallel()

	var events []string
	layout := validLayout()
	semantic := validDocument()
	converter, err := app.NewConverter(
		extractorStub{
			events: &events,
			layout: layout,
		},
		analyzerStub{
			events:   &events,
			document: semantic,
		},
		rendererStub{
			events:  &events,
			content: "converted\n",
		},
	)
	if err != nil {
		t.Fatalf("NewConverter() returned an unexpected error: %v", err)
	}
	destination := &destinationStub{events: &events}

	err = converter.Convert(
		context.Background(),
		bytes.NewReader([]byte("source")),
		destination,
	)
	if err != nil {
		t.Fatalf("Convert() returned an unexpected error: %v", err)
	}
	if got, want := string(destination.content), "converted\n"; got != want {
		t.Fatalf("committed content = %q, want %q", got, want)
	}
	if got, want := events, []string{"extract", "analyze", "render", "commit"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %#v, want %#v", got, want)
	}
}

func TestConverterReturnsDiagnosticsAfterCommit(t *testing.T) {
	t.Parallel()

	semantic := validDocument()
	semantic.Diagnostics = []document.Diagnostic{
		{
			Code:    document.DiagnosticCodeLikeText,
			Page:    1,
			Message: "code-like layout was preserved as plain text",
		},
	}
	converter, err := app.NewConverter(
		extractorStub{layout: validLayout()},
		analyzerStub{document: semantic},
		rendererStub{content: "content"},
	)
	if err != nil {
		t.Fatalf("NewConverter() returned an unexpected error: %v", err)
	}

	result, err := converter.ConvertWithResult(
		context.Background(),
		bytes.NewReader([]byte("source")),
		&destinationStub{},
	)
	if err != nil {
		t.Fatalf("ConvertWithResult() returned an unexpected error: %v", err)
	}
	if !reflect.DeepEqual(result.Diagnostics, semantic.Diagnostics) {
		t.Fatalf("diagnostics = %#v, want %#v", result.Diagnostics, semantic.Diagnostics)
	}
}

func TestConverterDoesNotExposePartialRendererOutput(t *testing.T) {
	t.Parallel()

	renderErr := errors.New("render failed")
	converter, err := app.NewConverter(
		extractorStub{layout: validLayout()},
		analyzerStub{document: validDocument()},
		rendererStub{content: "partial", err: renderErr},
	)
	if err != nil {
		t.Fatalf("NewConverter() returned an unexpected error: %v", err)
	}
	var output bytes.Buffer
	destination, err := app.NewWriterDestination(&output)
	if err != nil {
		t.Fatalf("NewWriterDestination() returned an unexpected error: %v", err)
	}

	err = converter.Convert(
		context.Background(),
		bytes.NewReader([]byte("source")),
		destination,
	)
	if !errors.Is(err, renderErr) {
		t.Fatalf("Convert() error = %v, want %v", err, renderErr)
	}
	if output.Len() != 0 {
		t.Fatalf("user-visible output = %q, want no output", output.String())
	}
}

func TestConverterPreservesStageErrorsWithoutCommitting(t *testing.T) {
	t.Parallel()

	extractErr := errors.New("extract failed")
	analyzeErr := errors.New("analyze failed")
	tests := []struct {
		name      string
		extractor extractorStub
		analyzer  analyzerStub
	}{
		{
			name:      "extraction",
			extractor: extractorStub{err: extractErr},
			analyzer:  analyzerStub{document: validDocument()},
		},
		{
			name:      "analysis",
			extractor: extractorStub{layout: validLayout()},
			analyzer:  analyzerStub{err: analyzeErr},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			converter, err := app.NewConverter(
				test.extractor,
				test.analyzer,
				rendererStub{content: "content"},
			)
			if err != nil {
				t.Fatalf("NewConverter() returned an unexpected error: %v", err)
			}
			destination := &destinationStub{}

			err = converter.Convert(
				context.Background(),
				bytes.NewReader([]byte("source")),
				destination,
			)
			if err == nil {
				t.Fatal("Convert() returned nil error for a failed stage")
			}
			if destination.commits != 0 {
				t.Fatalf("destination commit count = %d, want 0", destination.commits)
			}
		})
	}
}

func TestConverterPreservesDestinationError(t *testing.T) {
	t.Parallel()

	want := errors.New("commit failed")
	converter, err := app.NewConverter(
		extractorStub{layout: validLayout()},
		analyzerStub{document: validDocument()},
		rendererStub{content: "content"},
	)
	if err != nil {
		t.Fatalf("NewConverter() returned an unexpected error: %v", err)
	}

	err = converter.Convert(
		context.Background(),
		bytes.NewReader([]byte("source")),
		&destinationStub{err: want},
	)
	if !errors.Is(err, want) {
		t.Fatalf("Convert() error = %v, want %v", err, want)
	}
}

func TestConverterRejectsTextlessDocumentWithoutCommitting(t *testing.T) {
	t.Parallel()

	converter, err := app.NewConverter(
		extractorStub{layout: validLayout()},
		analyzerStub{document: &document.Document{}},
		rendererStub{content: "must not render"},
	)
	if err != nil {
		t.Fatalf("NewConverter() returned an unexpected error: %v", err)
	}
	destination := &destinationStub{}

	err = converter.Convert(
		context.Background(),
		bytes.NewReader([]byte("source")),
		destination,
	)
	if !errors.Is(err, app.ErrNoExtractableText) {
		t.Fatalf("Convert() error = %v, want %v", err, app.ErrNoExtractableText)
	}
	if destination.commits != 0 {
		t.Fatalf("destination commit count = %d, want 0", destination.commits)
	}
}

func TestNewConverterRejectsNilStages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		extractor extract.Extractor
		analyzer  analyzerStub
		renderer  rendererStub
	}{
		{name: "extractor"},
		{name: "analyzer", extractor: extractorStub{}},
		{name: "renderer", extractor: extractorStub{}, analyzer: analyzerStub{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var analyzerValue interface {
				Analyze(context.Context, *document.Layout) (*document.Document, error)
			}
			if test.name != "analyzer" {
				analyzerValue = test.analyzer
			}
			var rendererValue interface {
				Render(context.Context, *document.Document, io.Writer) error
			}
			if test.name != "renderer" {
				rendererValue = test.renderer
			}

			if _, err := app.NewConverter(test.extractor, analyzerValue, rendererValue); err == nil {
				t.Fatal("NewConverter() returned nil error for a nil stage")
			}
		})
	}
}

func validLayout() *document.Layout {
	return &document.Layout{
		Pages: []document.Page{{Number: 1, Width: 100, Height: 100}},
	}
}

func validDocument() *document.Document {
	return &document.Document{
		Blocks: []document.Block{
			&document.Paragraph{Text: "content"},
		},
	}
}

type extractorStub struct {
	events *[]string
	layout *document.Layout
	err    error
}

func (stub extractorStub) Name() string {
	return "stub"
}

func (stub extractorStub) Extract(
	context.Context,
	extract.Source,
) (*document.Layout, error) {
	addEvent(stub.events, "extract")
	return stub.layout, stub.err
}

type analyzerStub struct {
	events   *[]string
	document *document.Document
	err      error
}

func (stub analyzerStub) Analyze(
	context.Context,
	*document.Layout,
) (*document.Document, error) {
	addEvent(stub.events, "analyze")
	return stub.document, stub.err
}

type rendererStub struct {
	events  *[]string
	content string
	err     error
}

func (stub rendererStub) Render(
	_ context.Context,
	_ *document.Document,
	output io.Writer,
) error {
	addEvent(stub.events, "render")
	if _, err := io.WriteString(output, stub.content); err != nil {
		return err
	}
	return stub.err
}

type destinationStub struct {
	events  *[]string
	content []byte
	commits int
	err     error
}

func (stub *destinationStub) Commit(_ context.Context, content []byte) error {
	addEvent(stub.events, "commit")
	stub.commits++
	stub.content = append([]byte(nil), content...)
	return stub.err
}

func addEvent(events *[]string, event string) {
	if events != nil {
		*events = append(*events, event)
	}
}
