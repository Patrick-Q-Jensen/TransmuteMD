package document_test

import (
	"math"
	"strings"
	"testing"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/document"
)

func TestLayoutValidate(t *testing.T) {
	t.Parallel()

	layout := document.Layout{
		Pages: []document.Page{
			{
				Number: 1,
				Width:  612,
				Height: 792,
				TextRuns: []document.TextRun{
					{
						Text:   "TransmuteMD",
						Bounds: document.Rectangle{Left: 72, Top: 72, Right: 150, Bottom: 84},
						Style: document.TextStyle{
							FontName:   "Example Sans",
							FontSize:   12,
							FontWeight: 400,
						},
					},
				},
			},
		},
	}

	if err := layout.Validate(); err != nil {
		t.Fatalf("Validate() returned an unexpected error: %v", err)
	}
}

func TestLayoutValidateReportsNestedContext(t *testing.T) {
	t.Parallel()

	layout := document.Layout{
		Pages: []document.Page{
			{
				Number: 1,
				Width:  612,
				Height: 792,
				TextRuns: []document.TextRun{
					{
						Text:   "invalid",
						Bounds: document.Rectangle{Left: 20, Top: 10, Right: 5, Bottom: 20},
					},
				},
			},
		},
	}

	err := layout.Validate()
	if err == nil {
		t.Fatal("Validate() returned nil, want an invalid-bounds error")
	}
	if !strings.Contains(err.Error(), "page 1: text run 1: bounds: right must not be less than left") {
		t.Fatalf("Validate() error = %q, want nested page and text-run context", err)
	}
}

func TestLayoutValidateRejectsNonSequentialPages(t *testing.T) {
	t.Parallel()

	layout := document.Layout{
		Pages: []document.Page{{Number: 2, Width: 612, Height: 792}},
	}

	if err := layout.Validate(); err == nil {
		t.Fatal("Validate() returned nil for a non-sequential page number")
	}
}

func TestPageValidateRejectsInvalidDimensions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		page document.Page
	}{
		{
			name: "zero width",
			page: document.Page{Width: 0, Height: 792},
		},
		{
			name: "infinite height",
			page: document.Page{Width: 612, Height: math.Inf(1)},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.page.Validate(); err == nil {
				t.Fatal("Validate() returned nil for invalid dimensions")
			}
		})
	}
}

func TestRectangleDimensions(t *testing.T) {
	t.Parallel()

	rect := document.Rectangle{Left: 10, Top: 20, Right: 35, Bottom: 50}

	if got, want := rect.Width(), 25.0; got != want {
		t.Errorf("Width() = %v, want %v", got, want)
	}
	if got, want := rect.Height(), 30.0; got != want {
		t.Errorf("Height() = %v, want %v", got, want)
	}
}
