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

func TestBasicAnalyzerPreservesIndentedParagraphBoundaries(t *testing.T) {
	t.Parallel()

	layout := &document.Layout{
		Pages: []document.Page{
			{
				Number: 1,
				Width:  220,
				Height: 220,
				TextRuns: []document.TextRun{
					textRun("First paragraph begins", 20, 10, 190, 20),
					textRun("and continues across lines.", 10, 24, 190, 34),
					textRun("Second paragraph starts", 20, 38, 190, 48),
					textRun("and wraps too.", 10, 52, 105, 62),
				},
			},
		},
	}

	result, err := analyze.NewBasicAnalyzer().Analyze(context.Background(), layout)
	if err != nil {
		t.Fatalf("Analyze() returned an unexpected error: %v", err)
	}

	want := []string{
		"First paragraph begins and continues across lines.",
		"Second paragraph starts and wraps too.",
	}
	if got := paragraphTexts(t, result); !reflect.DeepEqual(got, want) {
		t.Fatalf("paragraphs = %#v, want %#v", got, want)
	}
}

func TestBasicAnalyzerStartsAfterShortSentenceEndingLine(t *testing.T) {
	t.Parallel()

	layout := &document.Layout{
		Pages: []document.Page{
			{
				Number: 1,
				Width:  220,
				Height: 220,
				TextRuns: []document.TextRun{
					textRun("The first paragraph wraps across", 10, 10, 190, 20),
					textRun("a short final line.", 10, 24, 90, 34),
					textRun("The next paragraph starts here", 10, 38, 190, 48),
					textRun("and remains wrapped.", 10, 52, 130, 62),
				},
			},
		},
	}

	result, err := analyze.NewBasicAnalyzer().Analyze(context.Background(), layout)
	if err != nil {
		t.Fatalf("Analyze() returned an unexpected error: %v", err)
	}

	want := []string{
		"The first paragraph wraps across a short final line.",
		"The next paragraph starts here and remains wrapped.",
	}
	if got := paragraphTexts(t, result); !reflect.DeepEqual(got, want) {
		t.Fatalf("paragraphs = %#v, want %#v", got, want)
	}
}

func TestBasicAnalyzerJoinsSoftHyphenatedWraps(t *testing.T) {
	t.Parallel()

	layout := &document.Layout{
		Pages: []document.Page{
			{
				Number: 1,
				Width:  220,
				Height: 220,
				TextRuns: []document.TextRun{
					textRun("Document trans-", 10, 10, 190, 20),
					textRun("mutation keeps words readable.", 10, 24, 190, 34),
				},
			},
		},
	}

	result, err := analyze.NewBasicAnalyzer().Analyze(context.Background(), layout)
	if err != nil {
		t.Fatalf("Analyze() returned an unexpected error: %v", err)
	}

	want := []string{"Document transmutation keeps words readable."}
	if got := paragraphTexts(t, result); !reflect.DeepEqual(got, want) {
		t.Fatalf("paragraphs = %#v, want %#v", got, want)
	}
}

func TestBasicAnalyzerDetectsHeadingFromSizeAndSpacing(t *testing.T) {
	t.Parallel()

	title := textRun("Document title", 10, 10, 180, 30)
	title.Style = document.TextStyle{FontSize: 24, FontWeight: 700}
	layout := &document.Layout{
		Pages: []document.Page{
			{
				Number: 1,
				Width:  220,
				Height: 220,
				TextRuns: []document.TextRun{
					textRun("continues here.", 10, 59, 100, 69),
					title,
					textRun("The body paragraph", 10, 45, 180, 55),
				},
			},
		},
	}

	result, err := analyze.NewBasicAnalyzer().Analyze(context.Background(), layout)
	if err != nil {
		t.Fatalf("Analyze() returned an unexpected error: %v", err)
	}
	if got, want := len(result.Blocks), 2; got != want {
		t.Fatalf("block count = %d, want %d", got, want)
	}
	heading, ok := result.Blocks[0].(*document.Heading)
	if !ok {
		t.Fatalf("block 1 has type %T, want *document.Heading", result.Blocks[0])
	}
	if heading.Level != 1 || heading.Text != "Document title" {
		t.Fatalf("heading = %+v, want level 1 document title", heading)
	}
	paragraph, ok := result.Blocks[1].(*document.Paragraph)
	if !ok {
		t.Fatalf("block 2 has type %T, want *document.Paragraph", result.Blocks[1])
	}
	if got, want := paragraph.Text, "The body paragraph continues here."; got != want {
		t.Fatalf("paragraph = %q, want %q", got, want)
	}
}

