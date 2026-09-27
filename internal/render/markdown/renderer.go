package markdown

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/document"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/render"
)

var (
	errNilDocument = errors.New("document must not be nil")
	errNilOutput   = errors.New("output writer must not be nil")
)

// Renderer writes semantic documents as Markdown.
type Renderer struct{}

var _ render.Renderer = (*Renderer)(nil)

// NewRenderer creates a Markdown renderer.
func NewRenderer() *Renderer {
	return &Renderer{}
}

// Render writes semantic blocks separated by one blank line.
func (*Renderer) Render(
	ctx context.Context,
	doc *document.Document,
	output io.Writer,
) error {
	if doc == nil {
		return errNilDocument
	}
	if output == nil {
		return errNilOutput
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("render Markdown: %w", err)
	}
	if err := doc.Validate(); err != nil {
		return fmt.Errorf("validate document for Markdown rendering: %w", err)
	}
	if err := validateBlocks(ctx, doc); err != nil {
		return err
	}

	for index, block := range doc.Blocks {
		rendered, err := renderBlock(ctx, block)
		if err != nil {
			return fmt.Errorf("render block %d: %w", index+1, err)
		}
		if index > 0 {
			if err := writeString(ctx, output, "\n"); err != nil {
				return fmt.Errorf("write block %d separator: %w", index+1, err)
			}
		}

		if err := writeString(ctx, output, rendered+"\n"); err != nil {
			return fmt.Errorf("write block %d: %w", index+1, err)
		}
	}
	return nil
}

func validateBlocks(ctx context.Context, doc *document.Document) error {
	for index, block := range doc.Blocks {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("validate Markdown block %d: %w", index+1, err)
		}
		var text string
		switch typed := block.(type) {
		case *document.Paragraph:
			text = typed.Text
		case *document.Heading:
			text = typed.Text
		default:
			return fmt.Errorf(
				"render Markdown block %d: unsupported block type %T",
				index+1,
				block,
			)
		}
		if !utf8.ValidString(text) {
			return fmt.Errorf("render Markdown block %d: text is not valid UTF-8", index+1)
		}
	}
	return nil
}

func renderBlock(ctx context.Context, block document.Block) (string, error) {
	switch typed := block.(type) {
	case *document.Paragraph:
		return escapeParagraph(ctx, normalizeLineEndings(typed.Text))
	case *document.Heading:
		content, err := escapeParagraph(ctx, typed.Text)
		if err != nil {
			return "", err
		}
		return strings.Repeat("#", typed.Level) + " " + content, nil
	default:
		return "", fmt.Errorf("unsupported block type %T", block)
	}
}

func writeString(ctx context.Context, output io.Writer, content string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	written, err := io.WriteString(output, content)
	if err != nil {
		return err
	}
	if written != len(content) {
		return io.ErrShortWrite
	}
	return nil
}

func normalizeLineEndings(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}

func escapeParagraph(ctx context.Context, text string) (string, error) {
	lines := strings.Split(text, "\n")
	var result strings.Builder
	result.Grow(len(text))

	for index, line := range lines {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if index > 0 {
			result.WriteByte('\n')
		}
		if err := escapeLine(ctx, &result, line); err != nil {
			return "", err
		}
	}
	return result.String(), nil
}

func escapeLine(ctx context.Context, output *strings.Builder, line string) error {
	if strings.HasPrefix(line, "\t") {
		output.WriteString("&#9;")
		line = line[1:]
	} else if strings.HasPrefix(line, "    ") {
		output.WriteString("&#32;")
		line = line[1:]
	}

	blockMarker := markdownBlockMarker(line)
	for index, value := range line {
		if err := ctx.Err(); err != nil {
			return err
		}
		if index == blockMarker || isInlineMarkdownPunctuation(value) {
			output.WriteByte('\\')
		}
		output.WriteRune(value)
	}
	return nil
}

func markdownBlockMarker(line string) int {
	first := firstNonSpace(line)
	if first < 0 {
		return -1
	}

	switch line[first] {
	case '#', '+':
		if markerIsFollowedBySpace(line, first) {
			return first
		}
	case '-':
		if markerIsFollowedBySpace(line, first) || isThematicBreak(line[first:]) {
			return first
		}
	case '=':
		if isSetextUnderline(line[first:]) {
			return first
		}
	}

	digitEnd := first
	for digitEnd < len(line) &&
		digitEnd-first < 9 &&
		line[digitEnd] >= '0' &&
		line[digitEnd] <= '9' {
		digitEnd++
	}
	if digitEnd == first || digitEnd >= len(line) {
		return -1
	}
	if line[digitEnd] != '.' && line[digitEnd] != ')' {
		return -1
	}
	if markerIsFollowedBySpace(line, digitEnd) {
		return digitEnd
	}
	return -1
}

func firstNonSpace(line string) int {
	for index := 0; index < len(line) && index < 4; index++ {
		if line[index] != ' ' {
			return index
		}
	}
	return -1
}

func markerIsFollowedBySpace(line string, marker int) bool {
	next := marker + 1
	return next == len(line) || line[next] == ' ' || line[next] == '\t'
}

func isThematicBreak(line string) bool {
	count := 0
	for _, value := range line {
		switch value {
		case '-':
			count++
		case ' ', '\t':
		default:
			return false
		}
	}
	return count >= 3
}

func isSetextUnderline(line string) bool {
	count := 0
	for _, value := range line {
		switch value {
		case '=':
			count++
		case ' ', '\t':
		default:
			return false
		}
	}
	return count > 0
}

func isInlineMarkdownPunctuation(value rune) bool {
	switch value {
	case '\\', '`', '*', '_', '[', ']', '<', '>', '&', '|', '~':
		return true
	default:
		return false
	}
}
