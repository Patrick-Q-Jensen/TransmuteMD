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

func TestRendererWritesOrderedAndUnorderedLists(t *testing.T) {
	t.Parallel()

	doc := &document.Document{
		Blocks: []document.Block{
			&document.Paragraph{Text: "Intro."},
			&document.List{
				Kind: document.ListKindUnordered,
				Items: []string{
					"First *literal* item",
					"# Not a heading\ncontinued",
				},
			},
			&document.List{
				Kind:  document.ListKindOrdered,
				Start: 3,
				Items: []string{"Third item", "Fourth item"},
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
		"  continued\n\n" +
		"3. Third item\n" +
		"4. Fourth item\n"
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
						Items: []string{string([]byte{0xff})},
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
			clone.Blocks[index] = &copied
		case *document.Heading:
			copied := *typed
			clone.Blocks[index] = &copied
		case *document.List:
			copied := *typed
			copied.Items = append([]string(nil), typed.Items...)
			clone.Blocks[index] = &copied
		}
	}
	return clone
}
