package document

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

const maximumOrderedListMarker = 999_999_999

// Document contains semantic blocks in reading order.
type Document struct {
	Blocks []Block
}

// Validate checks the semantic document invariants required by renderers.
func (d Document) Validate() error {
	for i, block := range d.Blocks {
		if block == nil {
			return fmt.Errorf("block %d: must not be nil", i+1)
		}
		if err := block.validate(); err != nil {
			return fmt.Errorf("block %d: %w", i+1, err)
		}
	}
	return nil
}

// Block is semantic content that can be rendered. Implementations are defined
// by this package so every renderer can handle the complete block set.
type Block interface {
	isBlock()
	validate() error
}

// Paragraph is a block of plain text.
type Paragraph struct {
	Text  string
	Links []TextLink
}

func (*Paragraph) isBlock() {}

func (p *Paragraph) validate() error {
	if p == nil {
		return errors.New("paragraph must not be nil")
	}
	if p.Text == "" {
		return errors.New("paragraph text must not be empty")
	}
	return validateTextLinks(p.Text, p.Links)
}

// Heading is a section title with a Markdown-compatible level from 1 to 6.
type Heading struct {
	Level int
	Text  string
	Links []TextLink
}

func (*Heading) isBlock() {}

func (h *Heading) validate() error {
	if h == nil {
		return errors.New("heading must not be nil")
	}
	if h.Level < 1 || h.Level > 6 {
		return errors.New("heading level must be between 1 and 6")
	}
	if h.Text == "" {
		return errors.New("heading text must not be empty")
	}
	if strings.ContainsAny(h.Text, "\r\n") {
		return errors.New("heading text must be a single line")
	}
	return validateTextLinks(h.Text, h.Links)
}

// ListKind identifies the semantic ordering of a list.
type ListKind uint8

const (
	// ListKindUnordered identifies a bulleted list.
	ListKindUnordered ListKind = iota + 1
	// ListKindOrdered identifies a numbered list.
	ListKindOrdered
)

// List is a flat sequence of plain-text items.
type List struct {
	Kind  ListKind
	Start int
	Items []ListItem
}

func (*List) isBlock() {}

func (l *List) validate() error {
	if l == nil {
		return errors.New("list must not be nil")
	}
	switch l.Kind {
	case ListKindUnordered:
		if l.Start != 0 {
			return errors.New("unordered list start must be zero")
		}
	case ListKindOrdered:
		if l.Start < 0 || l.Start > maximumOrderedListMarker {
			return fmt.Errorf(
				"ordered list start must be between 0 and %d",
				maximumOrderedListMarker,
			)
		}
		if len(l.Items) > 0 &&
			len(l.Items)-1 > maximumOrderedListMarker-l.Start {
			return fmt.Errorf(
				"ordered list markers must not exceed %d",
				maximumOrderedListMarker,
			)
		}
	default:
		return fmt.Errorf("unsupported list kind %d", l.Kind)
	}
	if len(l.Items) == 0 {
		return errors.New("list must contain at least one item")
	}
	for index, item := range l.Items {
		if item.Text == "" {
			return fmt.Errorf("list item %d text must not be empty", index+1)
		}
		if err := validateTextLinks(item.Text, item.Links); err != nil {
			return fmt.Errorf("list item %d: %w", index+1, err)
		}
	}
	return nil
}

// ListItem is plain text with optional external links.
type ListItem struct {
	Text  string
	Links []TextLink
}

// TextLink identifies linked text by UTF-8 byte offsets.
type TextLink struct {
	Start       int
	End         int
	Destination string
}

func validateTextLinks(text string, links []TextLink) error {
	previousEnd := 0
	for index, link := range links {
		if link.Start < previousEnd || link.Start < 0 || link.End <= link.Start ||
			link.End > len(text) {
			return fmt.Errorf("link %d has invalid or overlapping text range", index+1)
		}
		if !utf8.RuneStart(text[link.Start]) ||
			link.End < len(text) && !utf8.RuneStart(text[link.End]) {
			return fmt.Errorf("link %d range must align with UTF-8 boundaries", index+1)
		}
		if strings.ContainsAny(text[link.Start:link.End], "\r\n") {
			return fmt.Errorf("link %d text must be a single line", index+1)
		}
		destination, err := url.Parse(link.Destination)
		if err != nil || !destination.IsAbs() {
			return fmt.Errorf("link %d destination must be an absolute HTTP, HTTPS, or mailto URI", index+1)
		}
		scheme := strings.ToLower(destination.Scheme)
		if scheme != "http" && scheme != "https" && scheme != "mailto" ||
			(scheme == "http" || scheme == "https") && destination.Host == "" ||
			scheme == "mailto" && destination.Opaque == "" {
			return fmt.Errorf("link %d destination must be an absolute HTTP, HTTPS, or mailto URI", index+1)
		}
		previousEnd = link.End
	}
	return nil
}
