package analyze_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/analyze"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/document"
)

func TestBasicAnalyzerOrdersTextAndGroupsParagraphs(t *testing.T) {
	t.Parallel()

	layout := &document.Layout{
		Pages: []document.Page{
			{
				Number: 1,
				Width:  200,
				Height: 200,
				TextRuns: []document.TextRun{
					textRun("world", 46, 10, 76, 20),
					textRun("continues.", 10, 24, 65, 34),
					textRun(",", 40, 10, 43, 20),
					textRun("paragraph.", 48, 60, 100, 70),
					textRun("Hello", 10, 10, 40, 20),
					textRun("Second", 10, 60, 43, 70),
				},
			},
			{
				Number: 2,
				Width:  200,
				Height: 200,
				TextRuns: []document.TextRun{
					textRun("page.", 42, 10, 70, 20),
					textRun("Next", 10, 10, 37, 20),
				},
			},
		},
	}
	before := cloneLayout(*layout)

	result, err := analyze.NewBasicAnalyzer().Analyze(context.Background(), layout)
	if err != nil {
		t.Fatalf("Analyze() returned an unexpected error: %v", err)
	}

	got := paragraphTexts(t, result)
	want := []string{
		"Hello, world continues.",
		"Second paragraph.",
		"Next page.",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("paragraphs = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(*layout, before) {
		t.Fatal("Analyze() mutated its input layout")
	}
}

func TestBasicAnalyzerUsesVisibleWhitespaceAndIgnoresControlWhitespace(t *testing.T) {
	t.Parallel()

	layout := &document.Layout{
		Pages: []document.Page{
			{
				Number: 1,
				Width:  200,
				Height: 200,
				TextRuns: []document.TextRun{
					textRun("joined", 10, 10, 40, 20),
					textRun(" ", 40, 10, 43, 20),
					textRun("text", 43, 10, 65, 20),
					textRun(" with ", 65, 10, 90, 20),
					textRun("spaces", 90, 10, 120, 20),
					textRun("\r\n", 0, 0, 0, 0),
				},
			},
		},
	}

	result, err := analyze.NewBasicAnalyzer().Analyze(context.Background(), layout)
	if err != nil {
		t.Fatalf("Analyze() returned an unexpected error: %v", err)
	}

	if got, want := paragraphTexts(t, result), []string{"joined text with spaces"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("paragraphs = %#v, want %#v", got, want)
	}
}

func TestBasicAnalyzerReturnsEmptyDocumentForWhitespaceOnlyLayout(t *testing.T) {
	t.Parallel()

	layout := &document.Layout{
		Pages: []document.Page{
			{
				Number: 1,
				Width:  200,
				Height: 200,
				TextRuns: []document.TextRun{
					textRun("\n", 0, 0, 0, 0),
				},
			},
		},
	}

	result, err := analyze.NewBasicAnalyzer().Analyze(context.Background(), layout)
	if err != nil {
		t.Fatalf("Analyze() returned an unexpected error: %v", err)
	}
	if len(result.Blocks) != 0 {
		t.Fatalf("block count = %d, want 0", len(result.Blocks))
	}
}

func TestBasicAnalyzerRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	analyzer := analyze.NewBasicAnalyzer()
	if _, err := analyzer.Analyze(context.Background(), nil); err == nil {
		t.Fatal("Analyze() returned nil error for a nil layout")
	}

	invalid := &document.Layout{
		Pages: []document.Page{{Number: 1, Width: 0, Height: 200}},
	}
	_, err := analyzer.Analyze(context.Background(), invalid)
	if err == nil {
		t.Fatal("Analyze() returned nil error for an invalid layout")
	}
	if !strings.Contains(err.Error(), "validate layout for analysis: page 1") {
		t.Fatalf("Analyze() error = %q, want validation context", err)
	}
}

func TestBasicAnalyzerHonorsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := analyze.NewBasicAnalyzer().Analyze(ctx, &document.Layout{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Analyze() error = %v, want %v", err, context.Canceled)
	}
}

func textRun(text string, left, top, right, bottom float64) document.TextRun {
	return document.TextRun{
		Text:   text,
		Bounds: document.Rectangle{Left: left, Top: top, Right: right, Bottom: bottom},
		Style:  document.TextStyle{FontSize: 10},
	}
}

func paragraphTexts(t *testing.T, doc *document.Document) []string {
	t.Helper()

	texts := make([]string, 0, len(doc.Blocks))
	for index, block := range doc.Blocks {
		paragraph, ok := block.(*document.Paragraph)
		if !ok {
			t.Fatalf("block %d has type %T, want *document.Paragraph", index+1, block)
		}
		texts = append(texts, paragraph.Text)
	}
	return texts
}

func cloneLayout(layout document.Layout) document.Layout {
	clone := layout
	clone.Pages = append([]document.Page(nil), layout.Pages...)
	for index := range clone.Pages {
		clone.Pages[index].TextRuns = append(
			[]document.TextRun(nil),
			layout.Pages[index].TextRuns...,
		)
	}
	return clone
}
