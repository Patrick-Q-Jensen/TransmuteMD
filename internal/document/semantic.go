package document

import (
	"errors"
	"fmt"
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
