package pdfium

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"sync"
	"testing"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/extract"
	"github.com/klippa-app/go-pdfium/enums"
	pdfiumerrors "github.com/klippa-app/go-pdfium/errors"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/responses"
	"github.com/klippa-app/go-pdfium/structs"
)

type eventLog struct {
	mu     sync.Mutex
	events []string
}

func (l *eventLog) add(event string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, event)
}

func (l *eventLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.events)
}

type poolStub struct {
	log      *eventLog
	worker   instance
	getErr   error
	closeErr error
}

type negativeSizeSource struct{}

func (negativeSizeSource) ReadAt([]byte, int64) (int, error) {
	return 0, io.EOF
}

func (negativeSizeSource) Size() int64 {
	return -1
}

func (p *poolStub) getInstance(context.Context) (instance, error) {
	p.log.add("acquire instance")
	return p.worker, p.getErr
}

func (p *poolStub) Close() error {
	p.log.add("close runtime")
	return p.closeErr
}

type instanceStub struct {
	log              *eventLog
	document         references.FPDF_DOCUMENT
	openErr          error
	closeDocumentErr error
	pageCount        int
	pageCountErr     error
	pageSize         *responses.FPDF_GetPageSizeByIndex
	pageSizeErr      error
	structuredText   *responses.GetPageTextStructured
	structuredErr    error
	annotationCount  int
	annotation       references.FPDF_ANNOTATION
	annotationType   enums.FPDF_ANNOTATION_SUBTYPE
	annotationLink   references.FPDF_LINK
	linkAction       *references.FPDF_ACTION
	actionType       enums.FPDF_ACTION_ACTION
	actionURI        *string
	annotationRect   structs.FPDF_FS_RECTF
	closeErr         error
	killErr          error
	killDone         chan struct{}
	request          *requests.OpenDocument
	pageSizeRequest  *requests.FPDF_GetPageSizeByIndex
	textRequest      *requests.GetPageTextStructured
}

func (i *instanceStub) OpenDocument(request *requests.OpenDocument) (*responses.OpenDocument, error) {
	i.log.add("open document")
	i.request = request
	if i.openErr != nil {
		return nil, i.openErr
	}
	return &responses.OpenDocument{Document: i.document}, nil
}

func (i *instanceStub) FPDF_CloseDocument(
	*requests.FPDF_CloseDocument,
) (*responses.FPDF_CloseDocument, error) {
	i.log.add("close document")
	return &responses.FPDF_CloseDocument{}, i.closeDocumentErr
}

func (i *instanceStub) FPDF_GetPageCount(
	*requests.FPDF_GetPageCount,
) (*responses.FPDF_GetPageCount, error) {
	i.log.add("get page count")
	return &responses.FPDF_GetPageCount{PageCount: i.pageCount}, i.pageCountErr
}

func (i *instanceStub) FPDF_GetPageSizeByIndex(
	request *requests.FPDF_GetPageSizeByIndex,
) (*responses.FPDF_GetPageSizeByIndex, error) {
	i.log.add("get page size")
	i.pageSizeRequest = request
	return i.pageSize, i.pageSizeErr
}

func (i *instanceStub) GetPageTextStructured(
	request *requests.GetPageTextStructured,
) (*responses.GetPageTextStructured, error) {
	i.log.add("get structured text")
	i.textRequest = request
	return i.structuredText, i.structuredErr
}

func (i *instanceStub) FPDFPage_GetAnnotCount(
	*requests.FPDFPage_GetAnnotCount,
) (*responses.FPDFPage_GetAnnotCount, error) {
	i.log.add("get annotation count")
	return &responses.FPDFPage_GetAnnotCount{Count: i.annotationCount}, nil
}

func (i *instanceStub) FPDFPage_GetAnnot(
	*requests.FPDFPage_GetAnnot,
) (*responses.FPDFPage_GetAnnot, error) {
	i.log.add("get annotation")
	return &responses.FPDFPage_GetAnnot{Annotation: i.annotation}, nil
}

func (i *instanceStub) FPDFPage_CloseAnnot(
	*requests.FPDFPage_CloseAnnot,
) (*responses.FPDFPage_CloseAnnot, error) {
	i.log.add("close annotation")
	return &responses.FPDFPage_CloseAnnot{}, nil
}

func (i *instanceStub) FPDFAnnot_GetSubtype(
	*requests.FPDFAnnot_GetSubtype,
) (*responses.FPDFAnnot_GetSubtype, error) {
	return &responses.FPDFAnnot_GetSubtype{Subtype: i.annotationType}, nil
}

