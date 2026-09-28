package pdfium

import (
	"bytes"
	"context"
	"errors"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/document"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/extract"
	"github.com/klippa-app/go-pdfium/enums"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/responses"
	"github.com/klippa-app/go-pdfium/structs"
)

func TestExtractorMapsPDFiumLayout(t *testing.T) {
	t.Parallel()

	log := &eventLog{}
	action := references.FPDF_ACTION("action")
	uri := "https://example.test/docs"
	worker := &instanceStub{
		log:       log,
		document:  "document",
		pageCount: 1,
		pageSize: &responses.FPDF_GetPageSizeByIndex{
			Page:   0,
			Width:  612,
			Height: 792,
		},
		structuredText: &responses.GetPageTextStructured{
			Page: 0,
			Chars: []*responses.GetPageTextStructuredChar{
				{
					Text:  "T",
					Angle: math.Pi / 2,
					PointPosition: responses.CharPosition{
						Left:   72,
						Top:    720,
						Right:  80,
						Bottom: 708,
					},
					FontInformation: &responses.FontInformation{
						Size:         12,
						RenderedSize: 13,
						Weight:       700,
						Name:         "Example Sans",
						Flags:        fontFlagItalic,
					},
				},
				{
					Text: "",
				},
			},
		},
		annotationCount: 1,
		annotation:      "annotation",
		annotationType:  enums.FPDF_ANNOT_SUBTYPE_LINK,
		annotationLink:  "link",
		linkAction:      &action,
		actionType:      enums.FPDF_ACTION_ACTION_URI,
		actionURI:       &uri,
		annotationRect: structs.FPDF_FS_RECTF{
			Left:   70,
			Top:    722,
			Right:  90,
			Bottom: 706,
		},
	}
	runtime := &Runtime{pool: &poolStub{log: log, worker: worker}}
	extractor, err := NewExtractor(runtime)
	if err != nil {
		t.Fatalf("NewExtractor() returned an unexpected error: %v", err)
	}

	layout, err := extractor.Extract(
		context.Background(),
		bytes.NewReader([]byte("%PDF-source")),
	)
	if err != nil {
		t.Fatalf("Extract() returned an unexpected error: %v", err)
	}

	if got, want := extractor.Name(), "pdfium-wasm"; got != want {
		t.Fatalf("Name() = %q, want %q", got, want)
	}
	if len(layout.Pages) != 1 {
		t.Fatalf("page count = %d, want 1", len(layout.Pages))
	}
	page := layout.Pages[0]
	if page.Number != 1 || page.Width != 612 || page.Height != 792 {
		t.Fatalf("page = %+v, want number 1 and size 612x792", page)
	}
	if len(page.TextRuns) != 1 {
		t.Fatalf("text run count = %d, want 1", len(page.TextRuns))
	}

	run := page.TextRuns[0]
	if got, want := run.Text, "T"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if got, want := run.Bounds.Top, 72.0; got != want {
		t.Errorf("top = %v, want %v", got, want)
	}
	if got, want := run.Bounds.Bottom, 84.0; got != want {
		t.Errorf("bottom = %v, want %v", got, want)
	}
	if math.Abs(run.RotationDegrees-90) > 0.0001 {
		t.Errorf("rotation = %v, want 90 degrees", run.RotationDegrees)
	}
	if got, want := run.Style.FontSize, 13.0; got != want {
		t.Errorf("font size = %v, want rendered size %v", got, want)
	}
	if run.Style.FontName != "Example Sans" || run.Style.FontWeight != 700 || !run.Style.Italic {
		t.Errorf("style = %+v, want mapped font name, weight, and italic flag", run.Style)
	}
	if got, want := len(page.Links), 1; got != want {
		t.Fatalf("link count = %d, want %d", got, want)
	}
	link := page.Links[0]
	if link.Target != (document.LinkTarget{
		Kind: document.LinkTargetExternal,
		URI:  uri,
	}) {
		t.Errorf("link target = %+v, want external URI %q", link.Target, uri)
	}

	if link.Bounds != (document.Rectangle{Left: 70, Top: 70, Right: 90, Bottom: 86}) {
		t.Errorf("link bounds = %+v, want normalized PDF rectangle", link.Bounds)
	}
	if !slices.Contains(log.snapshot(), "close annotation") {
		t.Fatal("link annotation was not closed")
	}

	if worker.pageSizeRequest == nil || worker.pageSizeRequest.Index != 0 {
		t.Fatalf("page size request = %+v, want page index 0", worker.pageSizeRequest)
	}
	if worker.textRequest == nil {
		t.Fatal("structured text request was nil")
	}
	if worker.textRequest.Mode != "char" || !worker.textRequest.CollectFontInformation {
		t.Fatalf("structured text request = %+v, want character mode with font information", worker.textRequest)
	}
}

