package analyze_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/analyze"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/document"
)

func FuzzBasicAnalyzer(f *testing.F) {
	f.Add("ordinary text", uint16(10), uint16(10), uint16(50), uint16(12))
	f.Add("1. list item", uint16(0), uint16(0), uint16(80), uint16(10))
	f.Add("• unicode item", uint16(20), uint16(40), uint16(100), uint16(16))

	f.Fuzz(func(
		t *testing.T,
		input string,
		x uint16,
		y uint16,
		width uint16,
		height uint16,
	) {
		if len(input) > 4096 {
			t.Skip()
		}
		input = strings.ToValidUTF8(input, "\uFFFD")
		if input == "" {
			input = " "
		}

		left := float64(x % 500)
		top := float64(y % 700)
		runWidth := float64(width%500 + 1)
		runHeight := float64(height%100 + 1)
		layout := &document.Layout{
			Pages: []document.Page{
				{
					Number: 1,
					Width:  max(612, left+runWidth),
					Height: max(792, top+runHeight),
					TextRuns: []document.TextRun{
						{
							Text: input,
							Bounds: document.Rectangle{
								Left:   left,
								Top:    top,
								Right:  left + runWidth,
								Bottom: top + runHeight,
							},
							Style: document.TextStyle{
								FontName: "Fuzz Sans",
								FontSize: runHeight,
							},
						},
					},
				},
			},
		}

		result, err := analyze.NewBasicAnalyzer().Analyze(
			context.Background(),
			layout,
		)
		if err != nil {
			t.Fatalf("Analyze() returned an unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("Analyze() returned a nil document")
		}
		if err := result.Validate(); err != nil {
			t.Fatalf("Analyze() returned an invalid document: %v", err)
		}
	})
}
