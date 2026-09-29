package document

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"
)

// Layout contains the observed physical content of a source document.
type Layout struct {
	Pages       []Page
	Diagnostics []Diagnostic
}

// Validate checks the invariants required by layout analysis.
func (l Layout) Validate() error {
	for i, page := range l.Pages {
		if page.Number != i+1 {
			return fmt.Errorf("page %d: number must be %d, got %d", i+1, i+1, page.Number)
		}
		if err := page.Validate(); err != nil {
			return fmt.Errorf("page %d: %w", page.Number, err)
		}
		for linkIndex, link := range page.Links {
			if link.Target.Kind == LinkTargetPage &&
				link.Target.Page > len(l.Pages) {
				return fmt.Errorf(
					"page %d: link %d: target page must not exceed document page count %d",
					page.Number,
					linkIndex+1,
					len(l.Pages),
				)
			}
		}
	}
	return validateDiagnostics(l.Diagnostics)
}

// Page contains the physical text observed on one source page. Dimensions are
// measured in points.
type Page struct {
	Number           int
	Width            float64
	Height           float64
	TextRuns         []TextRun
	TextPlaceholders []TextPlaceholder
	Links            []LinkAnnotation
	Rulings          []Ruling
}

// Validate checks page dimensions and text runs.
func (p Page) Validate() error {
	if !isPositiveFinite(p.Width) {
		return errors.New("width must be finite and greater than zero")
	}
	if !isPositiveFinite(p.Height) {
		return errors.New("height must be finite and greater than zero")
	}
	for i, run := range p.TextRuns {
		if err := run.Validate(); err != nil {
			return fmt.Errorf("text run %d: %w", i+1, err)
		}
	}
	for i, placeholder := range p.TextPlaceholders {
		if err := placeholder.Validate(); err != nil {
			return fmt.Errorf("text placeholder %d: %w", i+1, err)
		}
	}
	for i, link := range p.Links {
		if err := link.Validate(); err != nil {
			return fmt.Errorf("link %d: %w", i+1, err)
		}
	}
	for i, ruling := range p.Rulings {
		if err := ruling.Validate(); err != nil {
			return fmt.Errorf("ruling %d: %w", i+1, err)
		}
	}
	return nil
}

// TextPlaceholder is an authored text position without extractable content.
// Analysis may use its placement as supporting evidence for empty fields.
type TextPlaceholder struct {
	Position Point
	FontSize float64
}

// Validate checks that a text placeholder has usable physical placement.
func (p TextPlaceholder) Validate() error {
	if err := p.Position.Validate(); err != nil {
		return fmt.Errorf("position: %w", err)
	}
	if !isPositiveFinite(p.FontSize) {
		return errors.New("font size must be finite and greater than zero")
	}
	return nil
}

// Ruling is a visible horizontal or vertical path edge observed on a page.
type Ruling struct {
	Start Point
	End   Point
	Width float64
}

// Validate checks that a ruling is finite, axis-aligned, and non-empty.
func (r Ruling) Validate() error {
	if err := r.Start.Validate(); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	if err := r.End.Validate(); err != nil {
		return fmt.Errorf("end: %w", err)
	}
	if !isFinite(r.Width) || r.Width < 0 {
		return errors.New("width must be finite and non-negative")
	}
	if r.Start == r.End {
		return errors.New("must have non-zero length")
	}
	if r.Start.X != r.End.X && r.Start.Y != r.End.Y {
		return errors.New("must be horizontal or vertical")
	}
	return nil
}

// Point is a position in page coordinates with a top-left origin.
type Point struct {
	X float64
	Y float64
}

// Validate checks that both coordinates are finite.
func (p Point) Validate() error {
	if !isFinite(p.X) || !isFinite(p.Y) {
		return errors.New("coordinates must be finite")
	}
	return nil
}

// LinkAnnotation is a reliable link observed in the source layout.
type LinkAnnotation struct {
	Bounds Rectangle
	Target LinkTarget
}

