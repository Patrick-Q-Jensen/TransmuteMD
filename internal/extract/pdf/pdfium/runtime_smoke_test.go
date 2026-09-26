package pdfium

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
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

	var pageCount int
	err = runtime.withDocument(
		ctx,
		bytes.NewReader(data),
		func(worker instance, document references.FPDF_DOCUMENT) error {
			response, err := worker.FPDF_GetPageCount(&requests.FPDF_GetPageCount{
				Document: document,
			})
			if err != nil {
				return err
			}
			pageCount = response.PageCount
			return nil
		},
	)
	if err != nil {
		t.Fatalf("withDocument() returned an unexpected error: %v", err)
	}
	if got, want := pageCount, 1; got != want {
		t.Fatalf("page count = %d, want %d", got, want)
	}
}