func TestExtractPageRulingsMapsVisibleAxisAlignedSegments(t *testing.T) {
	t.Parallel()

	path := references.FPDF_PAGEOBJECT("path")
	segments := []references.FPDF_PATHSEGMENT{
		"move-horizontal",
		"line-horizontal",
		"move-vertical",
		"line-vertical",
		"line-diagonal",
	}
	worker := &instanceStub{
		log:         &eventLog{},
		pageObjects: []references.FPDF_PAGEOBJECT{path},
		pageObjectTypes: map[references.FPDF_PAGEOBJECT]enums.FPDF_PAGEOBJ{
			path: enums.FPDF_PAGEOBJ_PATH,
		},
		pathFillModes: map[references.FPDF_PAGEOBJECT]enums.FPDF_FILLMODE{
			path: enums.FPDF_FILLMODE_WINDING,
		},
		pathSegments: map[references.FPDF_PAGEOBJECT][]references.FPDF_PATHSEGMENT{
			path: segments,
		},
		segmentTypes: map[references.FPDF_PATHSEGMENT]enums.FPDF_SEGMENT{
			segments[0]: enums.FPDF_SEGMENT_MOVETO,
			segments[1]: enums.FPDF_SEGMENT_LINETO,
			segments[2]: enums.FPDF_SEGMENT_MOVETO,
			segments[3]: enums.FPDF_SEGMENT_LINETO,
			segments[4]: enums.FPDF_SEGMENT_LINETO,
		},
		segmentPoints: map[references.FPDF_PATHSEGMENT]structs.FPDF_FS_POINTF{
			segments[0]: {X: 10, Y: 20},
			segments[1]: {X: 40, Y: 20},
			segments[2]: {X: 50, Y: 10},
			segments[3]: {X: 50, Y: 30},
			segments[4]: {X: 60, Y: 40},
		},
		strokeWidths: map[references.FPDF_PAGEOBJECT]float32{path: 0.5},
	}
	counts := extractionCounts{}

	got, err := extractPageRulings(
		worker,
		"document",
		0,
		100,
		extract.DefaultLimits(),
		&counts,
	)
	if err != nil {
		t.Fatalf("extractPageRulings() returned an unexpected error: %v", err)
	}
	want := []document.Ruling{
		{
			Start: document.Point{X: 10, Y: 80},
			End:   document.Point{X: 40, Y: 80},
			Width: 0.5,
		},
		{
			Start: document.Point{X: 50, Y: 70},
			End:   document.Point{X: 50, Y: 90},
			Width: 0.5,
		},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("rulings = %#v, want %#v", got, want)
	}
	if counts.pageObjects != 1 || counts.pathSegments != 5 || counts.rulings != 2 {
		t.Fatalf("extraction counts = %+v, want 1 object, 5 segments, 2 rulings", counts)
	}
}

