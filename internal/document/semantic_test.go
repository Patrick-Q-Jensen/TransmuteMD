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
			&document.Paragraph{
				Text: "Visit the site.",
				Links: []document.TextLink{
					{Start: 10, End: 14, Destination: "https://example.test"},
				},
			},
			&document.List{
				Kind: document.ListKindUnordered,
				Items: []document.ListItem{
					{
						Text: "First item",
						Children: []document.List{
							{
								Kind: document.ListKindUnordered,
								Items: []document.ListItem{
									{Text: "Nested item"},
								},
							},
						},
					},
					{Text: "Second item"},
				},
			},
			&document.List{
				Kind:  document.ListKindOrdered,
				Start: 3,
				Items: []document.ListItem{{Text: "Third item"}, {Text: "Fourth item"}},
			},
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

func TestDocumentValidateRejectsInvalidList(t *testing.T) {
	t.Parallel()

	var nilList *document.List
	tests := []struct {
		name string
		list *document.List
	}{
		{name: "nil", list: nilList},
		{name: "unknown kind", list: &document.List{Kind: document.ListKind(99), Items: []document.ListItem{{Text: "item"}}}},
		{name: "unordered start", list: &document.List{Kind: document.ListKindUnordered, Start: 1, Items: []document.ListItem{{Text: "item"}}}},
		{name: "negative ordered start", list: &document.List{Kind: document.ListKindOrdered, Start: -1, Items: []document.ListItem{{Text: "item"}}}},
		{name: "oversized ordered start", list: &document.List{Kind: document.ListKindOrdered, Start: 1_000_000_000, Items: []document.ListItem{{Text: "item"}}}},
		{name: "oversized ordered range", list: &document.List{Kind: document.ListKindOrdered, Start: 999_999_999, Items: []document.ListItem{{Text: "item"}, {Text: "item"}}}},
		{name: "no items", list: &document.List{Kind: document.ListKindUnordered}},
		{name: "empty item", list: &document.List{Kind: document.ListKindUnordered, Items: []document.ListItem{{}}}},
		{
			name: "invalid child",
			list: &document.List{
				Kind: document.ListKindUnordered,
				Items: []document.ListItem{
					{
						Text:     "item",
						Children: []document.List{{Kind: document.ListKindUnordered}},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			doc := document.Document{Blocks: []document.Block{test.list}}
			if err := doc.Validate(); err == nil {
				t.Fatalf("Validate() returned nil for invalid list %+v", test.list)
			}
		})
	}
}

func TestDocumentValidateRejectsInvalidTextLink(t *testing.T) {
	t.Parallel()

	tests := []document.TextLink{
		{Start: -1, End: 1, Destination: "https://example.test"},
		{Start: 0, End: 20, Destination: "https://example.test"},
		{Start: 0, End: 4, Destination: "relative"},
		{Start: 0, End: 4, Destination: "javascript:alert(1)"},
	}
	for _, link := range tests {
		doc := document.Document{Blocks: []document.Block{
			&document.Paragraph{Text: "text", Links: []document.TextLink{link}},
		}}
		if err := doc.Validate(); err == nil {
			t.Fatalf("Validate() returned nil for invalid link %+v", link)
		}
	}
}
