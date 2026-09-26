package render_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/document"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/render"
)

type rendererStub struct {
	content string
	err     error
}

func (s rendererStub) Render(_ context.Context, _ *document.Document, output io.Writer) error {
	if _, err := io.WriteString(output, s.content); err != nil {
		return err
	}
	return s.err
}

var _ render.Renderer = rendererStub{}

func TestRendererContractUsesWriterBoundary(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	var renderer render.Renderer = rendererStub{content: "rendered"}

	err := renderer.Render(context.Background(), &document.Document{}, &output)
	if err != nil {
		t.Fatalf("Render() returned an unexpected error: %v", err)
	}
	if got, want := output.String(), "rendered"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestRendererContractAllowsPartialWritesOnError(t *testing.T) {
	t.Parallel()

	want := errors.New("render failed")
	var output bytes.Buffer
	var renderer render.Renderer = rendererStub{content: "partial", err: want}

	got := renderer.Render(context.Background(), &document.Document{}, &output)
	if !errors.Is(got, want) {
		t.Fatalf("Render() error = %v, want %v", got, want)
	}
	if got, want := output.String(), "partial"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
