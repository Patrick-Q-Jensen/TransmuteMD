package pdfium

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/analyze"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/document"
)

func TestRuntimeOpensGeneratedPDF(t *testing.T) {
	fixturePath := filepath.Join(
		"..", "..", "..", "..",
		"testdata", "pdf", "generated", "simple.pdf",
	)
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	runtime, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime() returned an unexpected error: %v", err)
	}
	defer func() {
		if err := runtime.Close(); err != nil {
			t.Errorf("Close() returned an unexpected error: %v", err)
		}
	}()

	extractor, err := NewExtractor(runtime)
	if err != nil {
		t.Fatalf("NewExtractor() returned an unexpected error: %v", err)
	}

	layout, err := extractor.Extract(ctx, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Extract() returned an unexpected error: %v", err)
	}
	if got, want := len(layout.Pages), 1; got != want {
		t.Fatalf("page count = %d, want %d", got, want)
	}
	page := layout.Pages[0]
	if page.Width != 612 || page.Height != 792 {
		t.Fatalf("page size = %vx%v, want 612x792", page.Width, page.Height)
	}

	var text strings.Builder
	for _, run := range page.TextRuns {
		text.WriteString(run.Text)
	}
	if got, want := text.String(), "TransmuteMD PDFium smoke test"; !strings.Contains(got, want) {
		t.Fatalf("extracted text = %q, want it to contain %q", got, want)
	}

	analyzed, err := analyze.NewBasicAnalyzer().Analyze(ctx, layout)
	if err != nil {
		t.Fatalf("Analyze() returned an unexpected error: %v", err)
	}
	if got, want := len(analyzed.Blocks), 1; got != want {
		t.Fatalf("analyzed block count = %d, want %d", got, want)
	}
	paragraph, ok := analyzed.Blocks[0].(*document.Paragraph)
	if !ok {
		t.Fatalf("analyzed block has type %T, want *document.Paragraph", analyzed.Blocks[0])
	}
	if got, want := paragraph.Text, "TransmuteMD PDFium smoke test"; got != want {
		t.Fatalf("analyzed paragraph = %q, want %q", got, want)
	}
}