func TestBasicAnalyzerDetectsSeparatedBoldHeading(t *testing.T) {
	t.Parallel()

	heading := textRun("Section", 10, 52, 80, 62)
	heading.Style = document.TextStyle{FontSize: 10, FontWeight: 700}
	layout := &document.Layout{
		Pages: []document.Page{
			{
				Number: 1,
				Width:  220,
				Height: 220,
				TextRuns: []document.TextRun{
					textRun("First paragraph", 10, 10, 180, 20),
					textRun("continues.", 10, 24, 80, 34),
					heading,
					textRun("Second paragraph", 10, 80, 180, 90),
					textRun("continues.", 10, 94, 80, 104),
				},
			},
		},
	}

	result, err := analyze.NewBasicAnalyzer().Analyze(context.Background(), layout)
	if err != nil {
		t.Fatalf("Analyze() returned an unexpected error: %v", err)
	}
	if got, want := len(result.Blocks), 3; got != want {
		t.Fatalf("block count = %d, want %d", got, want)
	}
	got, ok := result.Blocks[1].(*document.Heading)
	if !ok {
		t.Fatalf("block 2 has type %T, want *document.Heading", result.Blocks[1])
	}
	if got.Level != 3 || got.Text != "Section" {
		t.Fatalf("heading = %+v, want level 3 section", got)
	}
}

func TestBasicAnalyzerDoesNotPromoteOnlyLineToHeading(t *testing.T) {
	t.Parallel()

	line := textRun("Standalone text", 10, 10, 180, 30)
	line.Style = document.TextStyle{FontSize: 24, FontWeight: 700}
	layout := &document.Layout{
		Pages: []document.Page{
			{
				Number:   1,
				Width:    220,
				Height:   220,
				TextRuns: []document.TextRun{line},
			},
		},
	}

	result, err := analyze.NewBasicAnalyzer().Analyze(context.Background(), layout)
	if err != nil {
		t.Fatalf("Analyze() returned an unexpected error: %v", err)
	}
	if got := paragraphTexts(t, result); !reflect.DeepEqual(got, []string{"Standalone text"}) {
		t.Fatalf("paragraphs = %#v, want standalone paragraph", got)
	}
}

func TestBasicAnalyzerRequiresSpacingForModestSizeIncrease(t *testing.T) {
	t.Parallel()

	emphasized := textRun("emphasized text", 10, 24, 140, 36)
	emphasized.Style = document.TextStyle{FontSize: 12, FontWeight: 400}
	layout := &document.Layout{
		Pages: []document.Page{
			{
				Number: 1,
				Width:  220,
				Height: 220,
				TextRuns: []document.TextRun{
					textRun("Paragraph begins with", 10, 10, 180, 20),
					emphasized,
					textRun("and continues normally.", 10, 40, 180, 50),
				},
			},
		},
	}

	result, err := analyze.NewBasicAnalyzer().Analyze(context.Background(), layout)
	if err != nil {
		t.Fatalf("Analyze() returned an unexpected error: %v", err)
	}
	if got := paragraphTexts(t, result); !reflect.DeepEqual(
		got,
		[]string{"Paragraph begins with emphasized text and continues normally."},
	) {
		t.Fatalf("paragraphs = %#v, want one paragraph", got)
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
