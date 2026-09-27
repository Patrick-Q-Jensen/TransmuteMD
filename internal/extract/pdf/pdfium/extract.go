package pdfium

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/document"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/extract"
	"github.com/klippa-app/go-pdfium/enums"
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
	errNilAnnotCount     = errors.New("PDFium returned no annotation count")
	errNilAnnotation     = errors.New("PDFium returned no annotation")
	errNilAnnotSubtype   = errors.New("PDFium returned no annotation subtype")
	errNilAnnotLink      = errors.New("PDFium returned no annotation link")
	errNilLinkAction     = errors.New("PDFium returned no link action")
	errNilActionType     = errors.New("PDFium returned no action type")
	errNilAnnotRect      = errors.New("PDFium returned no annotation rectangle")
	errMismatchedPage    = errors.New("PDFium returned data for an unexpected page")
)

// Extractor extracts physical text layout through PDFium WebAssembly.
type Extractor struct {
	runtime *Runtime
	limits  extract.Limits
}

var _ extract.Extractor = (*Extractor)(nil)

// NewExtractor creates an extractor that borrows instances from runtime.
// The caller retains ownership of runtime and must close it after all
// extraction has finished.
func NewExtractor(runtime *Runtime) (*Extractor, error) {
	return NewExtractorWithLimits(runtime, extract.DefaultLimits())
}

