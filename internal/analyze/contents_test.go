package analyze

import (
	"reflect"
	"testing"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/document"
)

func TestDetectContentsEntriesUsesStructureAndIndentation(t *testing.T) {
	t.Parallel()

	lines := []textLine{
		{text: "Contents", left: 10, top: 10, bottom: 20},
		{text: "1. Introduction ........ 3", left: 10, top: 24, bottom: 34},
		{text: "1.1. Scope ........ 4", left: 24, top: 38, bottom: 48},
		{text: "1.2. At root is not nested ........ 5", left: 10, top: 52, bottom: 62},
		{text: "Not a contents entry", left: 10, top: 66, bottom: 76},
	}

	got := detectContentsEntries(lines)
	if got[0] != nil || got[4] != nil {
		t.Fatalf("non-entry lines were detected: %#v", got)
	}
	wantRoot := &detectedContentsEntry{
		number:    "1.",
		title:     "Introduction",
		page:      3,
		depth:     1,
		lineCount: 1,
	}
	if !reflect.DeepEqual(got[1], wantRoot) {
		t.Fatalf("root entry = %#v, want %#v", got[1], wantRoot)
	}
	wantNested := &detectedContentsEntry{
		number:    "1.1.",
		title:     "Scope",
		page:      4,
		depth:     2,
		lineCount: 1,
	}
	if !reflect.DeepEqual(got[2], wantNested) {
		t.Fatalf("nested entry = %#v, want %#v", got[2], wantNested)
	}
	if got[3] != nil {
		t.Fatalf("unindented nested entry = %#v, want nil", got[3])
	}
}

func TestDetectContentsEntriesJoinsIndentedContinuation(t *testing.T) {
	t.Parallel()

	lines := []textLine{
		{text: "Contents", left: 10, top: 10, bottom: 20},
		{text: "1. Introduction ........ 3", left: 10, top: 24, bottom: 34},
		{
			text:   "1.1. Acknowledgement of Transactional",
			left:   24,
			top:    38,
			bottom: 48,
		},
		{text: "Messages ........ 4", left: 38, top: 52, bottom: 62},
	}

	got := detectContentsEntries(lines)
	want := &detectedContentsEntry{
		number:    "1.1.",
		title:     "Acknowledgement of Transactional Messages",
		page:      4,
		depth:     2,
		lineCount: 2,
	}
	if !reflect.DeepEqual(got[2], want) {
		t.Fatalf("wrapped entry = %#v, want %#v", got[2], want)
	}
	if got[3] != nil {
		t.Fatalf("continuation line = %#v, want nil", got[3])
	}
}

func TestDetectContentsEntryRejectsMissingSignals(t *testing.T) {
	t.Parallel()

	for _, text := range []string{
		"Introduction ........ 3",
		"1. Introduction 3",
		"1. Introduction ........",
		"1. Introduction ........ page 3",
		"1. ........ 3",
	} {
		if got, ok := detectContentsEntry(text); ok {
			t.Fatalf("detectContentsEntry(%q) = %#v, true; want false", text, got)
		}
	}
}

func TestPrepareContentsLinesCollapsesAlignedPageNumbers(t *testing.T) {
	t.Parallel()

	lines := []textLine{
		testContentsLine("Contents", 10, 10, 80, 20),
		testContentsLine("1. Overview", 10, 30, 80, 40),
		testContentsLine("2", 196, 30.5, 202, 39.5),
		testContentsLine("1.1. Details", 22, 44, 94, 54),
		testContentsLine("3", 196, 44.5, 202, 53.5),
		testContentsLineWithPage(
			"2. A deliberately wider title",
			"4",
			10,
			58,
			154,
			196,
			202,
		),
	}

	got := prepareContentsLines(lines, 220, false)
	if len(got) != 4 {
		t.Fatalf("line count = %d, want 4", len(got))
	}
	want := []detectedContentsEntry{
		{number: "1.", title: "Overview", page: 2, depth: 1, lineCount: 1},
		{number: "1.1.", title: "Details", page: 3, depth: 2, lineCount: 1},
		{
			number:    "2.",
			title:     "A deliberately wider title",
			page:      4,
			depth:     1,
			lineCount: 1,
		},
	}
	for index, entry := range want {
		line := got[index+1]
		if line.contents == nil {
			t.Fatalf("line %d contents = nil", index+2)
		}
		if !reflect.DeepEqual(*line.contents, entry) {
			t.Fatalf(
				"line %d contents = %#v, want %#v",
				index+2,
				*line.contents,
				entry,
			)
		}
		if line.right != 202 {
			t.Fatalf("line %d right = %v, want 202", index+2, line.right)
		}
	}
}

