package analyze_test

import (
	"context"
	"errors"
	"fmt"
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

func TestBasicAnalyzerPreservesConsistentlyTrackedWords(t *testing.T) {
	t.Parallel()

	glyph := func(text string, left, right float64) document.TextRun {
		return document.TextRun{
			Text:   text,
			Bounds: document.Rectangle{Left: left, Top: 10, Right: right, Bottom: 16},
			Style:  document.TextStyle{FontName: "Tracked Sans", FontSize: 6},
		}
	}
	layout := &document.Layout{
		Pages: []document.Page{
			{
				Number: 1,
				Width:  100,
				Height: 100,
				TextRuns: []document.TextRun{
					glyph("B", 10, 13),
					glyph("M", 14.3, 18),
					glyph("I", 19.3, 20),
					glyph("T", 24, 27),
					glyph("E", 28.3, 31),
					glyph("S", 32.3, 35),
					glyph("T", 36.3, 39),
				},
			},
		},
	}

	result, err := analyze.NewBasicAnalyzer().Analyze(context.Background(), layout)
	if err != nil {
		t.Fatalf("Analyze() returned an unexpected error: %v", err)
	}

	want := []string{"BMI TEST"}
	if got := paragraphTexts(t, result); !reflect.DeepEqual(got, want) {
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

func TestBasicAnalyzerJoinsPDFDiscretionaryBreakWraps(t *testing.T) {
	t.Parallel()

	layout := &document.Layout{
		Pages: []document.Page{
			{
				Number: 1,
				Width:  220,
				Height: 220,
				TextRuns: []document.TextRun{
					textRun("The imple\u0002", 10, 10, 190, 20),
					textRun("mentation remains readable.", 10, 24, 190, 34),
				},
			},
		},
	}

	result, err := analyze.NewBasicAnalyzer().Analyze(context.Background(), layout)
	if err != nil {
		t.Fatalf("Analyze() returned an unexpected error: %v", err)
	}

	want := []string{"The implementation remains readable."}
	if got := paragraphTexts(t, result); !reflect.DeepEqual(got, want) {
		t.Fatalf("paragraphs = %#v, want %#v", got, want)
	}
}

func TestBasicAnalyzerDoesNotJoinDiscretionaryBreakAcrossTableColumns(
	t *testing.T,
) {
	t.Parallel()

	layout := &document.Layout{
		Pages: []document.Page{
			{
				Number: 1,
				Width:  220,
				Height: 220,
				TextRuns: []document.TextRun{
					textRun("Step", 10, 10, 30, 20),
					textRun("destina\u0002", 100, 10, 150, 20),
					textRun("and check", 10, 24, 60, 34),
				},
			},
		},
	}

	result, err := analyze.NewBasicAnalyzer().Analyze(context.Background(), layout)
	if err != nil {
		t.Fatalf("Analyze() returned an unexpected error: %v", err)
	}

	want := []string{"Step destina- and check"}
	if got := paragraphTexts(t, result); !reflect.DeepEqual(got, want) {
		t.Fatalf("paragraphs = %#v, want %#v", got, want)
	}
}

func TestBasicAnalyzerDoesNotJoinDiscretionaryBreakInAmbiguousColumnGroup(
	t *testing.T,
) {
	t.Parallel()

	layout := &document.Layout{
		Pages: []document.Page{
			{
				Number: 1,
				Width:  220,
				Height: 220,
				TextRuns: []document.TextRun{
					textRun("Install", 10, 10, 45, 20),
					textRun("Expected result", 120, 10, 190, 20),
					textRun("direc\u0002", 10, 24, 50, 34),
					textRun("stallation continues.", 10, 38, 120, 48),
				},
			},
		},
	}

	result, err := analyze.NewBasicAnalyzer().Analyze(context.Background(), layout)
	if err != nil {
		t.Fatalf("Analyze() returned an unexpected error: %v", err)
	}

	want := []string{"Install Expected result direc- stallation continues."}
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

func TestBasicAnalyzerDetectsUnorderedListWithContinuation(t *testing.T) {
	t.Parallel()

	layout := &document.Layout{
		Pages: []document.Page{
			{
				Number: 1,
				Width:  220,
				Height: 220,
				TextRuns: []document.TextRun{
					textRun("Introductory paragraph.", 10, 10, 150, 20),
					textRun("- First item wraps", 10, 38, 140, 48),
					textRun("onto another line.", 24, 52, 130, 62),
					textRun("\u2022 Second item.", 10, 66, 110, 76),
					textRun("Following paragraph.", 10, 94, 150, 104),
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
	first, ok := result.Blocks[0].(*document.Paragraph)
	if !ok || first.Text != "Introductory paragraph." {
		t.Fatalf("block 1 = %#v, want introductory paragraph", result.Blocks[0])
	}
	list, ok := result.Blocks[1].(*document.List)
	if !ok {
		t.Fatalf("block 2 has type %T, want *document.List", result.Blocks[1])
	}
	if list.Kind != document.ListKindUnordered || list.Start != 0 {
		t.Fatalf("list kind/start = %d/%d, want unordered/0", list.Kind, list.Start)
	}
	wantItems := []document.ListItem{
		{Text: "First item wraps onto another line."},
		{Text: "Second item."},
	}
	if !reflect.DeepEqual(list.Items, wantItems) {
		t.Fatalf("list items = %#v, want %#v", list.Items, wantItems)
	}
	last, ok := result.Blocks[2].(*document.Paragraph)
	if !ok || last.Text != "Following paragraph." {
		t.Fatalf("block 3 = %#v, want following paragraph", result.Blocks[2])
	}
}

func TestBasicAnalyzerDetectsOrderedListSequence(t *testing.T) {
	t.Parallel()

	layout := &document.Layout{
		Pages: []document.Page{
			{
				Number: 1,
				Width:  220,
				Height: 220,
				TextRuns: []document.TextRun{
					textRun("3) Third item.", 10, 10, 110, 20),
					textRun("4. Fourth item.", 10, 24, 120, 34),
				},
			},
		},
	}

	result, err := analyze.NewBasicAnalyzer().Analyze(context.Background(), layout)
	if err != nil {
		t.Fatalf("Analyze() returned an unexpected error: %v", err)
	}
	if got, want := len(result.Blocks), 1; got != want {
		t.Fatalf("block count = %d, want %d", got, want)
	}
	list, ok := result.Blocks[0].(*document.List)
	if !ok {
		t.Fatalf("block 1 has type %T, want *document.List", result.Blocks[0])
	}
	if list.Kind != document.ListKindOrdered || list.Start != 3 {
		t.Fatalf("list kind/start = %d/%d, want ordered/3", list.Kind, list.Start)
	}
	wantItems := []document.ListItem{{Text: "Third item."}, {Text: "Fourth item."}}
	if !reflect.DeepEqual(list.Items, wantItems) {
		t.Fatalf("list items = %#v, want %#v", list.Items, wantItems)
	}
}

func TestBasicAnalyzerExcludesListMarkersFromParagraphMargins(t *testing.T) {
	t.Parallel()

	layout := &document.Layout{
		Pages: []document.Page{
			{
				Number: 1,
				Width:  220,
				Height: 220,
				TextRuns: []document.TextRun{
					textRun("- First item.", 10, 10, 100, 20),
					textRun("- Second item.", 10, 24, 110, 34),
					textRun("First paragraph begins here", 20, 52, 190, 62),
					textRun("and ends on a short line.", 20, 66, 90, 76),
					textRun("Second paragraph starts here", 20, 80, 190, 90),
					textRun("and continues.", 20, 94, 110, 104),
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
	if _, ok := result.Blocks[0].(*document.List); !ok {
		t.Fatalf("block 1 has type %T, want *document.List", result.Blocks[0])
	}
	for index, want := range []string{
		"First paragraph begins here and ends on a short line.",
		"Second paragraph starts here and continues.",
	} {
		paragraph, ok := result.Blocks[index+1].(*document.Paragraph)
		if !ok {
			t.Fatalf(
				"block %d has type %T, want *document.Paragraph",
				index+2,
				result.Blocks[index+1],
			)
		}
		if paragraph.Text != want {
			t.Fatalf("block %d text = %q, want %q", index+2, paragraph.Text, want)
		}
	}
}

func TestBasicAnalyzerRequiresMarkerSeparator(t *testing.T) {
	t.Parallel()

	layout := &document.Layout{
		Pages: []document.Page{
			{
				Number: 1,
				Width:  220,
				Height: 220,
				TextRuns: []document.TextRun{
					textRun("1.2 is not an ordered marker", 10, 10, 180, 20),
					textRun("-not an unordered marker", 10, 24, 170, 34),
				},
			},
		},
	}

	result, err := analyze.NewBasicAnalyzer().Analyze(context.Background(), layout)
	if err != nil {
		t.Fatalf("Analyze() returned an unexpected error: %v", err)
	}
	want := []string{"1.2 is not an ordered marker -not an unordered marker"}
	if got := paragraphTexts(t, result); !reflect.DeepEqual(got, want) {
		t.Fatalf("paragraphs = %#v, want %#v", got, want)
	}
}

func TestBasicAnalyzerOrdersTwoColumnsBeforeGroupingParagraphs(t *testing.T) {
	t.Parallel()

	title := textRun("Two-column report", 10, 10, 210, 30)
	title.Style = document.TextStyle{FontSize: 20, FontWeight: 700}
	layout := &document.Layout{
		Pages: []document.Page{
			{
				Number: 1,
				Width:  220,
				Height: 220,
				TextRuns: []document.TextRun{
					textRun("Right starts here", 125, 44, 205, 54),
					textRun("Left starts here", 10, 44, 90, 54),
					textRun("right continues.", 125, 58, 200, 68),
					title,
					textRun("left continues.", 10, 58, 85, 68),
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
	heading, ok := result.Blocks[0].(*document.Heading)
	if !ok || heading.Text != "Two-column report" {
		t.Fatalf("block 1 = %#v, want report heading", result.Blocks[0])
	}
	for index, want := range []string{
		"Left starts here left continues.",
		"Right starts here right continues.",
	} {
		paragraph, ok := result.Blocks[index+1].(*document.Paragraph)
		if !ok {
			t.Fatalf(
				"block %d has type %T, want *document.Paragraph",
				index+2,
				result.Blocks[index+1],
			)
		}
		if paragraph.Text != want {
			t.Fatalf("block %d text = %q, want %q", index+2, paragraph.Text, want)
		}
	}
}

func TestBasicAnalyzerLeavesAmbiguousColumnsInRowOrder(t *testing.T) {
	t.Parallel()

	layout := &document.Layout{
		Pages: []document.Page{
			{
				Number: 1,
				Width:  220,
				Height: 220,
				TextRuns: []document.TextRun{
					textRun("Left", 10, 10, 80, 20),
					textRun("right", 125, 10, 200, 20),
				},
			},
		},
	}

	result, err := analyze.NewBasicAnalyzer().Analyze(context.Background(), layout)
	if err != nil {
		t.Fatalf("Analyze() returned an unexpected error: %v", err)
	}
	want := []string{"Left right"}
	if got := paragraphTexts(t, result); !reflect.DeepEqual(got, want) {
		t.Fatalf("paragraphs = %#v, want %#v", got, want)
	}
}

func TestBasicAnalyzerMapsLinkAnnotationsToSemanticText(t *testing.T) {
	t.Parallel()

	layout := &document.Layout{
		Pages: []document.Page{
			{
				Number: 1,
				Width:  220,
				Height: 220,
				TextRuns: []document.TextRun{
					textRun("Read ", 10, 10, 35, 20),
					textRun("the", 35, 10, 50, 20),
					textRun(" docs", 50, 10, 80, 20),
					textRun(" today.", 80, 10, 120, 20),
				},
				Links: []document.LinkAnnotation{
					{
						Bounds:      document.Rectangle{Left: 35, Top: 8, Right: 80, Bottom: 22},
						Destination: "https://example.test/docs",
					},
				},
			},
		},
	}

	result, err := analyze.NewBasicAnalyzer().Analyze(context.Background(), layout)
	if err != nil {
		t.Fatalf("Analyze() returned an unexpected error: %v", err)
	}
	paragraph, ok := result.Blocks[0].(*document.Paragraph)
	if !ok {
		t.Fatalf("block 1 has type %T, want *document.Paragraph", result.Blocks[0])
	}
	if got, want := paragraph.Text, "Read the docs today."; got != want {
		t.Fatalf("paragraph text = %q, want %q", got, want)
	}
	wantLinks := []document.TextLink{
		{Start: 5, End: 13, Destination: "https://example.test/docs"},
	}
	if !reflect.DeepEqual(paragraph.Links, wantLinks) {
		t.Fatalf("paragraph links = %#v, want %#v", paragraph.Links, wantLinks)
	}
}

func TestBasicAnalyzerReportsStructureFallbacks(t *testing.T) {
	t.Parallel()

	codeLine1 := textRun("func main() {", 10, 100, 100, 110)
	codeLine1.Style.FontName = "Example Mono"
	codeLine2 := textRun("return", 10, 114, 60, 124)
	codeLine2.Style.FontName = "Example Mono"
	layout := &document.Layout{
		Pages: []document.Page{
			{
				Number: 1,
				Width:  400,
				Height: 300,
				TextRuns: []document.TextRun{
					textRun("A", 10, 10, 20, 20),
					textRun("B", 60, 10, 70, 20),
					textRun("C", 110, 10, 120, 20),
					textRun("D", 10, 24, 20, 34),
					textRun("E", 60, 24, 70, 34),
					textRun("F", 110, 24, 120, 34),
					textRun("G", 10, 38, 20, 48),
					textRun("H", 60, 38, 70, 48),
					textRun("I", 110, 38, 120, 48),
					codeLine1,
					codeLine2,
				},
			},
		},
	}

	result, err := analyze.NewBasicAnalyzer().Analyze(context.Background(), layout)
	if err != nil {
		t.Fatalf("Analyze() returned an unexpected error: %v", err)
	}
	want := []document.Diagnostic{
		{
			Code:    document.DiagnosticTableLikeText,
			Page:    1,
			Message: "table-like layout was preserved as plain text",
		},
		{
			Code:    document.DiagnosticCodeLikeText,
			Page:    1,
			Message: "code-like layout was preserved as plain text",
		},
	}
	if !reflect.DeepEqual(result.Diagnostics, want) {
		t.Fatalf("diagnostics = %#v, want %#v", result.Diagnostics, want)
	}
}

func TestBasicAnalyzerReportsAmbiguousLinkAnnotations(t *testing.T) {
	t.Parallel()

	layout := &document.Layout{
		Pages: []document.Page{
			{
				Number: 1,
				Width:  220,
				Height: 220,
				TextRuns: []document.TextRun{
					textRun("link", 10, 10, 40, 20),
				},
				Links: []document.LinkAnnotation{
					{
						Bounds:      document.Rectangle{Left: 8, Top: 8, Right: 42, Bottom: 22},
						Destination: "https://example.test/one",
					},
					{
						Bounds:      document.Rectangle{Left: 8, Top: 8, Right: 42, Bottom: 22},
						Destination: "https://example.test/two",
					},
				},
			},
		},
	}

	result, err := analyze.NewBasicAnalyzer().Analyze(context.Background(), layout)
	if err != nil {
		t.Fatalf("Analyze() returned an unexpected error: %v", err)
	}
	if got, want := len(result.Diagnostics), 1; got != want {
		t.Fatalf("diagnostic count = %d, want %d", got, want)
	}
	if result.Diagnostics[0].Code != document.DiagnosticAmbiguousLink {
		t.Fatalf(
			"diagnostic code = %q, want %q",
			result.Diagnostics[0].Code,
			document.DiagnosticAmbiguousLink,
		)
	}
	paragraph := result.Blocks[0].(*document.Paragraph)
	if len(paragraph.Links) != 0 {
		t.Fatalf("paragraph links = %#v, want none", paragraph.Links)
	}
}

func TestBasicAnalyzerSuppressesRepeatedHeadersAndFooters(t *testing.T) {
	t.Parallel()

	layout := &document.Layout{}
	for page := 1; page <= 3; page++ {
		layout.Pages = append(layout.Pages, document.Page{
			Number: page,
			Width:  220,
			Height: 220,
			TextRuns: []document.TextRun{
				textRun("Quarterly report", 10, 8, 110, 18),
				textRun(fmt.Sprintf("Body page %d.", page), 10, 60, 100, 70),
				textRun(fmt.Sprintf("Page %d", page), 90, 202, 130, 212),
			},
		})
	}

	result, err := analyze.NewBasicAnalyzer().Analyze(context.Background(), layout)
	if err != nil {
		t.Fatalf("Analyze() returned an unexpected error: %v", err)
	}
	want := []string{"Body page 1.", "Body page 2.", "Body page 3."}
	if got := paragraphTexts(t, result); !reflect.DeepEqual(got, want) {
		t.Fatalf("paragraphs = %#v, want %#v", got, want)
	}
}

func TestBasicAnalyzerPreservesRepeatedBodyText(t *testing.T) {
	t.Parallel()

	layout := &document.Layout{}
	for page := 1; page <= 3; page++ {
		layout.Pages = append(layout.Pages, document.Page{
			Number: page,
			Width:  220,
			Height: 220,
			TextRuns: []document.TextRun{
				textRun("Confidential", 70, 100, 150, 110),
			},
		})
	}

	result, err := analyze.NewBasicAnalyzer().Analyze(context.Background(), layout)
	if err != nil {
		t.Fatalf("Analyze() returned an unexpected error: %v", err)
	}
	want := []string{"Confidential", "Confidential", "Confidential"}
	if got := paragraphTexts(t, result); !reflect.DeepEqual(got, want) {
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
		clone.Pages[index].Links = append(
			[]document.LinkAnnotation(nil),
			layout.Pages[index].Links...,
		)
	}
	return clone
}
