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
				TextPlaceholders: []document.TextPlaceholder{
					{
						Position: document.Point{X: 180, Y: 80},
						FontSize: 10,
					},
				},
				Links: []document.LinkAnnotation{
					{
						Bounds: document.Rectangle{Left: 72, Top: 72, Right: 150, Bottom: 84},
						Target: document.LinkTarget{
							Kind: document.LinkTargetExternal,
							URI:  "https://example.test",
						},
					},
				},
				Rulings: []document.Ruling{
					{
						Start: document.Point{X: 72, Y: 100},
						End:   document.Point{X: 150, Y: 100},
						Width: 0.5,
					},
				},
			},
		},
	}

	if err := layout.Validate(); err != nil {
		t.Fatalf("Validate() returned an unexpected error: %v", err)
	}
}

func TestPageValidateRejectsInvalidTextPlaceholder(t *testing.T) {
	t.Parallel()

	page := document.Page{
		Width:  612,
		Height: 792,
		TextPlaceholders: []document.TextPlaceholder{{
			Position: document.Point{X: 10, Y: 20},
		}},
	}
	if err := page.Validate(); err == nil {
		t.Fatal("Validate() returned nil for a placeholder without a font size")
	}
}

func TestPageValidateRejectsInvalidRuling(t *testing.T) {
	t.Parallel()

	tests := []document.Ruling{
		{},
		{
			Start: document.Point{X: 1, Y: 1},
			End:   document.Point{X: 2, Y: 2},
		},
		{
			Start: document.Point{X: 1, Y: 1},
			End:   document.Point{X: 2, Y: 1},
			Width: -1,
		},
	}
	for _, ruling := range tests {
		page := document.Page{
			Width:   612,
			Height:  792,
			Rulings: []document.Ruling{ruling},
		}
		if err := page.Validate(); err == nil {
			t.Fatalf("Validate() returned nil for invalid ruling %+v", ruling)
		}
	}
}

func TestPageValidateRejectsInvalidLink(t *testing.T) {
	t.Parallel()

	page := document.Page{
		Width:  612,
		Height: 792,
		Links: []document.LinkAnnotation{
			{Bounds: document.Rectangle{Left: 1, Top: 1, Right: 2, Bottom: 2}},
		},
	}

	if err := page.Validate(); err == nil {
		t.Fatal("Validate() returned nil for a link without a destination")
	}
}

func TestLayoutValidateRejectsOutOfRangePageLink(t *testing.T) {
	t.Parallel()

	layout := document.Layout{
		Pages: []document.Page{
			{
				Number: 1,
				Width:  612,
				Height: 792,
				Links: []document.LinkAnnotation{
					{
						Bounds: document.Rectangle{Left: 1, Top: 1, Right: 2, Bottom: 2},
						Target: document.LinkTarget{
							Kind: document.LinkTargetPage,
							Page: 2,
						},
					},
				},
			},
		},
	}
	if err := layout.Validate(); err == nil {
		t.Fatal("Validate() returned nil for an out-of-range page target")
	}
}

func TestLinkTargetValidate(t *testing.T) {
	t.Parallel()

	valid := []document.LinkTarget{
		{Kind: document.LinkTargetExternal, URI: "https://example.test"},
		{Kind: document.LinkTargetPage, Page: 2},
		{Kind: document.LinkTargetNamed, Name: "section-two"},
	}
	for _, target := range valid {
		if err := target.Validate(); err != nil {
			t.Fatalf("Validate() returned an unexpected error for %+v: %v", target, err)
		}
	}

	invalid := []document.LinkTarget{
		{},
		{Kind: document.LinkTargetExternal, URI: "relative"},
		{Kind: document.LinkTargetExternal, URI: " https://example.test"},
		{Kind: document.LinkTargetPage},
		{Kind: document.LinkTargetNamed, Name: " "},
		{Kind: document.LinkTargetPage, Page: 1, Name: "mixed"},
	}
	for _, target := range invalid {
		if err := target.Validate(); err == nil {
			t.Fatalf("Validate() returned nil for invalid target %+v", target)
		}
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