// NewExtractorWithLimits creates an extractor with explicit safety limits.
func NewExtractorWithLimits(
	runtime *Runtime,
	limits extract.Limits,
) (*Extractor, error) {
	if runtime == nil {
		return nil, errNilRuntime
	}
	if err := limits.Validate(); err != nil {
		return nil, fmt.Errorf("validate PDF extraction limits: %w", err)
	}
	return &Extractor{runtime: runtime, limits: limits}, nil
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
	if source != nil && source.Size() > e.limits.MaxSourceBytes {
		return nil, extract.LimitError(
			"source bytes",
			source.Size(),
			e.limits.MaxSourceBytes,
		)
	}

	layout := &document.Layout{}
	err := e.runtime.withDocument(
		ctx,
		source,
		func(worker instance, pdf references.FPDF_DOCUMENT) error {
			return extractLayout(ctx, worker, pdf, layout, e.limits)
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
	limits extract.Limits,
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
		return fmt.Errorf("%w: %w", extract.ErrInvalidDocument, errNoPages)
	}
	if pageCount.PageCount > limits.MaxPages {
		return extract.LimitError(
			"pages",
			int64(pageCount.PageCount),
			int64(limits.MaxPages),
		)
	}

	layout.Pages = make([]document.Page, 0, pageCount.PageCount)
	counts := extractionCounts{}
	for index := range pageCount.PageCount {
		if err := ctx.Err(); err != nil {
			return err
		}

		page, diagnostics, err := extractPage(
			worker,
			pdf,
			index,
			limits,
			&counts,
		)
		if err != nil {
			return fmt.Errorf("page %d: %w", index+1, err)
		}
		layout.Pages = append(layout.Pages, page)
		layout.Diagnostics = append(layout.Diagnostics, diagnostics...)
	}
	return nil
}

type extractionCounts struct {
	textRuns    int
	annotations int
}

func extractPage(
	worker instance,
	pdf references.FPDF_DOCUMENT,
	index int,
	limits extract.Limits,
	counts *extractionCounts,
) (document.Page, []document.Diagnostic, error) {
	size, err := worker.FPDF_GetPageSizeByIndex(&requests.FPDF_GetPageSizeByIndex{
		Document: pdf,
		Index:    index,
	})
	if err != nil {
		return document.Page{}, nil, fmt.Errorf("get size: %w", err)
	}
	if size == nil {
		return document.Page{}, nil, errNilPageSize
	}
	if size.Page != index {
		return document.Page{}, nil, fmt.Errorf(
			"%w: size page index is %d, requested %d",
			errMismatchedPage,
			size.Page,
			index,
		)
	}
	if size.Width > float64(limits.MaxPageDimension) ||
		size.Height > float64(limits.MaxPageDimension) {
		return document.Page{}, nil, extract.LimitError(
			"page dimension points",
			int64(math.Ceil(math.Max(size.Width, size.Height))),
			int64(limits.MaxPageDimension),
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
		return document.Page{}, nil, fmt.Errorf("extract structured text: %w", err)
	}
	if text == nil {
		return document.Page{}, nil, errNilStructuredText
	}
	if text.Page != index {
		return document.Page{}, nil, fmt.Errorf(
			"%w: text page index is %d, requested %d",
			errMismatchedPage,
			text.Page,
			index,
		)
	}
	if len(text.Chars) > limits.MaxTextRuns-counts.textRuns {
		return document.Page{}, nil, extract.LimitError(
			"text runs",
			int64(counts.textRuns+len(text.Chars)),
			int64(limits.MaxTextRuns),
		)
	}
	counts.textRuns += len(text.Chars)

	runs := make([]document.TextRun, 0, len(text.Chars))
	for charIndex, char := range text.Chars {
		run, include, err := mapCharacter(size.Height, char)
		if err != nil {
			return document.Page{}, nil, fmt.Errorf("character %d: %w", charIndex+1, err)
		}
		if include {
			runs = append(runs, run)
		}
	}

	links, diagnostics, err := extractPageLinks(
		worker,
		pdf,
		index,
		size.Height,
		limits,
		counts,
	)
	if err != nil {
		return document.Page{}, nil, fmt.Errorf("extract links: %w", err)
	}

	return document.Page{
		Number:   index + 1,
		Width:    size.Width,
		Height:   size.Height,
		TextRuns: runs,
		Links:    links,
	}, diagnostics, nil
}

func extractPageLinks(
	worker instance,
	pdf references.FPDF_DOCUMENT,
	index int,
	pageHeight float64,
	limits extract.Limits,
	counts *extractionCounts,
) ([]document.LinkAnnotation, []document.Diagnostic, error) {
	page := requests.Page{ByIndex: &requests.PageByIndex{
		Document: pdf,
		Index:    index,
	}}
	count, err := worker.FPDFPage_GetAnnotCount(&requests.FPDFPage_GetAnnotCount{
		Page: page,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("get annotation count: %w", err)
	}
	if count == nil {
		return nil, nil, errNilAnnotCount
	}
	if count.Count > limits.MaxAnnotations-counts.annotations {
		return nil, nil, extract.LimitError(
			"annotations",
			int64(counts.annotations+count.Count),
			int64(limits.MaxAnnotations),
		)
	}
	counts.annotations += count.Count

	links := make([]document.LinkAnnotation, 0)
	diagnostics := make([]document.Diagnostic, 0)
	for annotationIndex := range count.Count {
		response, err := worker.FPDFPage_GetAnnot(&requests.FPDFPage_GetAnnot{
			Page:  page,
			Index: annotationIndex,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("annotation %d: get: %w", annotationIndex+1, err)
		}
		if response == nil || response.Annotation == "" {
			return nil, nil, fmt.Errorf("annotation %d: %w", annotationIndex+1, errNilAnnotation)
		}

		link, include, omission, linkErr := extractAnnotationLink(
			worker,
			pdf,
			response.Annotation,
			pageHeight,
		)
		_, closeErr := worker.FPDFPage_CloseAnnot(&requests.FPDFPage_CloseAnnot{
			Annotation: response.Annotation,
		})
		if closeErr != nil {
			linkErr = errors.Join(linkErr, fmt.Errorf("close: %w", closeErr))
		}
		if linkErr != nil {
			return nil, nil, fmt.Errorf("annotation %d: %w", annotationIndex+1, linkErr)
		}
		if include {
			links = append(links, link)
		}
		if omission != "" {
			diagnostics = append(diagnostics, document.Diagnostic{
				Code:    document.DiagnosticUnsupportedLink,
				Page:    index + 1,
				Message: omission,
			})
		}
	}
	return links, diagnostics, nil
}

func extractAnnotationLink(
	worker instance,
	pdf references.FPDF_DOCUMENT,
	annotation references.FPDF_ANNOTATION,
	pageHeight float64,
) (document.LinkAnnotation, bool, string, error) {
	subtype, err := worker.FPDFAnnot_GetSubtype(&requests.FPDFAnnot_GetSubtype{
		Annotation: annotation,
	})
	if err != nil {
		return document.LinkAnnotation{}, false, "", fmt.Errorf("get subtype: %w", err)
	}
	if subtype == nil {
		return document.LinkAnnotation{}, false, "", errNilAnnotSubtype
	}
	if subtype.Subtype != enums.FPDF_ANNOT_SUBTYPE_LINK {
		return document.LinkAnnotation{}, false, "", nil
	}

	link, err := worker.FPDFAnnot_GetLink(&requests.FPDFAnnot_GetLink{
		Annotation: annotation,
	})
	if err != nil {
		return document.LinkAnnotation{}, false, "", fmt.Errorf("get link: %w", err)
	}
	if link == nil || link.Link == "" {
		return document.LinkAnnotation{}, false, "", errNilAnnotLink
	}
	action, err := worker.FPDFLink_GetAction(&requests.FPDFLink_GetAction{
		Link: link.Link,
	})
	if err != nil {
		return document.LinkAnnotation{}, false, "", fmt.Errorf("get action: %w", err)
	}
	if action == nil {
		return document.LinkAnnotation{}, false, "", errNilLinkAction
	}
	if action.Action == nil {
		return document.LinkAnnotation{}, false,
			"link without a supported external action was preserved as text", nil
	}
	actionType, err := worker.FPDFAction_GetType(&requests.FPDFAction_GetType{
		Action: *action.Action,
	})
	if err != nil {
		return document.LinkAnnotation{}, false, "", fmt.Errorf("get action type: %w", err)
	}
	if actionType == nil {
		return document.LinkAnnotation{}, false, "", errNilActionType
	}
	if actionType.Type != enums.FPDF_ACTION_ACTION_URI {
		return document.LinkAnnotation{}, false,
			"non-URI link target was preserved as text", nil
	}
	uri, err := worker.FPDFAction_GetURIPath(&requests.FPDFAction_GetURIPath{
		Document: pdf,
		Action:   *action.Action,
	})
	if err != nil {
		return document.LinkAnnotation{}, false, "", fmt.Errorf("get URI: %w", err)
	}
	if uri == nil {
		return document.LinkAnnotation{}, false, "", errors.New("PDFium returned no action URI")
	}
	if uri.URIPath == nil || !isReliableExternalURI(*uri.URIPath) {
		return document.LinkAnnotation{}, false,
			"unsupported or unsafe link URI was preserved as text", nil
	}
	rect, err := worker.FPDFAnnot_GetRect(&requests.FPDFAnnot_GetRect{
		Annotation: annotation,
	})
	if err != nil {
		return document.LinkAnnotation{}, false, "", fmt.Errorf("get rectangle: %w", err)
	}
	if rect == nil {
		return document.LinkAnnotation{}, false, "", errNilAnnotRect
	}

	result := document.LinkAnnotation{
		Bounds: document.Rectangle{
			Left:   float64(rect.Rect.Left),
			Top:    pageHeight - float64(rect.Rect.Top),
			Right:  float64(rect.Rect.Right),
			Bottom: pageHeight - float64(rect.Rect.Bottom),
		},
		Destination: strings.TrimSpace(*uri.URIPath),
	}
	if err := result.Validate(); err != nil {
		return document.LinkAnnotation{}, false, "", err
	}
	return result, true, "", nil
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
