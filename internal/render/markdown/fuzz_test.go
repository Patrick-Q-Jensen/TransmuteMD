package markdown_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/document"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/render/markdown"
)

func FuzzRendererParagraph(f *testing.F) {
	f.Add("plain text")
	f.Add("# heading\r\n* emphasis *")
	f.Add("[label](https://example.test)\n---")

	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 4096 {
			t.Skip()
		}
		input = strings.ToValidUTF8(input, "\uFFFD")
		if input == "" {
			input = " "
		}

		doc := &document.Document{
			Blocks: []document.Block{
				&document.Paragraph{Text: input},
			},
		}
		var output bytes.Buffer
		if err := markdown.NewRenderer().Render(
			context.Background(),
			doc,
			&output,
		); err != nil {
			t.Fatalf("Render() returned an unexpected error: %v", err)
		}
		if !utf8.Valid(output.Bytes()) {
			t.Fatal("Render() returned invalid UTF-8")
		}
		if bytes.ContainsRune(output.Bytes(), '\r') {
			t.Fatalf("Render() retained a carriage return: %q", output.Bytes())
		}
	})
}
