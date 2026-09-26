package render

import (
	"context"
	"io"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/document"
)

// Renderer writes a semantic document in an output format.
//
// Implementations must not mutate the document. They must honor context
// cancellation at meaningful boundaries, return contextual errors, and accept
// only documents that pass document.Document.Validate. A renderer may write
// before returning an error; callers that require transactional output must
// render into a private buffer before committing bytes to user-visible output.
// The caller retains ownership of the writer and is responsible for closing it.
type Renderer interface {
	Render(ctx context.Context, doc *document.Document, output io.Writer) error
}
