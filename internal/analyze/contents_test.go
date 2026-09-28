package analyze

import (
	"reflect"
	"testing"
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