func (i *instanceStub) FPDFAnnot_GetLink(
	*requests.FPDFAnnot_GetLink,
) (*responses.FPDFAnnot_GetLink, error) {
	return &responses.FPDFAnnot_GetLink{Link: i.annotationLink}, nil
}

func (i *instanceStub) FPDFAnnot_GetRect(
	*requests.FPDFAnnot_GetRect,
) (*responses.FPDFAnnot_GetRect, error) {
	return &responses.FPDFAnnot_GetRect{Rect: i.annotationRect}, nil
}

func (i *instanceStub) FPDFLink_GetAction(
	*requests.FPDFLink_GetAction,
) (*responses.FPDFLink_GetAction, error) {
	return &responses.FPDFLink_GetAction{Action: i.linkAction}, nil
}

func (i *instanceStub) FPDFAction_GetType(
	*requests.FPDFAction_GetType,
) (*responses.FPDFAction_GetType, error) {
	return &responses.FPDFAction_GetType{Type: i.actionType}, nil
}

func (i *instanceStub) FPDFAction_GetURIPath(
	*requests.FPDFAction_GetURIPath,
) (*responses.FPDFAction_GetURIPath, error) {
	return &responses.FPDFAction_GetURIPath{URIPath: i.actionURI}, nil
}

func (i *instanceStub) Close() error {
	i.log.add("close instance")
	return i.closeErr
}

func (i *instanceStub) Kill() error {
	i.log.add("kill instance")
	if i.killDone != nil {
		close(i.killDone)
	}
	return i.killErr
}

