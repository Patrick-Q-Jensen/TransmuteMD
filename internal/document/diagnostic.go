package document

import (
	"errors"
	"fmt"
)

// DiagnosticCode identifies an engine-neutral conversion warning.
type DiagnosticCode string

const (
	DiagnosticUnsupportedLink DiagnosticCode = "unsupported-link"
	DiagnosticAmbiguousLink   DiagnosticCode = "ambiguous-link"
	DiagnosticTableLikeText   DiagnosticCode = "table-like-text"
	DiagnosticCodeLikeText    DiagnosticCode = "code-like-text"
)

// Diagnostic reports a non-fatal conversion fallback.
type Diagnostic struct {
	Code    DiagnosticCode
	Page    int
	Message string
}

// Validate checks diagnostic fields shared by layout and semantic documents.
func (d Diagnostic) Validate() error {
	if d.Code == "" {
		return errors.New("code must not be empty")
	}
	if d.Page < 0 {
		return errors.New("page must not be negative")
	}
	if d.Message == "" {
		return errors.New("message must not be empty")
	}
	return nil
}

func validateDiagnostics(diagnostics []Diagnostic) error {
	for index, diagnostic := range diagnostics {
		if err := diagnostic.Validate(); err != nil {
			return fmt.Errorf("diagnostic %d: %w", index+1, err)
		}
	}
	return nil
}
