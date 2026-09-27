package extract_test

import (
	"errors"
	"testing"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/extract"
)

func TestDefaultLimitsAreValid(t *testing.T) {
	t.Parallel()

	if err := extract.DefaultLimits().Validate(); err != nil {
		t.Fatalf("DefaultLimits().Validate() returned an unexpected error: %v", err)
	}
}

func TestLimitsValidateRejectsDisabledBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		disable func(*extract.Limits)
	}{
		{
			name: "source bytes",
			disable: func(limits *extract.Limits) {
				limits.MaxSourceBytes = 0
			},
		},
		{
			name: "pages",
			disable: func(limits *extract.Limits) {
				limits.MaxPages = 0
			},
		},
		{
			name: "text runs",
			disable: func(limits *extract.Limits) {
				limits.MaxTextRuns = 0
			},
		},
		{
			name: "annotations",
			disable: func(limits *extract.Limits) {
				limits.MaxAnnotations = 0
			},
		},
		{
			name: "page dimension",
			disable: func(limits *extract.Limits) {
				limits.MaxPageDimension = 0
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			limits := extract.DefaultLimits()
			test.disable(&limits)
			if err := limits.Validate(); err == nil {
				t.Fatal("Validate() returned nil for a disabled limit")
			}
		})
	}
}

func TestLimitErrorPreservesCategory(t *testing.T) {
	t.Parallel()

	err := extract.LimitError("pages", 3, 2)
	if !errors.Is(err, extract.ErrLimitExceeded) {
		t.Fatalf("LimitError() = %v, want %v", err, extract.ErrLimitExceeded)
	}
}