func TestRuntimeCloseIsIdempotent(t *testing.T) {
	t.Parallel()

	log := &eventLog{}
	closeErr := errors.New("close failed")
	runtime := &Runtime{pool: &poolStub{log: log, closeErr: closeErr}}

	first := runtime.Close()
	second := runtime.Close()

	if !errors.Is(first, closeErr) || !errors.Is(second, closeErr) {
		t.Fatalf("Close() errors = %v and %v, want both to wrap %v", first, second, closeErr)
	}
	if got, want := log.snapshot(), []string{"close runtime"}; !slices.Equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func TestRuntimeWithInstanceClosesAfterOperation(t *testing.T) {
	t.Parallel()

	log := &eventLog{}
	worker := &instanceStub{log: log}
	runtime := &Runtime{pool: &poolStub{log: log, worker: worker}}

	err := runtime.withInstance(context.Background(), func(instance) error {
		log.add("operation")
		return nil
	})

	if err != nil {
		t.Fatalf("withInstance() returned an unexpected error: %v", err)
	}
	want := []string{"acquire instance", "operation", "close instance"}
	if got := log.snapshot(); !slices.Equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func TestRuntimeWithInstanceJoinsOperationAndCloseErrors(t *testing.T) {
	t.Parallel()

	log := &eventLog{}
	operationErr := errors.New("operation failed")
	closeErr := errors.New("close failed")
	runtime := &Runtime{
		pool: &poolStub{
			log:    log,
			worker: &instanceStub{log: log, closeErr: closeErr},
		},
	}

	err := runtime.withInstance(context.Background(), func(instance) error {
		return operationErr
	})

	if !errors.Is(err, operationErr) {
		t.Fatalf("withInstance() error = %v, want operation error %v", err, operationErr)
	}
	if !errors.Is(err, closeErr) {
		t.Fatalf("withInstance() error = %v, want close error %v", err, closeErr)
	}
}

func TestRuntimeWithInstanceReportsAcquisitionFailure(t *testing.T) {
	t.Parallel()

	log := &eventLog{}
	acquireErr := errors.New("acquire failed")
	runtime := &Runtime{pool: &poolStub{log: log, getErr: acquireErr}}

	err := runtime.withInstance(context.Background(), func(instance) error {
		t.Fatal("operation called after acquisition failure")
		return nil
	})

	if !errors.Is(err, acquireErr) {
		t.Fatalf("withInstance() error = %v, want acquisition error %v", err, acquireErr)
	}
	if got, want := log.snapshot(), []string{"acquire instance"}; !slices.Equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func TestRuntimeWithInstanceKillsOnCancellation(t *testing.T) {
	t.Parallel()

	log := &eventLog{}
	killDone := make(chan struct{})
	worker := &instanceStub{log: log, killDone: killDone}
	runtime := &Runtime{pool: &poolStub{log: log, worker: worker}}
	ctx, cancel := context.WithCancel(context.Background())

	err := runtime.withInstance(ctx, func(instance) error {
		cancel()
		<-killDone
		return nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("withInstance() error = %v, want context cancellation", err)
	}
	want := []string{"acquire instance", "kill instance"}
	if got := log.snapshot(); !slices.Equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func TestRuntimeWithInstanceJoinsCancellationAndKillErrors(t *testing.T) {
	t.Parallel()

	log := &eventLog{}
	killDone := make(chan struct{})
	killErr := errors.New("kill failed")
	worker := &instanceStub{log: log, killDone: killDone, killErr: killErr}
	runtime := &Runtime{pool: &poolStub{log: log, worker: worker}}
	ctx, cancel := context.WithCancel(context.Background())

	err := runtime.withInstance(ctx, func(instance) error {
		cancel()
		<-killDone
		return nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("withInstance() error = %v, want context cancellation", err)
	}
	if !errors.Is(err, killErr) {
		t.Fatalf("withInstance() error = %v, want kill error %v", err, killErr)
	}
}

func TestRuntimeWithDocumentBridgesSourceAndClosesInOrder(t *testing.T) {
	t.Parallel()

	log := &eventLog{}
	worker := &instanceStub{
		log:       log,
		document:  references.FPDF_DOCUMENT("document"),
		pageCount: 1,
	}
	runtime := &Runtime{pool: &poolStub{log: log, worker: worker}}
	source := bytes.NewReader([]byte("%PDF-source"))

	err := runtime.withDocument(
		context.Background(),
		source,
		func(worker instance, document references.FPDF_DOCUMENT) error {
			log.add("operation")
			if document != references.FPDF_DOCUMENT("document") {
				t.Fatalf("document = %q, want %q", document, "document")
			}
			return nil
		},
	)

	if err != nil {
		t.Fatalf("withDocument() returned an unexpected error: %v", err)
	}
	if worker.request == nil {
		t.Fatal("OpenDocument() request was nil")
	}
	if got, want := worker.request.FileReaderSize, int64(len("%PDF-source")); got != want {
		t.Fatalf("FileReaderSize = %d, want %d", got, want)
	}
	content, err := io.ReadAll(worker.request.FileReader)
	if err != nil {
		t.Fatalf("read bridged source: %v", err)
	}
	if got, want := string(content), "%PDF-source"; got != want {
		t.Fatalf("bridged source = %q, want %q", got, want)
	}
	want := []string{
		"acquire instance",
		"open document",
		"operation",
		"close document",
		"close instance",
	}
	if got := log.snapshot(); !slices.Equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func TestRuntimeWithDocumentClosesInstanceAfterOpenFailure(t *testing.T) {
	t.Parallel()

	log := &eventLog{}
	openErr := errors.New("open failed")
	worker := &instanceStub{log: log, openErr: openErr}
	runtime := &Runtime{pool: &poolStub{log: log, worker: worker}}

	err := runtime.withDocument(
		context.Background(),
		bytes.NewReader([]byte("not a PDF")),
		func(instance, references.FPDF_DOCUMENT) error {
			t.Fatal("operation called after open failure")
			return nil
		},
	)

	if !errors.Is(err, openErr) {
		t.Fatalf("withDocument() error = %v, want open error %v", err, openErr)
	}
	want := []string{"acquire instance", "open document", "close instance"}
	if got := log.snapshot(); !slices.Equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func TestRuntimeWithDocumentClassifiesOpenFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		openErr  error
		expected error
	}{
		{
			name:     "incorrect format",
			openErr:  pdfiumerrors.ErrFormat,
			expected: extract.ErrInvalidDocument,
		},
		{
			name:     "unreadable PDF structure",
			openErr:  pdfiumerrors.ErrFile,
			expected: extract.ErrInvalidDocument,
		},
		{
			name:     "password protected",
			openErr:  pdfiumerrors.ErrPassword,
			expected: extract.ErrEncryptedDocument,
		},
		{
			name:     "unsupported encryption",
			openErr:  pdfiumerrors.ErrSecurity,
			expected: extract.ErrEncryptedDocument,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			log := &eventLog{}
			runtime := &Runtime{
				pool: &poolStub{
					log:    log,
					worker: &instanceStub{log: log, openErr: test.openErr},
				},
			}
			err := runtime.withDocument(
				context.Background(),
				bytes.NewReader([]byte("%PDF-source")),
				func(instance, references.FPDF_DOCUMENT) error {
					t.Fatal("operation called after open failure")
					return nil
				},
			)
			if !errors.Is(err, test.expected) {
				t.Fatalf("withDocument() error = %v, want %v", err, test.expected)
			}
		})
	}
}

func TestRuntimeWithDocumentJoinsOperationAndCleanupErrors(t *testing.T) {
	t.Parallel()

	log := &eventLog{}
	operationErr := errors.New("operation failed")
	documentCloseErr := errors.New("document close failed")
	instanceCloseErr := errors.New("instance close failed")
	worker := &instanceStub{
		log:              log,
		document:         references.FPDF_DOCUMENT("document"),
		closeDocumentErr: documentCloseErr,
		closeErr:         instanceCloseErr,
	}
	runtime := &Runtime{pool: &poolStub{log: log, worker: worker}}

	err := runtime.withDocument(
		context.Background(),
		bytes.NewReader([]byte("%PDF-source")),
		func(instance, references.FPDF_DOCUMENT) error {
			return operationErr
		},
	)

	for _, want := range []error{operationErr, documentCloseErr, instanceCloseErr} {
		if !errors.Is(err, want) {
			t.Errorf("withDocument() error = %v, want wrapped error %v", err, want)
		}
	}
	wantEvents := []string{
		"acquire instance",
		"open document",
		"close document",
		"close instance",
	}
	if got := log.snapshot(); !slices.Equal(got, wantEvents) {
		t.Fatalf("events = %v, want %v", got, wantEvents)
	}
}

func TestRuntimeWithDocumentRejectsEmptySource(t *testing.T) {
	t.Parallel()

	log := &eventLog{}
	runtime := &Runtime{pool: &poolStub{log: log}}

	err := runtime.withDocument(context.Background(), bytes.NewReader(nil), func(instance, references.FPDF_DOCUMENT) error {
		return nil
	})

	if !errors.Is(err, errEmptySource) {
		t.Fatalf("withDocument() error = %v, want %v", err, errEmptySource)
	}
	if !errors.Is(err, extract.ErrInvalidDocument) {
		t.Fatalf("withDocument() error = %v, want %v", err, extract.ErrInvalidDocument)
	}
	if got := log.snapshot(); len(got) != 0 {
		t.Fatalf("events = %v, want no PDFium calls", got)
	}
}

func TestRuntimeWithDocumentRejectsNilSource(t *testing.T) {
	t.Parallel()

	log := &eventLog{}
	runtime := &Runtime{pool: &poolStub{log: log}}

	err := runtime.withDocument(context.Background(), nil, func(instance, references.FPDF_DOCUMENT) error {
		return nil
	})

	if !errors.Is(err, errNilSource) {
		t.Fatalf("withDocument() error = %v, want %v", err, errNilSource)
	}
	if got := log.snapshot(); len(got) != 0 {
		t.Fatalf("events = %v, want no PDFium calls", got)
	}
}

func TestRuntimeWithDocumentRejectsNegativeSourceSize(t *testing.T) {
	t.Parallel()

	log := &eventLog{}
	runtime := &Runtime{pool: &poolStub{log: log}}

	err := runtime.withDocument(
		context.Background(),
		negativeSizeSource{},
		func(instance, references.FPDF_DOCUMENT) error {
			return nil
		},
	)

	if !errors.Is(err, errInvalidSourceSize) {
		t.Fatalf("withDocument() error = %v, want %v", err, errInvalidSourceSize)
	}
	if got := log.snapshot(); len(got) != 0 {
		t.Fatalf("events = %v, want no PDFium calls", got)
	}
}

func TestRuntimeRejectsInvalidCalls(t *testing.T) {
	t.Parallel()

	log := &eventLog{}
	runtime := &Runtime{pool: &poolStub{log: log}}

	if err := runtime.withInstance(nil, func(instance) error { return nil }); !errors.Is(err, errNilContext) {
		t.Fatalf("withInstance(nil) error = %v, want %v", err, errNilContext)
	}
	if err := runtime.withInstance(context.Background(), nil); !errors.Is(err, errNilOperation) {
		t.Fatalf("withInstance(nil operation) error = %v, want %v", err, errNilOperation)
	}

	if err := runtime.Close(); err != nil {
		t.Fatalf("Close() returned an unexpected error: %v", err)
	}
	if err := runtime.withInstance(context.Background(), func(instance) error { return nil }); !errors.Is(err, errRuntimeClosed) {
		t.Fatalf("withInstance() after Close error = %v, want %v", err, errRuntimeClosed)
	}
}

func TestNewRuntimeRejectsCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewRuntime(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("NewRuntime() error = %v, want context cancellation", err)
	}
}
