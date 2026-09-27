package cli

import (
	"bytes"
	"testing"
)

func FuzzRecognizePDF(f *testing.F) {
	f.Add([]byte("%PDF-1.7\n"))
	f.Add([]byte("prefix\n%PDF-1.4\n"))
	f.Add([]byte("not a PDF"))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 4096 {
			t.Skip()
		}

		err := recognizePDF(bytes.NewReader(input), int64(len(input)))
		header := input[:min(len(input), 1024)]
		recognized := len(input) > 0 && bytes.Contains(header, []byte("%PDF-"))
		if recognized && err != nil {
			t.Fatalf("recognizePDF() error = %v for recognized header", err)
		}
		if !recognized && err == nil {
			t.Fatal("recognizePDF() accepted input without a PDF header")
		}
	})
}
