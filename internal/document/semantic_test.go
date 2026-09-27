package document_test

import (
	"strings"
	"testing"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/document"
)

func TestDocumentValidate(t *testing.T) {
	t.Parallel()

	doc := document.Document{
		Blocks: []document.Block{
			&document.Heading{Level: 1, Text: "Title"},
			&document.Paragraph{Text: "A plain paragraph."},
		},
	}

	if err := doc.Validate(); err != nil {
		t.Fatalf("Validate() returned an unexpected error: %v", err)
	}
}

func TestDocumentValidateRejectsInvalidHeading(t *testing.T) {
	t.Parallel()

	tests := []document.Heading{
		{Level: 0, Text: "Title"},
		{Level: 7, Text: "Title"},
		{Level: 1},
		{Level: 1, Text: "Two\nlines"},
	}

	for _, heading := range tests {
		doc := document.Document{
			Blocks: []document.Block{&heading},
		}
		if err := doc.Validate(); err == nil {
			t.Fatalf("Validate() returned nil for invalid heading %+v", heading)
		}
	}
}

func TestDocumentValidateRejectsNilBlock(t *testing.T) {
	t.Parallel()

	doc := document.Document{Blocks: []document.Block{nil}}

	err := doc.Validate()
	if err == nil {
		t.Fatal("Validate() returned nil for a nil block")
	}
	if !strings.Contains(err.Error(), "block 1") {
		t.Fatalf("Validate() error = %q, want block context", err)
	}
}

func TestDocumentValidateRejectsEmptyParagraph(t *testing.T) {
	t.Parallel()

	doc := document.Document{
		Blocks: []document.Block{
			&document.Paragraph{},
		},
	}

	err := doc.Validate()
	if err == nil {
		t.Fatal("Validate() returned nil for an empty paragraph")
	}
	if !strings.Contains(err.Error(), "paragraph text must not be empty") {
		t.Fatalf("Validate() error = %q, want empty-paragraph detail", err)
	}
}

func TestDocumentValidateRejectsNilParagraph(t *testing.T) {
	t.Parallel()

	var paragraph *document.Paragraph
	doc := document.Document{Blocks: []document.Block{paragraph}}

	err := doc.Validate()
	if err == nil {
		t.Fatal("Validate() returned nil for a nil paragraph")
	}
	if !strings.Contains(err.Error(), "paragraph must not be nil") {
		t.Fatalf("Validate() error = %q, want nil-paragraph detail", err)
	}
}
