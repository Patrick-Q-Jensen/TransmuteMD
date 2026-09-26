package analyze_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/analyze"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/document"
)

type analyzerStub struct {
	document *document.Document
	err      error
}

func (s analyzerStub) Analyze(context.Context, *document.Layout) (*document.Document, error) {
	return s.document, s.err
}

var _ analyze.Analyzer = analyzerStub{}

func TestAnalyzerContractConnectsDocumentModels(t *testing.T) {
	t.Parallel()

	input := &document.Layout{}
	want := &document.Document{}
	var analyzer analyze.Analyzer = analyzerStub{document: want}

	got, err := analyzer.Analyze(context.Background(), input)
	if err != nil {
		t.Fatalf("Analyze() returned an unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("Analyze() = %p, want %p", got, want)
	}
}

func TestAnalyzerContractPreservesErrors(t *testing.T) {
	t.Parallel()

	want := errors.New("analysis failed")
	var analyzer analyze.Analyzer = analyzerStub{err: want}

	_, got := analyzer.Analyze(context.Background(), &document.Layout{})
	if !errors.Is(got, want) {
		t.Fatalf("Analyze() error = %v, want %v", got, want)
	}
}
