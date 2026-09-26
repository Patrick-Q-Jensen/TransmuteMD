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