func TestAppendRulingEnforcesLimit(t *testing.T) {
	t.Parallel()

	limits := extract.DefaultLimits()
	limits.MaxRulings = 1
	counts := extractionCounts{}
	rulings, err := appendRuling(
		nil,
		document.Point{X: 10, Y: 10},
		document.Point{X: 20, Y: 10},
		1,
		limits,
		&counts,
	)
	if err != nil {
		t.Fatalf("appendRuling() returned an unexpected error: %v", err)
	}
	_, err = appendRuling(
		rulings,
		document.Point{X: 10, Y: 20},
		document.Point{X: 20, Y: 20},
		1,
		limits,
		&counts,
	)
	if !errors.Is(err, extract.ErrLimitExceeded) {
		t.Fatalf("appendRuling() error = %v, want %v", err, extract.ErrLimitExceeded)
	}
}

func TestExtractAnnotationLinkMapsInternalGoToPage(t *testing.T) {
	t.Parallel()

	action := references.FPDF_ACTION("action")
	destination := references.FPDF_DEST("destination")
	worker := &instanceStub{
		log:            &eventLog{},
		annotationType: enums.FPDF_ANNOT_SUBTYPE_LINK,
		annotationLink: "link",
		linkAction:     &action,
		actionType:     enums.FPDF_ACTION_ACTION_GOTO,
		actionDest:     &destination,
		destPageIndex:  4,
		annotationRect: structs.FPDF_FS_RECTF{
			Left:   70,
			Top:    722,
			Right:  90,
			Bottom: 706,
		},
	}

	link, include, omission, err := extractAnnotationLink(
		worker,
		"document",
		"annotation",
		792,
	)
	if err != nil {
		t.Fatalf("extractAnnotationLink() returned an unexpected error: %v", err)
	}
	if !include || omission != 0 {
		t.Fatalf("include/omission = %v/%d, want true/zero", include, omission)
	}
	want := document.LinkTarget{Kind: document.LinkTargetPage, Page: 5}
	if link.Target != want {
		t.Fatalf("link target = %+v, want %+v", link.Target, want)
	}
}

func TestExtractAnnotationLinkIgnoresUnsafeURI(t *testing.T) {
	t.Parallel()

	log := &eventLog{}
	action := references.FPDF_ACTION("action")
	uri := "javascript:alert(1)"
	worker := &instanceStub{
		log:            log,
		annotationType: enums.FPDF_ANNOT_SUBTYPE_LINK,
		annotationLink: "link",
		linkAction:     &action,
		actionType:     enums.FPDF_ACTION_ACTION_URI,
		actionURI:      &uri,
	}

	_, include, omission, err := extractAnnotationLink(
		worker,
		"document",
		"annotation",
		792,
	)
	if err != nil {
		t.Fatalf("extractAnnotationLink() returned an unexpected error: %v", err)
	}
	if include {
		t.Fatal("extractAnnotationLink() included an unsafe URI")
	}
	if omission == 0 {
		t.Fatal("extractAnnotationLink() returned no omission diagnostic")
	}
	if omission != linkOmissionUnsafeURI {
		t.Fatalf("omission = %d, want unsafe URI category", omission)
	}
}

func TestUnsupportedLinkDiagnosticsAggregateByPageAndCategory(t *testing.T) {
	t.Parallel()

	got := unsupportedLinkDiagnostics(3, map[linkOmission]int{
		linkOmissionMissingAction:      1,
		linkOmissionUnresolvedInternal: 3,
		linkOmissionUnsafeURI:          4,
		linkOmissionUnsupportedAction:  2,
	})
	want := []document.Diagnostic{
		{
			Code:    document.DiagnosticUnsupportedLink,
			Page:    3,
			Message: "link without a supported action was preserved as text",
		},
		{
			Code:    document.DiagnosticUnsupportedLink,
			Page:    3,
			Message: "3 internal links without resolvable destinations were preserved as text",
		},
		{
			Code:    document.DiagnosticUnsupportedLink,
			Page:    3,
			Message: "4 unsupported or unsafe link URIs were preserved as text",
		},
		{
			Code:    document.DiagnosticUnsupportedLink,
			Page:    3,
			Message: "2 unsupported link actions were preserved as text",
		},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("diagnostics = %#v, want %#v", got, want)
	}
}

