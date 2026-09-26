package extract_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/document"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/extract"
)

type sourceStub struct {
	*bytes.Reader
}

func (s sourceStub) Size() int64 {
	return s.Reader.Size()
}

type extractorStub struct {
	layout *document.Layout
	err    error
}

func (extractorStub) Name() string {
	return "stub"
}

func (s extractorStub) Extract(context.Context, extract.Source) (*document.Layout, error) {
	return s.layout, s.err
}

var (
	_ extract.Source    = sourceStub{}
	_ extract.Extractor = extractorStub{}
)

func TestExtractorContractAcceptsNeutralSourceAndLayout(t *testing.T) {
	t.Parallel()

	source := sourceStub{Reader: bytes.NewReader([]byte("source"))}
	want := &document.Layout{}
	var extractor extract.Extractor = extractorStub{layout: want}

	got, err := extractor.Extract(context.Background(), source)
	if err != nil {
		t.Fatalf("Extract() returned an unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("Extract() = %p, want %p", got, want)
	}
	if got := extractor.Name(); got != "stub" {
		t.Fatalf("Name() = %q, want %q", got, "stub")
	}
}

func TestExtractorContractPreservesErrors(t *testing.T) {
	t.Parallel()

	want := errors.New("extract failed")
	var extractor extract.Extractor = extractorStub{err: want}

	_, got := extractor.Extract(context.Background(), sourceStub{Reader: bytes.NewReader(nil)})
	if !errors.Is(got, want) {
		t.Fatalf("Extract() error = %v, want %v", got, want)
	}
}
