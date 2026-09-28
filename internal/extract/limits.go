package extract

import (
	"errors"
	"fmt"
)

const (
	defaultMaxSourceBytes   int64 = 256 << 20
	defaultMaxPages               = 2_000
	defaultMaxTextRuns            = 1_000_000
	defaultMaxAnnotations         = 100_000
	defaultMaxPageObjects         = 1_000_000
	defaultMaxPathSegments        = 2_000_000
	defaultMaxRulings             = 1_000_000
	defaultMaxPageDimension       = 200_000
)

// ErrLimitExceeded indicates that conversion stopped at a configured safety
// boundary.
var ErrLimitExceeded = errors.New("document exceeds conversion limits")

// Limits bounds extraction work and engine-neutral layout growth.
type Limits struct {
	MaxSourceBytes   int64
	MaxPages         int
	MaxTextRuns      int
	MaxAnnotations   int
	MaxPageObjects   int
	MaxPathSegments  int
	MaxRulings       int
	MaxPageDimension int
}

// DefaultLimits returns the safety limits used by the CLI.
func DefaultLimits() Limits {
	return Limits{
		MaxSourceBytes:   defaultMaxSourceBytes,
		MaxPages:         defaultMaxPages,
		MaxTextRuns:      defaultMaxTextRuns,
		MaxAnnotations:   defaultMaxAnnotations,
		MaxPageObjects:   defaultMaxPageObjects,
		MaxPathSegments:  defaultMaxPathSegments,
		MaxRulings:       defaultMaxRulings,
		MaxPageDimension: defaultMaxPageDimension,
	}
}

// Validate checks that every safety boundary is enabled.
func (limits Limits) Validate() error {
	if limits.MaxSourceBytes <= 0 {
		return errors.New("maximum source bytes must be greater than zero")
	}
	if limits.MaxPages <= 0 {
		return errors.New("maximum pages must be greater than zero")
	}
	if limits.MaxTextRuns <= 0 {
		return errors.New("maximum text runs must be greater than zero")
	}
	if limits.MaxAnnotations <= 0 {
		return errors.New("maximum annotations must be greater than zero")
	}
	if limits.MaxPageObjects <= 0 {
		return errors.New("maximum page objects must be greater than zero")
	}
	if limits.MaxPathSegments <= 0 {
		return errors.New("maximum path segments must be greater than zero")
	}
	if limits.MaxRulings <= 0 {
		return errors.New("maximum rulings must be greater than zero")
	}
	if limits.MaxPageDimension <= 0 {
		return errors.New("maximum page dimension must be greater than zero")
	}
	return nil
}

// LimitError adds the observed value and configured boundary to a limit error.
func LimitError(resource string, observed, maximum int64) error {
	return fmt.Errorf(
		"%w: %s %d exceeds maximum %d",
		ErrLimitExceeded,
		resource,
		observed,
		maximum,
	)
}
