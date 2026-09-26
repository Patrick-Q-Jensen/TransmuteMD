package analyze

import (
	"context"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/document"
)

// Analyzer converts a physical layout into semantic blocks in reading order.
//
// Implementations must not mutate the input layout. They must honor context
// cancellation at meaningful boundaries and return contextual errors. Inputs
// must pass document.Layout.Validate; successful results must be non-nil and
// pass document.Document.Validate. The caller owns the returned document.
type Analyzer interface {
	Analyze(ctx context.Context, layout *document.Layout) (*document.Document, error)
}