func TestPrepareContentsLinesRequiresRepeatedAlignedRows(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		lines []textLine
	}{
		{
			name: "no contents heading",
			lines: []textLine{
				testContentsLine("Summary", 10, 10, 80, 20),
				testContentsLine("1. Overview", 10, 30, 80, 40),
				testContentsLine("2", 196, 30, 202, 40),
				testContentsLine("2. Results", 10, 44, 76, 54),
				testContentsLine("3", 196, 44, 202, 54),
			},
		},
		{
			name: "single row",
			lines: []textLine{
				testContentsLine("Contents", 10, 10, 80, 20),
				testContentsLine("1. Overview", 10, 30, 80, 40),
				testContentsLine("2", 196, 30, 202, 40),
			},
		},
		{
			name: "inconsistent page alignment",
			lines: []textLine{
				testContentsLine("Contents", 10, 10, 80, 20),
				testContentsLine("1. Overview", 10, 30, 80, 40),
				testContentsLine("2", 196, 30, 202, 40),
				testContentsLine("2. Results", 10, 44, 76, 54),
				testContentsLine("3", 170, 44, 176, 54),
			},
		},
		{
			name: "unindented nested entry",
			lines: []textLine{
				testContentsLine("Contents", 10, 10, 80, 20),
				testContentsLine("1. Overview", 10, 30, 80, 40),
				testContentsLine("2", 196, 30, 202, 40),
				testContentsLine("1.1. Details", 10, 44, 82, 54),
				testContentsLine("3", 196, 44, 202, 54),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := prepareContentsLines(test.lines, 220, false)
			if !reflect.DeepEqual(got, test.lines) {
				t.Fatalf("prepareContentsLines() changed rejected geometry")
			}
		})
	}
}

func TestPrepareContentsLinesAcceptsValidatedContinuation(t *testing.T) {
	t.Parallel()

	lines := []textLine{
		testContentsLine("Running header", 140, 10, 205, 20),
		testContentsLine("4.9.1. Data integrity", 30, 30, 120, 40),
		testContentsLine("20", 190, 30, 202, 40),
		testContentsLine("5. Verification", 10, 44, 90, 54),
		testContentsLine("21", 190, 44, 202, 54),
	}

	if got := prepareContentsLines(lines, 220, false); !reflect.DeepEqual(got, lines) {
		t.Fatal("continuation was accepted without preceding-page context")
	}
	got := prepareContentsLines(lines, 220, true)
	if len(got) != 3 {
		t.Fatalf("line count = %d, want 3", len(got))
	}
	if got[1].contents == nil || got[1].contents.depth != 3 {
		t.Fatalf("first continuation entry = %#v, want depth 3", got[1].contents)
	}
	if got[2].contents == nil || got[2].contents.depth != 1 {
		t.Fatalf("second continuation entry = %#v, want depth 1", got[2].contents)
	}
}

func TestPrepareContentsLinesJoinsWrappedGeometricEntry(t *testing.T) {
	t.Parallel()

	lines := []textLine{
		testContentsLine("Contents", 10, 10, 80, 20),
		testContentsLine("1. Overview", 10, 30, 80, 40),
		testContentsLine("2", 196, 30, 202, 40),
		testContentsLine("1.1. A deliberately wrapped", 22, 44, 150, 54),
		testContentsLine("contents title", 30, 56, 100, 66),
		testContentsLine("3", 196, 56, 202, 66),
	}

	got := prepareContentsLines(lines, 220, false)
	if len(got) != 3 {
		t.Fatalf("line count = %d, want 3", len(got))
	}
	entry := got[2].contents
	if entry == nil {
		t.Fatal("wrapped entry contents = nil")
	}
	if got, want := entry.title,
		"A deliberately wrapped contents title"; got != want {
		t.Fatalf("wrapped entry title = %q, want %q", got, want)
	}
	if got, want := entry.page, 3; got != want {
		t.Fatalf("wrapped entry page = %d, want %d", got, want)
	}
}

