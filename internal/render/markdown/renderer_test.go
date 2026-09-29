package markdown_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/document"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/render/markdown"
)

func TestRendererWritesPlainParagraphs(t *testing.T) {
	t.Parallel()

	doc := &document.Document{
		Blocks: []document.Block{
			&document.Paragraph{Text: "First paragraph."},
			&document.Paragraph{Text: "Second paragraph."},
		},
	}
	before := cloneDocument(*doc)
	var output bytes.Buffer

	err := markdown.NewRenderer().Render(context.Background(), doc, &output)
	if err != nil {
		t.Fatalf("Render() returned an unexpected error: %v", err)
	}
	if got, want := output.String(), "First paragraph.\n\nSecond paragraph.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(*doc, before) {
		t.Fatal("Render() mutated its input document")
	}
}

func TestRendererWritesHeadingAndParagraph(t *testing.T) {
	t.Parallel()

	doc := &document.Document{
		Blocks: []document.Block{
			&document.Heading{Level: 2, Text: "A *literal* heading"},
			&document.Paragraph{Text: "Body text."},
		},
	}
	var output bytes.Buffer

	err := markdown.NewRenderer().Render(context.Background(), doc, &output)
	if err != nil {
		t.Fatalf("Render() returned an unexpected error: %v", err)
	}
	if got, want := output.String(), "## A \\*literal\\* heading\n\nBody text.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestRendererWritesBlankFieldAndTableCheckboxes(t *testing.T) {
	t.Parallel()

	doc := &document.Document{Blocks: []document.Block{
		&document.Field{Kind: document.FieldKindBlank, Label: "Approval:"},
		&document.Table{Rows: []document.TableRow{
			{Cells: []document.TableCell{{Text: "Mark"}, {Text: "Choice"}}},
			{Cells: []document.TableCell{{Checkbox: document.CheckboxUnchecked}, {Text: "First"}}},
			{Cells: []document.TableCell{{Checkbox: document.CheckboxChecked}, {Text: "Second"}}},
		}},
	}}
	var output bytes.Buffer

	err := markdown.NewRenderer().Render(context.Background(), doc, &output)
	if err != nil {
		t.Fatalf("Render() returned an unexpected error: %v", err)
	}
	want := "Approval: ________\n\n" +
		"| Mark | Choice |\n" +
		"| --- | --- |\n" +
		"| [ ] | First |\n" +
		"| [x] | Second |\n"
	if got := output.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestRendererWritesOrderedAndUnorderedLists(t *testing.T) {
	t.Parallel()

	doc := &document.Document{
		Blocks: []document.Block{
			&document.Paragraph{Text: "Intro."},
			&document.List{
				Kind: document.ListKindUnordered,
				Items: []document.ListItem{
					{Text: "First *literal* item"},
					{
						Text: "# Not a heading\ncontinued",
						Children: []document.List{
							{
								Kind: document.ListKindUnordered,
								Items: []document.ListItem{
									{Text: "Nested item"},
								},
							},
						},
					},
				},
			},
			&document.List{
				Kind:  document.ListKindOrdered,
				Start: 3,
				Items: []document.ListItem{{Text: "Third item"}, {Text: "Fourth item"}},
			},
		},
	}
	before := cloneDocument(*doc)
	var output bytes.Buffer

	err := markdown.NewRenderer().Render(context.Background(), doc, &output)
	if err != nil {
		t.Fatalf("Render() returned an unexpected error: %v", err)
	}
	want := "Intro.\n\n" +
		"- First \\*literal\\* item\n" +
		"- \\# Not a heading\n" +
		"  continued\n" +
		"  - Nested item\n\n" +
		"3. Third item\n" +
		"4. Fourth item\n"
	if got := output.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(*doc, before) {
		t.Fatal("Render() mutated its input document")
	}
}

func TestRendererWritesSemanticLinks(t *testing.T) {
	t.Parallel()

	doc := &document.Document{
		Blocks: []document.Block{
			&document.Paragraph{
				Text: "Read *the docs*.",
				Links: []document.TextLink{
					{
						Start: 5,
						End:   15,
						Target: document.LinkTarget{
							Kind: document.LinkTargetExternal,
							URI:  "https://example.test/a_(b)?x=1&y=2",
						},
					},
				},
			},
		},
	}
	var output bytes.Buffer

	err := markdown.NewRenderer().Render(context.Background(), doc, &output)
	if err != nil {
		t.Fatalf("Render() returned an unexpected error: %v", err)
	}
	want := "Read [\\*the docs\\*](https://example.test/a_\\(b\\)?x=1&y=2).\n"
	if got := output.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestRendererWritesResolvedInternalLinkAndAnchor(t *testing.T) {
	t.Parallel()

	doc := &document.Document{
		Blocks: []document.Block{
			&document.Heading{
				Level:  2,
				Text:   "2. Results",
				Anchor: "2-results",
			},
			&document.Paragraph{
				Text: "See 2. Results.",
				Links: []document.TextLink{
					{
						Start: 4,
						End:   14,
						Target: document.LinkTarget{
							Kind: document.LinkTargetNamed,
							Name: "2-results",
						},
					},
				},
			},
			&document.Paragraph{
				Text: "Unresolved page target.",
				Links: []document.TextLink{
					{
						Start: 0,
						End:   10,
						Target: document.LinkTarget{
							Kind: document.LinkTargetPage,
							Page: 3,
						},
					},
				},
			},
		},
	}
	var output bytes.Buffer

	err := markdown.NewRenderer().Render(context.Background(), doc, &output)
	if err != nil {
		t.Fatalf("Render() returned an unexpected error: %v", err)
	}
	want := "<a id=\"2-results\"></a>\n" +
		"## 2\\. Results\n\n" +
		"See [2\\. Results](#2-results).\n\n" +
		"Unresolved page target.\n"
	if got := output.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestRendererWritesMarkdownTable(t *testing.T) {
	t.Parallel()

	doc := &document.Document{
		Blocks: []document.Block{
			&document.Table{
				Rows: []document.TableRow{
					{
						Cells: []document.TableCell{
							{Text: "Name"},
							{Text: "Expected | result"},
						},
					},
					{
						Cells: []document.TableCell{
							{Text: "Case *A*"},
							{
								Text: "Passed",
								Links: []document.TextLink{
									{
										Start: 0,
										End:   6,
										Target: document.LinkTarget{
											Kind: document.LinkTargetExternal,
											URI:  "https://example.test/result",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	before := cloneDocument(*doc)
	var output bytes.Buffer

	err := markdown.NewRenderer().Render(context.Background(), doc, &output)
	if err != nil {
		t.Fatalf("Render() returned an unexpected error: %v", err)
	}
	want := "| Name | Expected \\| result |\n" +
		"| --- | --- |\n" +
		"| Case \\*A\\* | [Passed](https://example.test/result) |\n"
	if got := output.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(*doc, before) {
		t.Fatal("Render() mutated its input document")
	}
}

func TestRendererEscapesMarkdownAndNormalizesLineEndings(t *testing.T) {
	t.Parallel()

	doc := &document.Document{
		Blocks: []document.Block{
			&document.Paragraph{
				Text: "# A *plain* [link](https://example.test?a=1&b=2)\r\n" +
					"1. Next | `code`\r" +
					"---\n" +
					"    indented",
			},
		},
	}
	var output bytes.Buffer

	err := markdown.NewRenderer().Render(context.Background(), doc, &output)
	if err != nil {
		t.Fatalf("Render() returned an unexpected error: %v", err)
	}
	want := "\\# A \\*plain\\* \\[link\\](https://example.test?a=1\\&b=2)\n" +
		"1\\. Next \\| \\`code\\`\n" +
		"\\---\n" +
		"&#32;   indented\n"
	if got := output.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if strings.Contains(output.String(), "\r") {
		t.Fatalf("output contains a carriage return: %q", output.String())
	}
}

func TestRendererWritesEmptyDocumentWithoutOutput(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	err := markdown.NewRenderer().Render(
		context.Background(),
		&document.Document{},
		&output,
	)
	if err != nil {
		t.Fatalf("Render() returned an unexpected error: %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("output = %q, want empty output", output.String())
	}
}

func TestRendererRejectsInvalidInputBeforeWriting(t *testing.T) {
	t.Parallel()

	renderer := markdown.NewRenderer()
	tests := []struct {
		name string
		doc  *document.Document
	}{
		{name: "nil document"},
		{
			name: "invalid document",
			doc: &document.Document{
				Blocks: []document.Block{&document.Paragraph{}},
			},
		},
		{
			name: "invalid UTF-8",
			doc: &document.Document{
				Blocks: []document.Block{
					&document.Paragraph{Text: string([]byte{0xff})},
				},
			},
		},
		{
			name: "invalid UTF-8 list item",
			doc: &document.Document{
				Blocks: []document.Block{
					&document.List{
						Kind:  document.ListKindUnordered,
						Items: []document.ListItem{{Text: string([]byte{0xff})}},
					},
				},
			},
		},
		{
			name: "invalid UTF-8 nested list item",
			doc: &document.Document{
				Blocks: []document.Block{
					&document.List{
						Kind: document.ListKindUnordered,
						Items: []document.ListItem{
							{
								Text: "parent",
								Children: []document.List{
									{
										Kind: document.ListKindUnordered,
										Items: []document.ListItem{
											{Text: string([]byte{0xff})},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "invalid UTF-8 table cell",
			doc: &document.Document{
				Blocks: []document.Block{
					&document.Table{
						Rows: []document.TableRow{
							{Cells: []document.TableCell{{Text: "A"}, {Text: "B"}}},
							{
								Cells: []document.TableCell{
									{Text: string([]byte{0xff})},
									{Text: "C"},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			if err := renderer.Render(context.Background(), test.doc, &output); err == nil {
				t.Fatal("Render() returned nil error for invalid input")
			}
			if output.Len() != 0 {
				t.Fatalf("output = %q, want no output", output.String())
			}
		})
	}
}

func TestRendererRejectsNilOutput(t *testing.T) {
	t.Parallel()

	err := markdown.NewRenderer().Render(
		context.Background(),
		&document.Document{},
		nil,
	)
	if err == nil {
		t.Fatal("Render() returned nil error for a nil output writer")
	}
}

func TestRendererHonorsCancellationBeforeWriting(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var output bytes.Buffer
	err := markdown.NewRenderer().Render(
		ctx,
		&document.Document{
			Blocks: []document.Block{
				&document.Paragraph{Text: "content"},
			},
		},
		&output,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Render() error = %v, want %v", err, context.Canceled)
	}
	if output.Len() != 0 {
		t.Fatalf("output = %q, want no output", output.String())
	}
}

func TestRendererPreservesWriterError(t *testing.T) {
	t.Parallel()

	want := errors.New("write failed")
	err := markdown.NewRenderer().Render(
		context.Background(),
		&document.Document{
			Blocks: []document.Block{
				&document.Paragraph{Text: "content"},
			},
		},
		errorWriter{err: want},
	)
	if !errors.Is(err, want) {
		t.Fatalf("Render() error = %v, want %v", err, want)
	}
	if !strings.Contains(err.Error(), "write block 1") {
		t.Fatalf("Render() error = %q, want block context", err)
	}
}

func TestRendererRejectsShortWrite(t *testing.T) {
	t.Parallel()

	err := markdown.NewRenderer().Render(
		context.Background(),
		&document.Document{
			Blocks: []document.Block{
				&document.Paragraph{Text: "content"},
			},
		},
		shortWriter{},
	)
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("Render() error = %v, want %v", err, io.ErrShortWrite)
	}
}

type errorWriter struct {
	err error
}

func (writer errorWriter) Write([]byte) (int, error) {
	return 0, writer.err
}

type shortWriter struct{}

func (shortWriter) Write(content []byte) (int, error) {
	return len(content) - 1, nil
}

func cloneDocument(doc document.Document) document.Document {
	clone := doc
	clone.Blocks = append([]document.Block(nil), doc.Blocks...)
	for index, block := range clone.Blocks {
		switch typed := block.(type) {
		case *document.Paragraph:
			copied := *typed
			copied.Links = append([]document.TextLink(nil), typed.Links...)
			clone.Blocks[index] = &copied
		case *document.Heading:
			copied := *typed
			copied.Links = append([]document.TextLink(nil), typed.Links...)
			clone.Blocks[index] = &copied
		case *document.List:
			copied := cloneList(*typed)
			clone.Blocks[index] = &copied
		case *document.Table:
			copied := cloneTable(*typed)
			clone.Blocks[index] = &copied
		}
	}
	return clone
}

func cloneTable(table document.Table) document.Table {
	clone := table
	clone.Rows = append([]document.TableRow(nil), table.Rows...)
	for rowIndex := range clone.Rows {
		clone.Rows[rowIndex].Cells = append(
			[]document.TableCell(nil),
			table.Rows[rowIndex].Cells...,
		)
		for cellIndex := range clone.Rows[rowIndex].Cells {
			clone.Rows[rowIndex].Cells[cellIndex].Links = append(
				[]document.TextLink(nil),
				table.Rows[rowIndex].Cells[cellIndex].Links...,
			)
		}
	}
	return clone
}

func cloneList(list document.List) document.List {
	clone := list
	clone.Items = append([]document.ListItem(nil), list.Items...)
	for itemIndex := range clone.Items {
		clone.Items[itemIndex].Links = append(
			[]document.TextLink(nil),
			list.Items[itemIndex].Links...,
		)
		if list.Items[itemIndex].Children == nil {
			continue
		}
		clone.Items[itemIndex].Children = make(
			[]document.List,
			len(list.Items[itemIndex].Children),
		)
		for childIndex, child := range list.Items[itemIndex].Children {
			clone.Items[itemIndex].Children[childIndex] = cloneList(child)
		}
	}
	return clone
}