// Validate checks link annotation geometry and destination.
func (l LinkAnnotation) Validate() error {
	if err := l.Bounds.Validate(); err != nil {
		return fmt.Errorf("bounds: %w", err)
	}
	if err := l.Target.Validate(); err != nil {
		return fmt.Errorf("target: %w", err)
	}
	return nil
}

// LinkTargetKind identifies the destination represented by a link target.
type LinkTargetKind uint8

const (
	// LinkTargetExternal identifies an absolute HTTP, HTTPS, or mailto URI.
	LinkTargetExternal LinkTargetKind = iota + 1
	// LinkTargetPage identifies a one-based page in the current document.
	LinkTargetPage
	// LinkTargetNamed identifies a named destination in the current document.
	LinkTargetNamed
)

// LinkTarget is an engine-neutral external or intra-document destination.
type LinkTarget struct {
	Kind LinkTargetKind
	URI  string
	Page int
	Name string
}

// Validate checks that exactly the fields required by the target kind are set.
func (t LinkTarget) Validate() error {
	switch t.Kind {
	case LinkTargetExternal:
		if t.Page != 0 ||
			t.Name != "" ||
			t.URI != strings.TrimSpace(t.URI) ||
			!isReliableExternalURI(t.URI) {
			return errors.New("external target must contain only an absolute HTTP, HTTPS, or mailto URI")
		}
	case LinkTargetPage:
		if t.URI != "" || t.Name != "" || t.Page < 1 {
			return errors.New("page target must contain only a positive one-based page number")
		}
	case LinkTargetNamed:
		if t.URI != "" || t.Page != 0 || strings.TrimSpace(t.Name) == "" {
			return errors.New("named target must contain only a non-empty name")
		}
	default:
		return fmt.Errorf("unsupported target kind %d", t.Kind)
	}
	return nil
}

func isReliableExternalURI(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || !parsed.IsAbs() {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return parsed.Host != ""
	case "mailto":
		return parsed.Opaque != ""
	default:
		return false
	}
}

// TextRun is consecutive text that shares physical placement and style
// evidence. Runs retain extraction order; analysis determines reading order.
type TextRun struct {
	Text            string
	Bounds          Rectangle
	Style           TextStyle
	RotationDegrees float64
}

// Validate checks that a text run is usable by layout analysis.
func (r TextRun) Validate() error {
	if r.Text == "" {
		return errors.New("text must not be empty")
	}
	if err := r.Bounds.Validate(); err != nil {
		return fmt.Errorf("bounds: %w", err)
	}
	if err := r.Style.Validate(); err != nil {
		return fmt.Errorf("style: %w", err)
	}
	if !isFinite(r.RotationDegrees) {
		return errors.New("rotation must be finite")
	}
	return nil
}

// Rectangle describes a box in points using a top-left origin. X increases to
// the right and Y increases downward.
type Rectangle struct {
	Left   float64
	Top    float64
	Right  float64
	Bottom float64
}

// Width returns the rectangle width in points.
func (r Rectangle) Width() float64 {
	return r.Right - r.Left
}

// Height returns the rectangle height in points.
func (r Rectangle) Height() float64 {
	return r.Bottom - r.Top
}

// Validate checks that all coordinates are finite and ordered.
func (r Rectangle) Validate() error {
	if !isFinite(r.Left) || !isFinite(r.Top) || !isFinite(r.Right) || !isFinite(r.Bottom) {
		return errors.New("coordinates must be finite")
	}
	if r.Right < r.Left {
		return errors.New("right must not be less than left")
	}
	if r.Bottom < r.Top {
		return errors.New("bottom must not be less than top")
	}
	return nil
}

// TextStyle contains engine-neutral style evidence observed during extraction.
// Empty or zero values mean that the extractor could not determine a property.
type TextStyle struct {
	FontName   string
	FontSize   float64
	FontWeight int
	Italic     bool
}

// Validate checks numeric style properties.
func (s TextStyle) Validate() error {
	if !isFinite(s.FontSize) || s.FontSize < 0 {
		return errors.New("font size must be finite and non-negative")
	}
	if s.FontWeight < 0 {
		return errors.New("font weight must be non-negative")
	}
	return nil
}

func isPositiveFinite(value float64) bool {
	return value > 0 && isFinite(value)
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
