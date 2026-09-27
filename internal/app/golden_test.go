package app_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/analyze"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/app"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/extract/pdf/pdfium"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/render/markdown"
)

func TestGeneratedPDFMatchesMarkdownGolden(t *testing.T) {
	root := repositoryRoot(t)
	sourceData, err := os.ReadFile(
		filepath.Join(root, "testdata", "pdf", "generated", "simple.pdf"),
	)
	if err != nil {
		t.Fatalf("read generated PDF fixture: %v", err)
	}
	want, err := os.ReadFile(
		filepath.Join(root, "testdata", "pdf", "generated", "simple.md.golden"),
	)
	if err != nil {
		t.Fatalf("read Markdown golden: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pdfRuntime, err := pdfium.NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime() returned an unexpected error: %v", err)
	}
	defer func() {
		if err := pdfRuntime.Close(); err != nil {
			t.Errorf("close PDFium runtime: %v", err)
		}
	}()

	extractor, err := pdfium.NewExtractor(pdfRuntime)
	if err != nil {
		t.Fatalf("NewExtractor() returned an unexpected error: %v", err)
	}
	converter, err := app.NewConverter(
		extractor,
		analyze.NewBasicAnalyzer(),
		markdown.NewRenderer(),
	)
	if err != nil {
		t.Fatalf("NewConverter() returned an unexpected error: %v", err)
	}
	var got bytes.Buffer
	destination, err := app.NewWriterDestination(&got)
	if err != nil {
		t.Fatalf("NewWriterDestination() returned an unexpected error: %v", err)
	}

	if err := converter.Convert(ctx, bytes.NewReader(sourceData), destination); err != nil {
		t.Fatalf("Convert() returned an unexpected error: %v", err)
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatalf("Markdown output = %q, want %q", got.Bytes(), want)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve golden test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
}
