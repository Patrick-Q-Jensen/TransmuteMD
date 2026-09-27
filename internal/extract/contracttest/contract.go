// Package contracttest defines backend-independent extractor conformance tests.
package contracttest

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/extract"
)

// Harness supplies one extractor and representative sources to the shared
// contract suite.
type Harness struct {
	Extractor        extract.Extractor
	ValidSource      extract.Source
	InvalidSource    extract.Source
	ExpectedName     string
	ExpectedPages    int
	ExpectedText     string
	AssertSourceOpen func(t *testing.T)
}

// Run exercises the engine-neutral extractor contract.
func Run(t *testing.T, harness Harness) {
	t.Helper()

	t.Run("stable name", func(t *testing.T) {
		if harness.Extractor == nil {
			t.Fatal("extractor must not be nil")
		}
		if got := harness.Extractor.Name(); got == "" || got != harness.ExpectedName {
			t.Fatalf("Name() = %q, want %q", got, harness.ExpectedName)
		}
	})

	t.Run("valid neutral layout", func(t *testing.T) {
		layout, err := harness.Extractor.Extract(
			context.Background(),
			harness.ValidSource,
		)
		if err != nil {
			t.Fatalf("Extract() returned an unexpected error: %v", err)
		}
		if layout == nil {
			t.Fatal("Extract() returned a nil layout")
		}
		if err := layout.Validate(); err != nil {
			t.Fatalf("extracted layout is invalid: %v", err)
		}
		if got := len(layout.Pages); got != harness.ExpectedPages {
			t.Fatalf("page count = %d, want %d", got, harness.ExpectedPages)
		}

		var text strings.Builder
		for _, page := range layout.Pages {
			for _, run := range page.TextRuns {
				text.WriteString(run.Text)
			}
		}
		if !strings.Contains(text.String(), harness.ExpectedText) {
			t.Fatalf(
				"extracted text = %q, want it to contain %q",
				text.String(),
				harness.ExpectedText,
			)
		}
		if harness.AssertSourceOpen != nil {
			harness.AssertSourceOpen(t)
		}
	})

	t.Run("invalid document category", func(t *testing.T) {
		_, err := harness.Extractor.Extract(
			context.Background(),
			harness.InvalidSource,
		)
		if !errors.Is(err, extract.ErrInvalidDocument) {
			t.Fatalf("Extract() error = %v, want %v", err, extract.ErrInvalidDocument)
		}
	})

	t.Run("pre-cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := harness.Extractor.Extract(ctx, harness.ValidSource)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Extract() error = %v, want %v", err, context.Canceled)
		}
	})
}
