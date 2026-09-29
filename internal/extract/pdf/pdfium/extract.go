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
	engineName           = "pdfium-wasm"
	fontFlagItalic       = 1 << 6
	rulingAxisTolerance  = 0.25
	placeholderTolerance = 0.01
)

var (
	errNilRuntime        = errors.New("PDFium runtime must not be nil")
	errNoPages           = errors.New("PDF contains no pages")
	errNilPageCount      = errors.New("PDFium returned no page count")
	errNilPageSize       = errors.New("PDFium returned no page size")
	errNilStructuredText = errors.New("PDFium returned no structured text")
	errNilObjectCount    = errors.New("PDFium returned no page object count")
	errNilPageObject     = errors.New("PDFium returned no page object")
	errNilPageObjectType = errors.New("PDFium returned no page object type")
	errNilObjectBounds   = errors.New("PDFium returned no page object bounds")
	errNilObjectMatrix   = errors.New("PDFium returned no page object matrix")
	errNilFontSize       = errors.New("PDFium returned no text object font size")
	errNilPathDrawMode   = errors.New("PDFium returned no path draw mode")
	errNilSegmentCount   = errors.New("PDFium returned no path segment count")
	errNilPathSegment    = errors.New("PDFium returned no path segment")
	errNilSegmentType    = errors.New("PDFium returned no path segment type")
	errNilSegmentPoint   = errors.New("PDFium returned no path segment point")
	errNilSegmentClose   = errors.New("PDFium returned no path segment close state")
	errNilStrokeWidth    = errors.New("PDFium returned no path stroke width")
	errNilCharacter      = errors.New("PDFium returned a nil text character")
	errNilAnnotCount     = errors.New("PDFium returned no annotation count")
	errNilAnnotation     = errors.New("PDFium returned no annotation")
	errNilAnnotSubtype   = errors.New("PDFium returned no annotation subtype")
	errNilAnnotLink      = errors.New("PDFium returned no annotation link")
	errNilLinkAction     = errors.New("PDFium returned no link action")
	errNilActionType     = errors.New("PDFium returned no action type")
	errNilDestPageIndex  = errors.New("PDFium returned no destination page index")
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
	textRuns     int
	annotations  int
	pageObjects  int
	pathSegments int
	rulings      int
}

type linkOmission uint8

const (
	linkOmissionMissingAction linkOmission = iota + 1
	linkOmissionUnresolvedInternal
	linkOmissionUnsafeURI
	linkOmissionUnsupportedAction
)

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

	rulings, placeholders, err := extractPageGeometry(
		worker,
		pdf,
		index,
		size.Height,
		limits,
		counts,
	)
	if err != nil {
		return document.Page{}, nil, fmt.Errorf("extract rulings: %w", err)
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
		Number:           index + 1,
		Width:            size.Width,
		Height:           size.Height,
		TextRuns:         runs,
		TextPlaceholders: placeholders,
		Links:            links,
		Rulings:          rulings,
	}, diagnostics, nil
}

type affineMatrix struct {
	a float64
	b float64
	c float64
	d float64
	e float64
	f float64
}

func identityMatrix() affineMatrix {
	return affineMatrix{a: 1, d: 1}
}

func composeMatrices(parent, child affineMatrix) affineMatrix {
	return affineMatrix{
		a: parent.a*child.a + parent.c*child.b,
		b: parent.b*child.a + parent.d*child.b,
		c: parent.a*child.c + parent.c*child.d,
		d: parent.b*child.c + parent.d*child.d,
		e: parent.a*child.e + parent.c*child.f + parent.e,
		f: parent.b*child.e + parent.d*child.f + parent.f,
	}
}

