package app_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/analyze"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/app"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/document"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/extract/pdf/pdfium"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/render/markdown"
)

func TestGeneratedPDFsMatchMarkdownGoldens(t *testing.T) {
	root := repositoryRoot(t)
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

	tests := []struct {
		name        string
		pdf         string
		golden      string
		diagnostics []document.Diagnostic
	}{
		{
			name:   "simple",
			pdf:    "simple.pdf",
			golden: "simple.md.golden",
		},
		{
			name:   "phase2",
			pdf:    "phase2.pdf",
			golden: "phase2.md.golden",
			diagnostics: []document.Diagnostic{
				{
					Code:    document.DiagnosticTableLikeText,
					Page:    3,
					Message: "table-like layout was preserved as plain text",
				},
				{
					Code:    document.DiagnosticCodeLikeText,
					Page:    3,
					Message: "code-like layout was preserved as plain text",
				},
			},
		},
		{
			name:   "contents",
			pdf:    "contents.pdf",
			golden: "contents.md.golden",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sourceData, err := os.ReadFile(
				filepath.Join(root, "testdata", "pdf", "generated", test.pdf),
			)
			if err != nil {
				t.Fatalf("read generated PDF fixture: %v", err)
			}
			want, err := os.ReadFile(
				filepath.Join(root, "testdata", "pdf", "generated", test.golden),
			)
			if err != nil {
				t.Fatalf("read Markdown golden: %v", err)
			}

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

			result, err := converter.ConvertWithResult(
				ctx,
				bytes.NewReader(sourceData),
				destination,
			)
			if err != nil {
				t.Fatalf("ConvertWithResult() returned an unexpected error: %v", err)
			}
			if !bytes.Equal(got.Bytes(), want) {
				t.Fatalf("Markdown output = %q, want %q", got.Bytes(), want)
			}
			if !reflect.DeepEqual(result.Diagnostics, test.diagnostics) {
				t.Fatalf(
					"diagnostics = %#v, want %#v",
					result.Diagnostics,
					test.diagnostics,
				)
			}
		})
	}
}

func repositoryRoot(t testing.TB) string {
	t.Helper()

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve golden test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
}
