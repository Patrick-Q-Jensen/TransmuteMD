package document

import (
	"errors"
	"fmt"
	"strings"
)

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