func extractPageGeometry(
	worker instance,
	pdf references.FPDF_DOCUMENT,
	index int,
	pageHeight float64,
	limits extract.Limits,
	counts *extractionCounts,
) ([]document.Ruling, []document.TextPlaceholder, error) {
	page := requests.Page{ByIndex: &requests.PageByIndex{
		Document: pdf,
		Index:    index,
	}}
	response, err := worker.FPDFPage_CountObjects(&requests.FPDFPage_CountObjects{
		Page: page,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("count page objects: %w", err)
	}
	if response == nil {
		return nil, nil, errNilObjectCount
	}
	if response.Count < 0 {
		return nil, nil, fmt.Errorf("page object count must not be negative: %d", response.Count)
	}
	if response.Count > limits.MaxPageObjects-counts.pageObjects {
		return nil, nil, extract.LimitError(
			"page objects",
			int64(counts.pageObjects+response.Count),
			int64(limits.MaxPageObjects),
		)
	}
	counts.pageObjects += response.Count

	var rulings []document.Ruling
	var placeholders []document.TextPlaceholder
	for objectIndex := range response.Count {
		object, err := worker.FPDFPage_GetObject(&requests.FPDFPage_GetObject{
			Page:  page,
			Index: objectIndex,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("page object %d: get: %w", objectIndex+1, err)
		}
		if object == nil || object.PageObject == "" {
			return nil, nil, fmt.Errorf("page object %d: %w", objectIndex+1, errNilPageObject)
		}
		objectRulings, err := extractObjectRulings(
			worker,
			object.PageObject,
			identityMatrix(),
			pageHeight,
			limits,
			counts,
			&placeholders,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("page object %d: %w", objectIndex+1, err)
		}
		rulings = append(rulings, objectRulings...)
	}
	return rulings, placeholders, nil
}

func extractObjectRulings(
	worker instance,
	object references.FPDF_PAGEOBJECT,
	parentMatrix affineMatrix,
	pageHeight float64,
	limits extract.Limits,
	counts *extractionCounts,
	placeholders *[]document.TextPlaceholder,
) ([]document.Ruling, error) {
	objectType, err := worker.FPDFPageObj_GetType(&requests.FPDFPageObj_GetType{
		PageObject: object,
	})
	if err != nil {
		return nil, fmt.Errorf("get type: %w", err)
	}
	if objectType == nil {
		return nil, errNilPageObjectType
	}
	if objectType.Type == enums.FPDF_PAGEOBJ_TEXT {
		placeholder, ok, err := extractTextPlaceholder(
			worker,
			object,
			parentMatrix,
			pageHeight,
		)
		if err != nil {
			return nil, err
		}
		if ok {
			*placeholders = append(*placeholders, placeholder)
		}
		return nil, nil
	}
	if objectType.Type == enums.FPDF_PAGEOBJ_FORM {
		return extractFormRulings(
			worker,
			object,
			parentMatrix,
			pageHeight,
			limits,
			counts,
			placeholders,
		)
	}
	if objectType.Type != enums.FPDF_PAGEOBJ_PATH {
		return nil, nil
	}

	drawMode, err := worker.FPDFPath_GetDrawMode(&requests.FPDFPath_GetDrawMode{
		PageObject: object,
	})
	if err != nil {
		return nil, fmt.Errorf("get draw mode: %w", err)
	}
	if drawMode == nil {
		return nil, errNilPathDrawMode
	}
	if !drawMode.Stroke && drawMode.FillMode == enums.FPDF_FILLMODE_NONE {
		return nil, nil
	}
	matrixResponse, err := worker.FPDFPageObj_GetMatrix(&requests.FPDFPageObj_GetMatrix{
		PageObject: object,
	})
	if err != nil {
		return nil, fmt.Errorf("get matrix: %w", err)
	}
	if matrixResponse == nil {
		return nil, errNilObjectMatrix
	}
	matrix := composeMatrices(parentMatrix, affineMatrix{
		a: float64(matrixResponse.Matrix.A),
		b: float64(matrixResponse.Matrix.B),
		c: float64(matrixResponse.Matrix.C),
		d: float64(matrixResponse.Matrix.D),
		e: float64(matrixResponse.Matrix.E),
		f: float64(matrixResponse.Matrix.F),
	})

	stroke, err := worker.FPDFPageObj_GetStrokeWidth(&requests.FPDFPageObj_GetStrokeWidth{
		PageObject: object,
	})
	if err != nil {
		return nil, fmt.Errorf("get stroke width: %w", err)
	}
	if stroke == nil {
		return nil, errNilStrokeWidth
	}

	count, err := worker.FPDFPath_CountSegments(&requests.FPDFPath_CountSegments{
		PageObject: object,
	})
	if err != nil {
		return nil, fmt.Errorf("count segments: %w", err)
	}
	if count == nil {
		return nil, errNilSegmentCount
	}
	if count.Count < 0 {
		return nil, fmt.Errorf("path segment count must not be negative: %d", count.Count)
	}
	if count.Count > limits.MaxPathSegments-counts.pathSegments {
		return nil, extract.LimitError(
			"path segments",
			int64(counts.pathSegments+count.Count),
			int64(limits.MaxPathSegments),
		)
	}
	counts.pathSegments += count.Count

	var (
		rulings        []document.Ruling
		previous       document.Point
		subpathStart   document.Point
		havePrevious   bool
		haveSubpath    bool
		transformedWid = transformedStrokeWidth(matrix, float64(stroke.StrokeWidth))
	)
	for segmentIndex := range count.Count {
		segment, err := worker.FPDFPath_GetPathSegment(&requests.FPDFPath_GetPathSegment{
			PageObject: object,
			Index:      segmentIndex,
		})
		if err != nil {
			return nil, fmt.Errorf("segment %d: get: %w", segmentIndex+1, err)
		}
		if segment == nil || segment.PathSegment == "" {
			return nil, fmt.Errorf("segment %d: %w", segmentIndex+1, errNilPathSegment)
		}
		segmentType, err := worker.FPDFPathSegment_GetType(
			&requests.FPDFPathSegment_GetType{PathSegment: segment.PathSegment},
		)
		if err != nil {
			return nil, fmt.Errorf("segment %d: get type: %w", segmentIndex+1, err)
		}
		if segmentType == nil {
			return nil, fmt.Errorf("segment %d: %w", segmentIndex+1, errNilSegmentType)
		}
		pointResponse, err := worker.FPDFPathSegment_GetPoint(
			&requests.FPDFPathSegment_GetPoint{PathSegment: segment.PathSegment},
		)
		if err != nil {
			return nil, fmt.Errorf("segment %d: get point: %w", segmentIndex+1, err)
		}
		if pointResponse == nil {
			return nil, fmt.Errorf("segment %d: %w", segmentIndex+1, errNilSegmentPoint)
		}
		point := transformPoint(
			matrix,
			float64(pointResponse.X),
			float64(pointResponse.Y),
			pageHeight,
		)

		switch segmentType.Type {
		case enums.FPDF_SEGMENT_MOVETO:
			previous = point
			subpathStart = point
			havePrevious = true
			haveSubpath = true
		case enums.FPDF_SEGMENT_LINETO:
			if havePrevious {
				rulings, err = appendRuling(
					rulings,
					previous,
					point,
					transformedWid,
					limits,
					counts,
				)
				if err != nil {
					return nil, err
				}
			}
			previous = point
			havePrevious = true
		default:
			previous = point
			havePrevious = true
		}

		closeResponse, err := worker.FPDFPathSegment_GetClose(
			&requests.FPDFPathSegment_GetClose{PathSegment: segment.PathSegment},
		)
		if err != nil {
			return nil, fmt.Errorf("segment %d: get close state: %w", segmentIndex+1, err)
		}
		if closeResponse == nil {
			return nil, fmt.Errorf("segment %d: %w", segmentIndex+1, errNilSegmentClose)
		}
		if closeResponse.IsClose && havePrevious && haveSubpath {
			rulings, err = appendRuling(
				rulings,
				previous,
				subpathStart,
				transformedWid,
				limits,
				counts,
			)
			if err != nil {
				return nil, err
			}
			previous = subpathStart
		}
	}
	return rulings, nil
}

func extractTextPlaceholder(
	worker instance,
	object references.FPDF_PAGEOBJECT,
	parentMatrix affineMatrix,
	pageHeight float64,
) (document.TextPlaceholder, bool, error) {
	bounds, err := worker.FPDFPageObj_GetBounds(&requests.FPDFPageObj_GetBounds{
		PageObject: object,
	})
	if err != nil {
		return document.TextPlaceholder{}, false, fmt.Errorf("get bounds: %w", err)
	}
	if bounds == nil {
		return document.TextPlaceholder{}, false, errNilObjectBounds
	}
	if math.Abs(float64(bounds.Right-bounds.Left)) > placeholderTolerance ||
		math.Abs(float64(bounds.Top-bounds.Bottom)) > placeholderTolerance {
		return document.TextPlaceholder{}, false, nil
	}
	fontSize, err := worker.FPDFTextObj_GetFontSize(&requests.FPDFTextObj_GetFontSize{
		PageObject: object,
	})
	if err != nil {
		return document.TextPlaceholder{}, false, fmt.Errorf("get font size: %w", err)
	}
	if fontSize == nil {
		return document.TextPlaceholder{}, false, errNilFontSize
	}
	scaleX := math.Hypot(parentMatrix.a, parentMatrix.b)
	scaleY := math.Hypot(parentMatrix.c, parentMatrix.d)
	size := float64(fontSize.FontSize) * (scaleX + scaleY) / 2
	if size <= 0 || math.IsNaN(size) || math.IsInf(size, 0) {
		return document.TextPlaceholder{}, false, nil
	}
	return document.TextPlaceholder{
		Position: transformPoint(
			parentMatrix,
			float64(bounds.Left+bounds.Right)/2,
			float64(bounds.Bottom+bounds.Top)/2,
			pageHeight,
		),
		FontSize: size,
	}, true, nil
}

func extractFormRulings(
	worker instance,
	object references.FPDF_PAGEOBJECT,
	parentMatrix affineMatrix,
	pageHeight float64,
	limits extract.Limits,
	counts *extractionCounts,
	placeholders *[]document.TextPlaceholder,
) ([]document.Ruling, error) {
	matrixResponse, err := worker.FPDFPageObj_GetMatrix(&requests.FPDFPageObj_GetMatrix{
		PageObject: object,
	})
	if err != nil {
		return nil, fmt.Errorf("get matrix: %w", err)
	}
	if matrixResponse == nil {
		return nil, errNilObjectMatrix
	}
	matrix := composeMatrices(parentMatrix, affineMatrix{
		a: float64(matrixResponse.Matrix.A),
		b: float64(matrixResponse.Matrix.B),
		c: float64(matrixResponse.Matrix.C),
		d: float64(matrixResponse.Matrix.D),
		e: float64(matrixResponse.Matrix.E),
		f: float64(matrixResponse.Matrix.F),
	})

	response, err := worker.FPDFFormObj_CountObjects(&requests.FPDFFormObj_CountObjects{
		PageObject: object,
	})
	if err != nil {
		return nil, fmt.Errorf("count child objects: %w", err)
	}
	if response == nil {
		return nil, errNilObjectCount
	}
	if response.Count < 0 {
		return nil, fmt.Errorf("child object count must not be negative: %d", response.Count)
	}
	if response.Count > limits.MaxPageObjects-counts.pageObjects {
		return nil, extract.LimitError(
			"page objects",
			int64(counts.pageObjects+response.Count),
			int64(limits.MaxPageObjects),
		)
	}
	counts.pageObjects += response.Count

	var rulings []document.Ruling
	for childIndex := range response.Count {
		child, err := worker.FPDFFormObj_GetObject(&requests.FPDFFormObj_GetObject{
			PageObject: object,
			Index:      uint64(childIndex),
		})
		if err != nil {
			return nil, fmt.Errorf("child object %d: get: %w", childIndex+1, err)
		}
		if child == nil || child.PageObject == "" {
			return nil, fmt.Errorf("child object %d: %w", childIndex+1, errNilPageObject)
		}
		childRulings, err := extractObjectRulings(
			worker,
			child.PageObject,
			matrix,
			pageHeight,
			limits,
			counts,
			placeholders,
		)
		if err != nil {
			return nil, fmt.Errorf("child object %d: %w", childIndex+1, err)
		}
		rulings = append(rulings, childRulings...)
	}
	return rulings, nil
}

func appendRuling(
	rulings []document.Ruling,
	start document.Point,
	end document.Point,
	width float64,
	limits extract.Limits,
	counts *extractionCounts,
) ([]document.Ruling, error) {
	ruling, ok := normalizedRuling(start, end, width)
	if !ok {
		return rulings, nil
	}
	if counts.rulings >= limits.MaxRulings {
		return nil, extract.LimitError(
			"rulings",
			int64(counts.rulings+1),
			int64(limits.MaxRulings),
		)
	}
	counts.rulings++
	return append(rulings, ruling), nil
}

func normalizedRuling(
	start document.Point,
	end document.Point,
	width float64,
) (document.Ruling, bool) {
	if math.Hypot(end.X-start.X, end.Y-start.Y) <= rulingAxisTolerance {
		return document.Ruling{}, false
	}
	switch {
	case math.Abs(start.Y-end.Y) <= rulingAxisTolerance:
		y := (start.Y + end.Y) / 2
		if start.X > end.X {
			start, end = end, start
		}
		start.Y = y
		end.Y = y
	case math.Abs(start.X-end.X) <= rulingAxisTolerance:
		x := (start.X + end.X) / 2
		if start.Y > end.Y {
			start, end = end, start
		}
		start.X = x
		end.X = x
	default:
		return document.Ruling{}, false
	}
	return document.Ruling{Start: start, End: end, Width: width}, true
}

func transformPoint(
	matrix affineMatrix,
	x float64,
	y float64,
	pageHeight float64,
) document.Point {
	return document.Point{
		X: matrix.a*x + matrix.c*y + matrix.e,
		Y: pageHeight - (matrix.b*x + matrix.d*y + matrix.f),
	}
}

func transformedStrokeWidth(matrix affineMatrix, width float64) float64 {
	scaleX := math.Hypot(matrix.a, matrix.b)
	scaleY := math.Hypot(matrix.c, matrix.d)
	return math.Abs(width) * (scaleX + scaleY) / 2
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
	omissions := make(map[linkOmission]int)
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
		if omission != 0 {
			omissions[omission]++
		}
	}
	return links, unsupportedLinkDiagnostics(index+1, omissions), nil
}

func extractAnnotationLink(
	worker instance,
	pdf references.FPDF_DOCUMENT,
	annotation references.FPDF_ANNOTATION,
	pageHeight float64,
) (document.LinkAnnotation, bool, linkOmission, error) {
	subtype, err := worker.FPDFAnnot_GetSubtype(&requests.FPDFAnnot_GetSubtype{
		Annotation: annotation,
	})
	if err != nil {
		return document.LinkAnnotation{}, false, 0, fmt.Errorf("get subtype: %w", err)
	}
	if subtype == nil {
		return document.LinkAnnotation{}, false, 0, errNilAnnotSubtype
	}
	if subtype.Subtype != enums.FPDF_ANNOT_SUBTYPE_LINK {
		return document.LinkAnnotation{}, false, 0, nil
	}

	link, err := worker.FPDFAnnot_GetLink(&requests.FPDFAnnot_GetLink{
		Annotation: annotation,
	})
	if err != nil {
		return document.LinkAnnotation{}, false, 0, fmt.Errorf("get link: %w", err)
	}
	if link == nil || link.Link == "" {
		return document.LinkAnnotation{}, false, 0, errNilAnnotLink
	}
	directDestination, err := worker.FPDFLink_GetDest(&requests.FPDFLink_GetDest{
		Document: pdf,
		Link:     link.Link,
	})
	if err != nil {
		return document.LinkAnnotation{}, false, 0, fmt.Errorf("get destination: %w", err)
	}
	if directDestination != nil && directDestination.Dest != nil {
		target, ok, err := extractPageTarget(
			worker,
			pdf,
			*directDestination.Dest,
		)
		if err != nil {
			return document.LinkAnnotation{}, false, 0, err
		}
		if !ok {
			return document.LinkAnnotation{}, false, linkOmissionUnresolvedInternal, nil
		}
		return linkAnnotation(worker, annotation, pageHeight, target)
	}
	action, err := worker.FPDFLink_GetAction(&requests.FPDFLink_GetAction{
		Link: link.Link,
	})
	if err != nil {
		return document.LinkAnnotation{}, false, 0, fmt.Errorf("get action: %w", err)
	}
	if action == nil {
		return document.LinkAnnotation{}, false, 0, errNilLinkAction
	}
	if action.Action == nil {
		return document.LinkAnnotation{}, false, linkOmissionMissingAction, nil
	}
	actionType, err := worker.FPDFAction_GetType(&requests.FPDFAction_GetType{
		Action: *action.Action,
	})
	if err != nil {
		return document.LinkAnnotation{}, false, 0, fmt.Errorf("get action type: %w", err)
	}
	if actionType == nil {
		return document.LinkAnnotation{}, false, 0, errNilActionType
	}
	var target document.LinkTarget
	switch actionType.Type {
	case enums.FPDF_ACTION_ACTION_GOTO:
		destination, err := worker.FPDFAction_GetDest(&requests.FPDFAction_GetDest{
			Document: pdf,
			Action:   *action.Action,
		})
		if err != nil {
			return document.LinkAnnotation{}, false, 0, fmt.Errorf("get action destination: %w", err)
		}
		if destination == nil || destination.Dest == nil {
			return document.LinkAnnotation{}, false, linkOmissionUnresolvedInternal, nil
		}
		var ok bool
		target, ok, err = extractPageTarget(worker, pdf, *destination.Dest)
		if err != nil {
			return document.LinkAnnotation{}, false, 0, err
		}
		if !ok {
			return document.LinkAnnotation{}, false, linkOmissionUnresolvedInternal, nil
		}
	case enums.FPDF_ACTION_ACTION_URI:
		uri, err := worker.FPDFAction_GetURIPath(&requests.FPDFAction_GetURIPath{
			Document: pdf,
			Action:   *action.Action,
		})
		if err != nil {
			return document.LinkAnnotation{}, false, 0, fmt.Errorf("get URI: %w", err)
		}
		if uri == nil {
			return document.LinkAnnotation{}, false, 0, errors.New("PDFium returned no action URI")
		}
		if uri.URIPath == nil || !isReliableExternalURI(*uri.URIPath) {
			return document.LinkAnnotation{}, false, linkOmissionUnsafeURI, nil
		}
		target = document.LinkTarget{
			Kind: document.LinkTargetExternal,
			URI:  strings.TrimSpace(*uri.URIPath),
		}
	default:
		return document.LinkAnnotation{}, false, linkOmissionUnsupportedAction, nil
	}
	return linkAnnotation(worker, annotation, pageHeight, target)
}

func extractPageTarget(
	worker instance,
	pdf references.FPDF_DOCUMENT,
	destination references.FPDF_DEST,
) (document.LinkTarget, bool, error) {
	page, err := worker.FPDFDest_GetDestPageIndex(
		&requests.FPDFDest_GetDestPageIndex{
			Document: pdf,
			Dest:     destination,
		},
	)
	if err != nil {
		return document.LinkTarget{}, false, fmt.Errorf("get destination page: %w", err)
	}
	if page == nil {
		return document.LinkTarget{}, false, errNilDestPageIndex
	}
	if page.Index < 0 {
		return document.LinkTarget{}, false, nil
	}
	target := document.LinkTarget{
		Kind: document.LinkTargetPage,
		Page: page.Index + 1,
	}
	if err := target.Validate(); err != nil {
		return document.LinkTarget{}, false, err
	}
	return target, true, nil
}

func linkAnnotation(
	worker instance,
	annotation references.FPDF_ANNOTATION,
	pageHeight float64,
	target document.LinkTarget,
) (document.LinkAnnotation, bool, linkOmission, error) {
	rect, err := worker.FPDFAnnot_GetRect(&requests.FPDFAnnot_GetRect{
		Annotation: annotation,
	})
	if err != nil {
		return document.LinkAnnotation{}, false, 0, fmt.Errorf("get rectangle: %w", err)
	}
	if rect == nil {
		return document.LinkAnnotation{}, false, 0, errNilAnnotRect
	}

	result := document.LinkAnnotation{
		Bounds: document.Rectangle{
			Left:   float64(rect.Rect.Left),
			Top:    pageHeight - float64(rect.Rect.Top),
			Right:  float64(rect.Rect.Right),
			Bottom: pageHeight - float64(rect.Rect.Bottom),
		},
		Target: target,
	}
	if err := result.Validate(); err != nil {
		return document.LinkAnnotation{}, false, 0, err
	}
	return result, true, 0, nil
}

func unsupportedLinkDiagnostics(
	page int,
	omissions map[linkOmission]int,
) []document.Diagnostic {
	var diagnostics []document.Diagnostic
	for _, category := range []linkOmission{
		linkOmissionMissingAction,
		linkOmissionUnresolvedInternal,
		linkOmissionUnsafeURI,
		linkOmissionUnsupportedAction,
	} {
		count := omissions[category]
		if count == 0 {
			continue
		}
		diagnostics = append(diagnostics, document.Diagnostic{
			Code:    document.DiagnosticUnsupportedLink,
			Page:    page,
			Message: category.message(count),
		})
	}
	return diagnostics
}

func (o linkOmission) message(count int) string {
	if count == 1 {
		switch o {
		case linkOmissionMissingAction:
			return "link without a supported action was preserved as text"
		case linkOmissionUnresolvedInternal:
			return "internal link without a resolvable destination was preserved as text"
		case linkOmissionUnsafeURI:
			return "unsupported or unsafe link URI was preserved as text"
		case linkOmissionUnsupportedAction:
			return "unsupported link action was preserved as text"
		}
	}
	switch o {
	case linkOmissionMissingAction:
		return fmt.Sprintf("%d links without supported actions were preserved as text", count)
	case linkOmissionUnresolvedInternal:
		return fmt.Sprintf("%d internal links without resolvable destinations were preserved as text", count)
	case linkOmissionUnsafeURI:
		return fmt.Sprintf("%d unsupported or unsafe link URIs were preserved as text", count)
	case linkOmissionUnsupportedAction:
		return fmt.Sprintf("%d unsupported link actions were preserved as text", count)
	default:
		return fmt.Sprintf("%d unsupported links were preserved as text", count)
	}
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
