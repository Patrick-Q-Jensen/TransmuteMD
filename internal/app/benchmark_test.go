package app_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/analyze"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/app"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/extract/pdf/pdfium"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/render/markdown"
)

func BenchmarkGeneratedPDFConversion(b *testing.B) {
	ctx := context.Background()
	pdfRuntime, err := pdfium.NewRuntime(ctx)
	if err != nil {
		b.Fatalf("NewRuntime() returned an unexpected error: %v", err)
	}
	b.Cleanup(func() {
		if err := pdfRuntime.Close(); err != nil {
			b.Errorf("close PDFium runtime: %v", err)
		}
	})

	extractor, err := pdfium.NewExtractor(pdfRuntime)
	if err != nil {
		b.Fatalf("NewExtractor() returned an unexpected error: %v", err)
	}
	converter, err := app.NewConverter(
		extractor,
		analyze.NewBasicAnalyzer(),
		markdown.NewRenderer(),
	)
	if err != nil {
		b.Fatalf("NewConverter() returned an unexpected error: %v", err)
	}

	for _, fixture := range []string{"simple.pdf", "phase2.pdf"} {
		b.Run(fixture, func(b *testing.B) {
			source, err := os.ReadFile(filepath.Join(
				repositoryRoot(b),
				"testdata",
				"pdf",
				"generated",
				fixture,
			))
			if err != nil {
				b.Fatalf("read generated PDF fixture: %v", err)
			}

			run := func() {
				var output bytes.Buffer
				destination, err := app.NewWriterDestination(&output)
				if err != nil {
					b.Fatalf("NewWriterDestination() returned an unexpected error: %v", err)
				}
				if err := converter.Convert(
					ctx,
					bytes.NewReader(source),
					destination,
				); err != nil {
					b.Fatalf("Convert() returned an unexpected error: %v", err)
				}
			}

			peakHeap := measurePeakHeap(run)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				run()
			}
			b.ReportMetric(float64(peakHeap), "peak-Go-heap-bytes")
		})
	}
}

func measurePeakHeap(operation func()) uint64 {
	runtime.GC()
	var baseline runtime.MemStats
	runtime.ReadMemStats(&baseline)

	var peak atomic.Uint64
	peak.Store(baseline.HeapInuse)
	done := make(chan struct{})
	result := make(chan uint64, 1)
	go func() {
		for {
			var current runtime.MemStats
			runtime.ReadMemStats(&current)
			peak.Store(max(peak.Load(), current.HeapInuse))
			select {
			case <-done:
				result <- peak.Load()
				return
			default:
				runtime.Gosched()
			}
		}
	}()

	operation()
	close(done)
	observed := <-result
	if observed <= baseline.HeapInuse {
		return 0
	}
	return observed - baseline.HeapInuse
}
