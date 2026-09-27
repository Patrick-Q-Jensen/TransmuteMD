package pdfium

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/extract/contracttest"
)

func TestExtractorContract(t *testing.T) {
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
	source := newContractSource(data)
	contracttest.Run(t, contracttest.Harness{
		Extractor:     extractor,
		ValidSource:   source,
		InvalidSource: bytes.NewReader([]byte("not a PDF")),
		ExpectedName:  "pdfium-wasm",
		ExpectedPages: 1,
		ExpectedText:  "TransmuteMD PDFium smoke test",
		AssertSourceOpen: func(t *testing.T) {
			t.Helper()
			if _, err := source.ReadAt(make([]byte, 1), 0); err != nil {
				t.Fatalf("source is not readable after extraction: %v", err)
			}
		},
	})
}

type contractSource struct {
	*bytes.Reader
}

func newContractSource(content []byte) *contractSource {
	return &contractSource{Reader: bytes.NewReader(content)}
}

func (source *contractSource) Size() int64 {
	return source.Reader.Size()
}
