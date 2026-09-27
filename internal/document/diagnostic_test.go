package document_test

import (
	"testing"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/document"
)

func TestDiagnosticValidate(t *testing.T) {
	t.Parallel()

	diagnostic := document.Diagnostic{
		Code:    document.DiagnosticTableLikeText,
		Page:    2,
		Message: "table-like text was preserved as paragraphs",
	}
	if err := diagnostic.Validate(); err != nil {
		t.Fatalf("Validate() returned an unexpected error: %v", err)
	}
}

func TestDiagnosticValidateRejectsMissingFields(t *testing.T) {
	t.Parallel()

	tests := []document.Diagnostic{
		{Page: 1, Message: "message"},
		{Code: document.DiagnosticCodeLikeText, Page: -1, Message: "message"},
		{Code: document.DiagnosticCodeLikeText, Page: 1},
	}
	for _, diagnostic := range tests {
		if err := diagnostic.Validate(); err == nil {
			t.Fatalf("Validate() returned nil for invalid diagnostic %+v", diagnostic)
		}
	}
}