func TestExtractPageLinksAggregatesRepeatedOmissions(t *testing.T) {
	t.Parallel()

	action := references.FPDF_ACTION("action")
	uri := "javascript:alert(1)"
	worker := &instanceStub{
		log:             &eventLog{},
		annotationCount: 3,
		annotation:      "annotation",
		annotationType:  enums.FPDF_ANNOT_SUBTYPE_LINK,
		annotationLink:  "link",
		linkAction:      &action,
		actionType:      enums.FPDF_ACTION_ACTION_URI,
		actionURI:       &uri,
	}
	counts := extractionCounts{}

	links, diagnostics, err := extractPageLinks(
		worker,
		"document",
		1,
		792,
		extract.DefaultLimits(),
		&counts,
	)
	if err != nil {
		t.Fatalf("extractPageLinks() returned an unexpected error: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("link count = %d, want 0", len(links))
	}
	want := []document.Diagnostic{{
		Code:    document.DiagnosticUnsupportedLink,
		Page:    2,
		Message: "3 unsupported or unsafe link URIs were preserved as text",
	}}
	if !slices.Equal(diagnostics, want) {
		t.Fatalf("diagnostics = %#v, want %#v", diagnostics, want)
	}
	if counts.annotations != 3 {
		t.Fatalf("annotation count = %d, want 3", counts.annotations)
	}
}

func TestExtractorClosesAnnotationAndPreservesInspectionErrors(t *testing.T) {
	t.Parallel()

	log := &eventLog{}
	inspectErr := errors.New("inspect annotation failed")
	closeErr := errors.New("close annotation failed")
	worker := &instanceStub{
		log:                log,
		document:           "document",
		pageCount:          1,
		pageSize:           &responses.FPDF_GetPageSizeByIndex{Page: 0, Width: 100, Height: 100},
		structuredText:     &responses.GetPageTextStructured{Page: 0},
		annotationCount:    1,
		annotation:         "annotation",
		annotationTypeErr:  inspectErr,
		closeAnnotationErr: closeErr,
	}
	extractor, err := NewExtractor(
		&Runtime{pool: &poolStub{log: log, worker: worker}},
	)
	if err != nil {
		t.Fatalf("NewExtractor() returned an unexpected error: %v", err)
	}

	_, err = extractor.Extract(
		context.Background(),
		bytes.NewReader([]byte("%PDF")),
	)
	for _, want := range []error{inspectErr, closeErr} {
		if !errors.Is(err, want) {
			t.Errorf("Extract() error = %v, want wrapped error %v", err, want)
		}
	}
	if !slices.Contains(log.snapshot(), "close annotation") {
		t.Fatal("annotation was not closed after inspection failure")
	}
}

func TestExtractorNormalizesUnavailableFontInformation(t *testing.T) {
	t.Parallel()

	run, include, err := mapCharacter(100, &responses.GetPageTextStructuredChar{
		Text: " ",
		PointPosition: responses.CharPosition{
			Left:   1,
			Top:    10,
			Right:  2,
			Bottom: 5,
		},
		FontInformation: &responses.FontInformation{
			Size:         8,
			RenderedSize: 0,
			Weight:       -1,
		},
	})

	if err != nil {
		t.Fatalf("mapCharacter() returned an unexpected error: %v", err)
	}
	if !include {
		t.Fatal("mapCharacter() excluded a visible space")
	}
	if got, want := run.Style.FontSize, 8.0; got != want {
		t.Errorf("font size = %v, want fallback size %v", got, want)
	}
	if got := run.Style.FontWeight; got != 0 {
		t.Errorf("font weight = %d, want unknown value normalized to zero", got)
	}
}

func TestMapCharacterRejectsInvalidPDFiumData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		char *responses.GetPageTextStructuredChar
	}{
		{
			name: "nil character",
			char: nil,
		},
		{
			name: "inverted vertical bounds",
			char: &responses.GetPageTextStructuredChar{
				Text: "T",
				PointPosition: responses.CharPosition{
					Left:   1,
					Top:    5,
					Right:  2,
					Bottom: 10,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, _, err := mapCharacter(100, test.char); err == nil {
				t.Fatal("mapCharacter() returned nil error for invalid data")
			}
		})
	}
}

