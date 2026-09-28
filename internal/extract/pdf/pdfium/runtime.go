package pdfium

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	gopdfium "github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/responses"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
)

var (
	errNilContext    = errors.New("context must not be nil")
	errNilOperation  = errors.New("PDFium operation must not be nil")
	errRuntimeClosed = errors.New("PDFium runtime is closed")
)

type instance interface {
	OpenDocument(request *requests.OpenDocument) (*responses.OpenDocument, error)
	FPDF_CloseDocument(request *requests.FPDF_CloseDocument) (*responses.FPDF_CloseDocument, error)
	FPDF_GetPageCount(request *requests.FPDF_GetPageCount) (*responses.FPDF_GetPageCount, error)
	FPDF_GetPageSizeByIndex(request *requests.FPDF_GetPageSizeByIndex) (*responses.FPDF_GetPageSizeByIndex, error)
	GetPageTextStructured(request *requests.GetPageTextStructured) (*responses.GetPageTextStructured, error)
	FPDFPage_CountObjects(request *requests.FPDFPage_CountObjects) (*responses.FPDFPage_CountObjects, error)
	FPDFPage_GetObject(request *requests.FPDFPage_GetObject) (*responses.FPDFPage_GetObject, error)
	FPDFPageObj_GetType(request *requests.FPDFPageObj_GetType) (*responses.FPDFPageObj_GetType, error)
	FPDFPageObj_GetMatrix(request *requests.FPDFPageObj_GetMatrix) (*responses.FPDFPageObj_GetMatrix, error)
	FPDFPath_GetDrawMode(request *requests.FPDFPath_GetDrawMode) (*responses.FPDFPath_GetDrawMode, error)
	FPDFPath_CountSegments(request *requests.FPDFPath_CountSegments) (*responses.FPDFPath_CountSegments, error)
	FPDFPath_GetPathSegment(request *requests.FPDFPath_GetPathSegment) (*responses.FPDFPath_GetPathSegment, error)
	FPDFPathSegment_GetType(request *requests.FPDFPathSegment_GetType) (*responses.FPDFPathSegment_GetType, error)
	FPDFPathSegment_GetPoint(request *requests.FPDFPathSegment_GetPoint) (*responses.FPDFPathSegment_GetPoint, error)
	FPDFPathSegment_GetClose(request *requests.FPDFPathSegment_GetClose) (*responses.FPDFPathSegment_GetClose, error)
	FPDFPageObj_GetStrokeWidth(request *requests.FPDFPageObj_GetStrokeWidth) (*responses.FPDFPageObj_GetStrokeWidth, error)
	FPDFPage_GetAnnotCount(request *requests.FPDFPage_GetAnnotCount) (*responses.FPDFPage_GetAnnotCount, error)
	FPDFPage_GetAnnot(request *requests.FPDFPage_GetAnnot) (*responses.FPDFPage_GetAnnot, error)
	FPDFPage_CloseAnnot(request *requests.FPDFPage_CloseAnnot) (*responses.FPDFPage_CloseAnnot, error)
	FPDFAnnot_GetSubtype(request *requests.FPDFAnnot_GetSubtype) (*responses.FPDFAnnot_GetSubtype, error)
	FPDFAnnot_GetLink(request *requests.FPDFAnnot_GetLink) (*responses.FPDFAnnot_GetLink, error)
	FPDFAnnot_GetRect(request *requests.FPDFAnnot_GetRect) (*responses.FPDFAnnot_GetRect, error)
	FPDFLink_GetDest(request *requests.FPDFLink_GetDest) (*responses.FPDFLink_GetDest, error)
	FPDFLink_GetAction(request *requests.FPDFLink_GetAction) (*responses.FPDFLink_GetAction, error)
	FPDFAction_GetType(request *requests.FPDFAction_GetType) (*responses.FPDFAction_GetType, error)
	FPDFAction_GetDest(request *requests.FPDFAction_GetDest) (*responses.FPDFAction_GetDest, error)
	FPDFAction_GetURIPath(request *requests.FPDFAction_GetURIPath) (*responses.FPDFAction_GetURIPath, error)
	FPDFDest_GetDestPageIndex(request *requests.FPDFDest_GetDestPageIndex) (*responses.FPDFDest_GetDestPageIndex, error)
	Close() error
	Kill() error
}

type instancePool interface {
	getInstance(ctx context.Context) (instance, error)
	Close() error
}

type upstreamPool struct {
	gopdfium.Pool
}

func (p upstreamPool) getInstance(ctx context.Context) (instance, error) {
	return p.GetInstanceWithContext(ctx)
}

// Runtime owns the process-scoped PDFium WebAssembly pool.
type Runtime struct {
	mu       sync.RWMutex
	pool     instancePool
	closed   bool
	closeErr error
}

// NewRuntime initializes the embedded PDFium WebAssembly backend.
func NewRuntime(ctx context.Context) (*Runtime, error) {
	if ctx == nil {
		return nil, errNilContext
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("initialize PDFium runtime: %w", err)
	}

	runtimeConfig := wazero.NewRuntimeConfig().
		WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling).
		WithCloseOnContextDone(true)

	pool, err := webassembly.Init(webassembly.Config{
		Context:       ctx,
		MinIdle:       0,
		MaxIdle:       1,
		MaxTotal:      1,
		FSConfig:      wazero.NewFSConfig(),
		RuntimeConfig: runtimeConfig,
		Stdout:        io.Discard,
		Stderr:        io.Discard,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize PDFium runtime: %w", err)
	}

	return &Runtime{pool: upstreamPool{Pool: pool}}, nil
}

// Close releases the PDFium pool and its WebAssembly runtime. It is safe to
// call Close more than once; every call returns the first close result.
func (r *Runtime) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return r.closeErr
	}
	r.closed = true
	r.closeErr = r.pool.Close()
	if r.closeErr != nil {
		r.closeErr = fmt.Errorf("close PDFium runtime: %w", r.closeErr)
	}
	return r.closeErr
}

func (r *Runtime) withInstance(
	ctx context.Context,
	operation func(instance) error,
) (err error) {
	if ctx == nil {
		return errNilContext
	}
	if operation == nil {
		return errNilOperation
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return errRuntimeClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	worker, err := r.pool.getInstance(ctx)
	if err != nil {
		return fmt.Errorf("acquire PDFium instance: %w", err)
	}

	killDone := make(chan error, 1)
	stopKill := context.AfterFunc(ctx, func() {
		killDone <- worker.Kill()
	})

	err = operation(worker)
	if stopKill() {
		if closeErr := worker.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close PDFium instance: %w", closeErr))
		}
	} else {
		if killErr := <-killDone; killErr != nil {
			err = errors.Join(err, fmt.Errorf("kill cancelled PDFium instance: %w", killErr))
		}
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		err = errors.Join(err, ctxErr)
	}
	return err
}
