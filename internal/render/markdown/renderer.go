package markdown

import (
	"context"
	"errors"
	"fmt"
	"html"
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
	anchors := documentAnchors(doc)

	for index, block := range doc.Blocks {
		rendered, err := renderBlock(ctx, block, anchors)
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

func documentAnchors(doc *document.Document) map[string]struct{} {
	anchors := make(map[string]struct{})
	for _, block := range doc.Blocks {
		if heading, ok := block.(*document.Heading); ok && heading.Anchor != "" {
			anchors[heading.Anchor] = struct{}{}
		}
	}
	return anchors
}

func validateBlocks(ctx context.Context, doc *document.Document) error {
	for index, block := range doc.Blocks {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("validate Markdown block %d: %w", index+1, err)
		}
		var text string
		switch typed := block.(type) {
		case *document.Field:
			text = typed.Label
		case *document.Paragraph:
			text = typed.Text
		case *document.Heading:
			text = typed.Text
		case *document.List:
			if err := validateListText(typed); err != nil {
				return fmt.Errorf("render Markdown block %d: %w", index+1, err)
			}
			continue
		case *document.Table:
			for rowIndex, row := range typed.Rows {
				for cellIndex, cell := range row.Cells {
					if !utf8.ValidString(cell.Text) {
						return fmt.Errorf(
							"render Markdown block %d: table row %d cell %d: text is not valid UTF-8",
							index+1,
							rowIndex+1,
							cellIndex+1,
						)
					}
				}
			}
			continue
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

func validateListText(list *document.List) error {
	for itemIndex, item := range list.Items {
		if !utf8.ValidString(item.Text) {
			return fmt.Errorf(
				"list item %d: text is not valid UTF-8",
				itemIndex+1,
			)
		}
		for childIndex := range item.Children {
			if err := validateListText(&item.Children[childIndex]); err != nil {
				return fmt.Errorf(
					"list item %d child list %d: %w",
					itemIndex+1,
					childIndex+1,
					err,
				)
			}
		}
	}
	return nil
}

func renderBlock(
	ctx context.Context,
	block document.Block,
	anchors map[string]struct{},
) (string, error) {
	switch typed := block.(type) {
	case *document.Field:
		label, err := renderLinkedText(ctx, typed.Label, typed.Links, anchors)
		if err != nil {
			return "", err
		}
		switch typed.Kind {
		case document.FieldKindBlank:
			return label + " ________", nil
		default:
			return "", fmt.Errorf("unsupported field kind %d", typed.Kind)
		}
	case *document.Paragraph:
		return renderLinkedText(ctx, typed.Text, typed.Links, anchors)
	case *document.Heading:
		content, err := renderLinkedText(ctx, typed.Text, typed.Links, anchors)
		if err != nil {
			return "", err
		}
		heading := strings.Repeat("#", typed.Level) + " " + content
		if typed.Anchor != "" {
			heading = `<a id="` + html.EscapeString(typed.Anchor) + `"></a>` +
				"\n" + heading
		}
		return heading, nil
	case *document.List:
		return renderList(ctx, typed, anchors)
	case *document.Table:
		return renderTable(ctx, typed, anchors)
	default:
		return "", fmt.Errorf("unsupported block type %T", block)
	}
}

func renderTable(
	ctx context.Context,
	table *document.Table,
	anchors map[string]struct{},
) (string, error) {
	var result strings.Builder
	for rowIndex, row := range table.Rows {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if rowIndex > 0 {
			result.WriteByte('\n')
		}
		result.WriteByte('|')
		for cellIndex, cell := range row.Cells {
			var content string
			switch cell.Checkbox {
			case document.CheckboxNone:
				var err error
				content, err = renderLinkedText(ctx, cell.Text, cell.Links, anchors)
				if err != nil {
					return "", fmt.Errorf(
						"render table row %d cell %d: %w",
						rowIndex+1,
						cellIndex+1,
						err,
					)
				}
			case document.CheckboxUnchecked:
				content = "[ ]"
			case document.CheckboxChecked:
				content = "[x]"
			default:
				return "", fmt.Errorf(
					"render table row %d cell %d: unsupported checkbox state %d",
					rowIndex+1,
					cellIndex+1,
					cell.Checkbox,
				)
			}
			result.WriteByte(' ')
			result.WriteString(content)
			result.WriteString(" |")
		}
		if rowIndex == 0 {
			result.WriteByte('\n')
			result.WriteByte('|')
			for range row.Cells {
				result.WriteString(" --- |")
			}
		}
	}
	return result.String(), nil
}

func renderList(
	ctx context.Context,
	list *document.List,
	anchors map[string]struct{},
) (string, error) {
	return renderListIndented(ctx, list, "", anchors)
}

func renderListIndented(
	ctx context.Context,
	list *document.List,
	indent string,
	anchors map[string]struct{},
) (string, error) {
	var result strings.Builder
	for index, item := range list.Items {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if index > 0 {
			result.WriteByte('\n')
		}

		result.WriteString(indent)
		prefix := "- "
		if list.Kind == document.ListKindOrdered {
			prefix = fmt.Sprintf("%d. ", list.Start+index)
		}
		content, err := renderLinkedText(
			ctx,
			item.Text,
			item.Links,
			anchors,
		)
		if err != nil {
			return "", fmt.Errorf("render list item %d: %w", index+1, err)
		}
		continuationIndent := indent + strings.Repeat(" ", len(prefix))
		content = strings.ReplaceAll(content, "\n", "\n"+continuationIndent)
		result.WriteString(prefix)
		result.WriteString(content)
		for childIndex := range item.Children {
			childIndent := indent + strings.Repeat(" ", len(prefix))
			child, err := renderListIndented(
				ctx,
				&item.Children[childIndex],
				childIndent,
				anchors,
			)
			if err != nil {
				return "", fmt.Errorf(
					"render list item %d child list %d: %w",
					index+1,
					childIndex+1,
					err,
				)
			}
			result.WriteByte('\n')
			result.WriteString(child)
		}
	}
	return result.String(), nil
}

func renderLinkedText(
	ctx context.Context,
	text string,
	links []document.TextLink,
	anchors map[string]struct{},
) (string, error) {
	if len(links) == 0 {
		return escapeParagraph(ctx, normalizeLineEndings(text))
	}

	var result strings.Builder
	start := 0
	for _, link := range links {
		before, err := escapeParagraph(
			ctx,
			normalizeLineEndings(text[start:link.Start]),
		)
		if err != nil {
			return "", err
		}
		label, err := escapeParagraph(
			ctx,
			normalizeLineEndings(text[link.Start:link.End]),
		)
		if err != nil {
			return "", err
		}
		result.WriteString(before)
		destination := ""
		switch link.Target.Kind {
		case document.LinkTargetExternal:
			destination = link.Target.URI
		case document.LinkTargetNamed:
			if _, ok := anchors[link.Target.Name]; ok {
				destination = "#" + link.Target.Name
			}
		}
		if destination == "" {
			result.WriteString(label)
			start = link.End
			continue
		}
		result.WriteByte('[')
		result.WriteString(label)
		result.WriteString("](")
		result.WriteString(escapeLinkDestination(destination))
		result.WriteByte(')')
		start = link.End
	}
	after, err := escapeParagraph(ctx, normalizeLineEndings(text[start:]))
	if err != nil {
		return "", err
	}
	result.WriteString(after)
	return result.String(), nil
}

func escapeLinkDestination(destination string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`(`, `\(`,
		`)`, `\)`,
		`<`, `%3C`,
		`>`, `%3E`,
	)
	return replacer.Replace(destination)
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
