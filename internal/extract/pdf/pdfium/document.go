package pdfium

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/extract"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
)

var (
	errNilSource         = errors.New("PDF source must not be nil")
	errEmptySource       = errors.New("PDF source must not be empty")
	errInvalidSourceSize = errors.New("PDF source size must not be negative")
)

type documentOperation func(instance, references.FPDF_DOCUMENT) error

func (r *Runtime) withDocument(
	ctx context.Context,
	source extract.Source,
	operation documentOperation,
) error {
	if source == nil {
		return errNilSource
	}
	if operation == nil {
		return errNilOperation
	}

	size := source.Size()
	if size < 0 {
		return errInvalidSourceSize
	}
	if size == 0 {
		return errEmptySource
	}

	reader := io.NewSectionReader(source, 0, size)
	return r.withInstance(ctx, func(worker instance) (err error) {
		opened, err := worker.OpenDocument(&requests.OpenDocument{
			FileReader:     reader,
			FileReaderSize: size,
		})
		if err != nil {
			return fmt.Errorf("open PDF document: %w", err)
		}

		defer func() {
			_, closeErr := worker.FPDF_CloseDocument(&requests.FPDF_CloseDocument{
				Document: opened.Document,
			})
			if closeErr != nil {
				err = errors.Join(err, fmt.Errorf("close PDF document: %w", closeErr))
			}
		}()

		return operation(worker, opened.Document)
	})
}
