package extract

import (
	"context"
	"io"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/document"
)

// Source provides immutable random access to one input document. Size must be
// non-negative and the source must remain readable for the duration of Extract.
// Extractors do not close or otherwise take ownership of a source.
type Source interface {
	io.ReaderAt
	Size() int64
}

// Extractor converts a source into an engine-neutral physical layout.
//
// Implementations must honor context cancellation at meaningful boundaries,
// return contextual errors, and release all engine resources before returning.
// A successful result must be non-nil and pass document.Layout.Validate. The
// caller owns the returned layout.
type Extractor interface {
	// Name returns a stable identifier for diagnostics and engine selection.
	Name() string

	// Extract observes the physical content of source without inferring
	// semantic structure.
	Extract(ctx context.Context, source Source) (*document.Layout, error)
}
