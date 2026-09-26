package pdfium

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/document"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/extract"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/responses"
)

const (
	engineName     = "pdfium-wasm"
	fontFlagItalic = 1 << 6
)

var (
	errNilRuntime        = errors.New("PDFium runtime must not be nil")
	errNoPages           = errors.New("PDF contains no pages")
	errNilPageCount      = errors.New("PDFium returned no page count")
	errNilPageSize       = errors.New("PDFium returned no page size")
	errNilStructuredText = errors.New("PDFium returned no structured text")
	errNilCharacter      = errors.New("PDFium returned a nil text character")
	errMismatchedPage    = errors.New("PDFium returned data for an unexpected page")
)

// Extractor extracts physical text layout through PDFium WebAssembly.
type Extractor struct {
	runtime *Runtime
}

var _ extract.Extractor = (*Extractor)(nil)

// NewExtractor creates an extractor that borrows instances from runtime.
// The caller retains ownership of runtime and must close it after all
// extraction has finished.
func NewExtractor(runtime *Runtime) (*Extractor, error) {
	if runtime == nil {
		return nil, errNilRuntime
	}
	return &Extractor{runtime: runtime}, nil
}

// Name returns the stable engine identifier.
func (*Extractor) Name() string {
	return engineName
}

// Extract converts observed PDF text and geometry into the neutral layout
// model.
func (e *Extractor) Extract(
	ctx context.Context,
	source extract.Source,
) (*document.Layout, error) {
	if e == nil || e.runtime == nil {
		return nil, errNilRuntime
	}

	layout := &document.Layout{}
	err := e.runtime.withDocument(
		ctx,
		source,
		func(worker instance, pdf references.FPDF_DOCUMENT) error {
			return extractLayout(ctx, worker, pdf, layout)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("extract PDF layout: %w", err)
	}
	if err := layout.Validate(); err != nil {
		return nil, fmt.Errorf("validate extracted PDF layout: %w", err)
	}
	return layout, nil
}

func extractLayout(
	ctx context.Context,
	worker instance,
	pdf references.FPDF_DOCUMENT,
	layout *document.Layout,
) error {
	pageCount, err := worker.FPDF_GetPageCount(&requests.FPDF_GetPageCount{
		Document: pdf,
	})
	if err != nil {
		return fmt.Errorf("get page count: %w", err)
	}
	if pageCount == nil {
		return errNilPageCount
	}
	if pageCount.PageCount <= 0 {
		return errNoPages
	}

	layout.Pages = make([]document.Page, 0, pageCount.PageCount)
	for index := range pageCount.PageCount {
		if err := ctx.Err(); err != nil {
			return err
		}

		page, err := extractPage(worker, pdf, index)
		if err != nil {
			return fmt.Errorf("page %d: %w", index+1, err)
		}
		layout.Pages = append(layout.Pages, page)
	}
	return nil
}

func extractPage(
	worker instance,
	pdf references.FPDF_DOCUMENT,
	index int,
) (document.Page, error) {
	size, err := worker.FPDF_GetPageSizeByIndex(&requests.FPDF_GetPageSizeByIndex{
		Document: pdf,
		Index:    index,
	})
	if err != nil {
		return document.Page{}, fmt.Errorf("get size: %w", err)
	}
	if size == nil {
		return document.Page{}, errNilPageSize
	}
	if size.Page != index {
		return document.Page{}, fmt.Errorf(
			"%w: size page index is %d, requested %d",
			errMismatchedPage,
			size.Page,
			index,
		)
	}

	text, err := worker.GetPageTextStructured(&requests.GetPageTextStructured{
		Page: requests.Page{
			ByIndex: &requests.PageByIndex{
				Document: pdf,
				Index:    index,
			},
		},
		Mode:                   requests.GetPageTextStructuredModeChars,
		CollectFontInformation: true,
	})
	if err != nil {
		return document.Page{}, fmt.Errorf("extract structured text: %w", err)
	}
	if text == nil {
		return document.Page{}, errNilStructuredText
	}
	if text.Page != index {
		return document.Page{}, fmt.Errorf(
			"%w: text page index is %d, requested %d",
			errMismatchedPage,
			text.Page,
			index,
		)
	}

	runs := make([]document.TextRun, 0, len(text.Chars))
	for charIndex, char := range text.Chars {
		run, include, err := mapCharacter(size.Height, char)
		if err != nil {
			return document.Page{}, fmt.Errorf("character %d: %w", charIndex+1, err)
		}
		if include {
			runs = append(runs, run)
		}
	}

	return document.Page{
		Number:   index + 1,
		Width:    size.Width,
		Height:   size.Height,
		TextRuns: runs,
	}, nil
}

func mapCharacter(
	pageHeight float64,
	char *responses.GetPageTextStructuredChar,
) (document.TextRun, bool, error) {
	if char == nil {
		return document.TextRun{}, false, errNilCharacter
	}
	if char.Text == "" {
		return document.TextRun{}, false, nil
	}

	position := char.PointPosition
	run := document.TextRun{
		Text: char.Text,
		Bounds: document.Rectangle{
			Left:   position.Left,
			Top:    pageHeight - position.Top,
			Right:  position.Right,
			Bottom: pageHeight - position.Bottom,
		},
		RotationDegrees: char.Angle * 180 / math.Pi,
	}

	if char.FontInformation != nil {
		fontSize := char.FontInformation.RenderedSize
		if fontSize <= 0 {
			fontSize = char.FontInformation.Size
		}
		fontWeight := char.FontInformation.Weight
		if fontWeight < 0 {
			fontWeight = 0
		}
		run.Style = document.TextStyle{
			FontName:   char.FontInformation.Name,
			FontSize:   fontSize,
			FontWeight: fontWeight,
			Italic:     char.FontInformation.Flags&fontFlagItalic != 0,
		}
	}

	if err := run.Validate(); err != nil {
		return document.TextRun{}, false, err
	}
	return run, true, nil
}
