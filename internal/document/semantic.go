package document

import (
	"errors"
	"fmt"
	"strings"
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
	Text string
}

func (*Paragraph) isBlock() {}

func (p *Paragraph) validate() error {
	if p == nil {
		return errors.New("paragraph must not be nil")
	}
	if p.Text == "" {
		return errors.New("paragraph text must not be empty")
	}
	return nil
}

// Heading is a section title with a Markdown-compatible level from 1 to 6.
type Heading struct {
	Level int
	Text  string
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
	return nil
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
	Items []string
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
		if item == "" {
			return fmt.Errorf("list item %d text must not be empty", index+1)
		}
	}
	return nil
}