func TestExtractorReportsPageContext(t *testing.T) {
	t.Parallel()

	log := &eventLog{}
	sizeErr := errors.New("size failed")
	worker := &instanceStub{
		log:         log,
		document:    "document",
		pageCount:   1,
		pageSizeErr: sizeErr,
	}
	runtime := &Runtime{pool: &poolStub{log: log, worker: worker}}
	extractor, err := NewExtractor(runtime)
	if err != nil {
		t.Fatalf("NewExtractor() returned an unexpected error: %v", err)
	}

	_, err = extractor.Extract(
		context.Background(),
		bytes.NewReader([]byte("%PDF-source")),
	)
	if !errors.Is(err, sizeErr) {
		t.Fatalf("Extract() error = %v, want page-size error %v", err, sizeErr)
	}
	if !strings.Contains(err.Error(), "page 1: get size") {
		t.Fatalf("Extract() error = %q, want page context", err)
	}
}

func TestExtractorRejectsMismatchedPageResponse(t *testing.T) {
	t.Parallel()

	log := &eventLog{}
	worker := &instanceStub{
		log:       log,
		document:  "document",
		pageCount: 1,
		pageSize: &responses.FPDF_GetPageSizeByIndex{
			Page:   1,
			Width:  612,
			Height: 792,
		},
	}
	runtime := &Runtime{pool: &poolStub{log: log, worker: worker}}
	extractor, err := NewExtractor(runtime)
	if err != nil {
		t.Fatalf("NewExtractor() returned an unexpected error: %v", err)
	}

	_, err = extractor.Extract(
		context.Background(),
		bytes.NewReader([]byte("%PDF-source")),
	)
	if !errors.Is(err, errMismatchedPage) {
		t.Fatalf("Extract() error = %v, want %v", err, errMismatchedPage)
	}
}

func TestExtractorRejectsDocumentWithoutPages(t *testing.T) {
	t.Parallel()

	log := &eventLog{}
	worker := &instanceStub{log: log, document: "document"}
	runtime := &Runtime{pool: &poolStub{log: log, worker: worker}}
	extractor, err := NewExtractor(runtime)
	if err != nil {
		t.Fatalf("NewExtractor() returned an unexpected error: %v", err)
	}

	_, err = extractor.Extract(
		context.Background(),
		bytes.NewReader([]byte("%PDF-source")),
	)
	if !errors.Is(err, errNoPages) {
		t.Fatalf("Extract() error = %v, want %v", err, errNoPages)
	}
	if !errors.Is(err, extract.ErrInvalidDocument) {
		t.Fatalf("Extract() error = %v, want %v", err, extract.ErrInvalidDocument)
	}
}

func TestNewExtractorRejectsNilRuntime(t *testing.T) {
	t.Parallel()

	_, err := NewExtractor(nil)
	if !errors.Is(err, errNilRuntime) {
		t.Fatalf("NewExtractor() error = %v, want %v", err, errNilRuntime)
	}
}

func TestNewExtractorRejectsInvalidLimits(t *testing.T) {
	t.Parallel()

	limits := extract.DefaultLimits()
	limits.MaxPages = 0
	_, err := NewExtractorWithLimits(&Runtime{}, limits)
	if err == nil {
		t.Fatal("NewExtractorWithLimits() returned nil for invalid limits")
	}
}

func TestExtractorEnforcesSourceSizeBeforeRuntimeAcquisition(t *testing.T) {
	t.Parallel()

	log := &eventLog{}
	runtime := &Runtime{pool: &poolStub{log: log}}
	limits := extract.DefaultLimits()
	limits.MaxSourceBytes = 3
	extractor, err := NewExtractorWithLimits(runtime, limits)
	if err != nil {
		t.Fatalf("NewExtractorWithLimits() returned an unexpected error: %v", err)
	}

	_, err = extractor.Extract(
		context.Background(),
		bytes.NewReader([]byte("%PDF")),
	)
	if !errors.Is(err, extract.ErrLimitExceeded) {
		t.Fatalf("Extract() error = %v, want %v", err, extract.ErrLimitExceeded)
	}
	if events := log.snapshot(); len(events) != 0 {
		t.Fatalf("runtime events = %v, want none", events)
	}
}