func TestMergeContentsContinuationPreservesHierarchy(t *testing.T) {
	t.Parallel()

	previous := &document.List{
		Kind: document.ListKindUnordered,
		Items: []document.ListItem{
			{
				Text: "4. Requirements",
				Children: []document.List{
					{
						Kind: document.ListKindUnordered,
						Items: []document.ListItem{
							{Text: "4.9. General"},
						},
					},
				},
			},
		},
	}
	continuation := &document.List{
		Kind: document.ListKindUnordered,
		Items: []document.ListItem{
			{
				Text: "4.9.1. Data integrity",
				Children: []document.List{
					{
						Kind: document.ListKindUnordered,
						Items: []document.ListItem{
							{Text: "4.10. Reporting"},
							{
								Text: "4.10.1. Authentication",
							},
						},
					},
				},
			},
			{Text: "5. Verification"},
		},
	}

	got := mergeContentsContinuation(
		[]document.Block{previous},
		[]document.Block{continuation},
	)
	if len(got) != 0 {
		t.Fatalf("remaining blocks = %d, want 0", len(got))
	}
	requirements := previous.Items[0]
	levelTwo := requirements.Children[0].Items
	if got, want := len(levelTwo), 2; got != want {
		t.Fatalf("level-two item count = %d, want %d", got, want)
	}
	if got, want := levelTwo[0].Children[0].Items[0].Text,
		"4.9.1. Data integrity"; got != want {
		t.Fatalf("4.9 child = %q, want %q", got, want)
	}
	if got, want := levelTwo[1].Text, "4.10. Reporting"; got != want {
		t.Fatalf("second level-two item = %q, want %q", got, want)
	}
	if got, want := levelTwo[1].Children[0].Items[0].Text,
		"4.10.1. Authentication"; got != want {
		t.Fatalf("4.10 child = %q, want %q", got, want)
	}
	if got, want := previous.Items[1].Text, "5. Verification"; got != want {
		t.Fatalf("next root item = %q, want %q", got, want)
	}
}

func TestNumberedHeadingContinuationRejectsDifferentScale(t *testing.T) {
	t.Parallel()

	current := textLine{
		text:   "2. Release profile",
		top:    10,
		bottom: 30,
		runs: []orderedRun{{
			run: document.TextRun{
				Style: document.TextStyle{FontSize: 20, FontWeight: 700},
			},
		}},
	}
	next := textLine{
		text:   "N/A",
		top:    38,
		bottom: 48,
		runs: []orderedRun{{
			run: document.TextRun{
				Style: document.TextStyle{FontSize: 10, FontWeight: 700},
			},
		}},
	}

	if isNumberedHeadingContinuation(current, next, 1, 0) {
		t.Fatal("smaller body line was accepted as a heading continuation")
	}
}

func testContentsLine(
	text string,
	left,
	top,
	right,
	bottom float64,
) textLine {
	run := document.TextRun{
		Text:   text,
		Bounds: document.Rectangle{Left: left, Top: top, Right: right, Bottom: bottom},
		Style:  document.TextStyle{FontSize: bottom - top},
	}
	return textLine{
		runs:   []orderedRun{{run: run, text: text}},
		top:    top,
		bottom: bottom,
		left:   left,
		right:  right,
		text:   text,
	}
}

func testContentsLineWithPage(
	title,
	page string,
	left,
	top,
	titleRight,
	pageLeft,
	pageRight float64,
) textLine {
	line := testContentsLine(title, left, top, titleRight, top+10)
	pageRun := document.TextRun{
		Text: page,
		Bounds: document.Rectangle{
			Left:   pageLeft,
			Top:    top,
			Right:  pageRight,
			Bottom: top + 10,
		},
		Style: document.TextStyle{FontSize: 10},
	}
	line.runs = append(line.runs, orderedRun{run: pageRun, text: page})
	line.right = pageRight
	line.text += " " + page
	return line
}