func TestExtractorEnforcesPDFiumLimitsAtAvailableBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		configure      func(*instanceStub, *extract.Limits)
		forbiddenEvent string
	}{
		{
			name: "page count before page iteration",
			configure: func(worker *instanceStub, limits *extract.Limits) {
				worker.pageCount = 2
				limits.MaxPages = 1
			},
			forbiddenEvent: "get page size",
		},
		{
			name: "page dimension before text extraction",
			configure: func(worker *instanceStub, limits *extract.Limits) {
				worker.pageSize.Width = 101
				limits.MaxPageDimension = 100
			},
			forbiddenEvent: "get structured text",
		},
		{
			name: "cumulative text runs before mapping",
			configure: func(worker *instanceStub, limits *extract.Limits) {
				worker.structuredText.Chars = []*responses.GetPageTextStructuredChar{
					{Text: "a"},
					{Text: "b"},
				}
				limits.MaxTextRuns = 1
			},
			forbiddenEvent: "get annotation count",
		},
		{
			name: "cumulative annotations before enumeration",
			configure: func(worker *instanceStub, limits *extract.Limits) {
				worker.annotationCount = 2
				limits.MaxAnnotations = 1
			},
			forbiddenEvent: "get annotation",
		},
		{
			name: "cumulative page objects before enumeration",
			configure: func(worker *instanceStub, limits *extract.Limits) {
				worker.pageObjects = []references.FPDF_PAGEOBJECT{"one", "two"}
				limits.MaxPageObjects = 1
			},
			forbiddenEvent: "get page object",
		},
		{
			name: "cumulative path segments before enumeration",
			configure: func(worker *instanceStub, limits *extract.Limits) {
				path := references.FPDF_PAGEOBJECT("path")
				worker.pageObjects = []references.FPDF_PAGEOBJECT{path}
				worker.pageObjectTypes = map[references.FPDF_PAGEOBJECT]enums.FPDF_PAGEOBJ{
					path: enums.FPDF_PAGEOBJ_PATH,
				}
				worker.pathStrokes = map[references.FPDF_PAGEOBJECT]bool{path: true}
				worker.pathSegments = map[references.FPDF_PAGEOBJECT][]references.FPDF_PATHSEGMENT{
					path: {"one", "two"},
				}
				limits.MaxPathSegments = 1
			},
			forbiddenEvent: "get path segment",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			log := &eventLog{}
			worker := &instanceStub{
				log:       log,
				document:  "document",
				pageCount: 1,
				pageSize: &responses.FPDF_GetPageSizeByIndex{
					Page:   0,
					Width:  100,
					Height: 100,
				},
				structuredText: &responses.GetPageTextStructured{Page: 0},
			}
			limits := extract.DefaultLimits()
			test.configure(worker, &limits)
			extractor, err := NewExtractorWithLimits(
				&Runtime{pool: &poolStub{log: log, worker: worker}},
				limits,
			)
			if err != nil {
				t.Fatalf("NewExtractorWithLimits() returned an unexpected error: %v", err)
			}

			_, err = extractor.Extract(
				context.Background(),
				bytes.NewReader([]byte("%PDF")),
			)
			if !errors.Is(err, extract.ErrLimitExceeded) {
				t.Fatalf(
					"Extract() error = %v, want %v",
					err,
					extract.ErrLimitExceeded,
				)
			}
			if slices.Contains(log.snapshot(), test.forbiddenEvent) {
				t.Fatalf(
					"runtime events = %v, must not contain %q",
					log.snapshot(),
					test.forbiddenEvent,
				)
			}
		})
	}
}
