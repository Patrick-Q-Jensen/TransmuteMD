package pdfium

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rc4"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/extract"
)

func TestExtractorHandlesDifficultPDFInputs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	runtime, err := NewRuntime(ctx)
	if err != nil {
		t.Fatalf("NewRuntime() returned an unexpected error: %v", err)
	}
	defer func() {
		if err := runtime.Close(); err != nil {
			t.Errorf("close PDFium runtime: %v", err)
		}
	}()

	extractor, err := NewExtractor(runtime)
	if err != nil {
		t.Fatalf("NewExtractor() returned an unexpected error: %v", err)
	}

	t.Run("malformed", func(t *testing.T) {
		_, err := extractor.Extract(
			ctx,
			bytes.NewReader([]byte("%PDF-1.4\n1 0 obj\nbroken\n%%EOF\n")),
		)
		if !errors.Is(err, extract.ErrInvalidDocument) {
			t.Fatalf("Extract() error = %v, want %v", err, extract.ErrInvalidDocument)
		}
	})

	t.Run("encrypted", func(t *testing.T) {
		_, err := extractor.Extract(
			ctx,
			bytes.NewReader(buildEncryptedTestPDF()),
		)
		if !errors.Is(err, extract.ErrEncryptedDocument) {
			t.Fatalf("Extract() error = %v, want %v", err, extract.ErrEncryptedDocument)
		}
	})

	t.Run("rotated page and subset font name", func(t *testing.T) {
		layout, err := extractor.Extract(
			ctx,
			bytes.NewReader(buildRotatedSubsetFontTestPDF()),
		)
		if err != nil {
			t.Fatalf("Extract() returned an unexpected error: %v", err)
		}
		if len(layout.Pages) != 1 {
			t.Fatalf("page count = %d, want 1", len(layout.Pages))
		}
		page := layout.Pages[0]
		if page.Width != 792 || page.Height != 612 {
			t.Fatalf(
				"rotated page size = %gx%g, want 792x612",
				page.Width,
				page.Height,
			)
		}
		var text strings.Builder
		for _, run := range page.TextRuns {
			if run.Style.FontName != "ABCDEF+Helvetica" {
				t.Fatalf(
					"font name = %q, want subset font name",
					run.Style.FontName,
				)
			}
			text.WriteString(run.Text)
		}
		if got, want := text.String(), "Rotated subset font"; got != want {
			t.Fatalf("extracted text = %q, want %q", got, want)
		}
	})

	t.Run("image only", func(t *testing.T) {
		layout, err := extractor.Extract(
			ctx,
			bytes.NewReader(buildImageOnlyTestPDF()),
		)
		if err != nil {
			t.Fatalf("Extract() returned an unexpected error: %v", err)
		}
		if len(layout.Pages) != 1 {
			t.Fatalf("page count = %d, want 1", len(layout.Pages))
		}
		if got := len(layout.Pages[0].TextRuns); got != 0 {
			t.Fatalf("text run count = %d, want 0", got)
		}
	})
}

func buildRotatedSubsetFontTestPDF() []byte {
	content := []byte("BT\n/F1 12 Tf\n72 72 Td\n(Rotated subset font) Tj\nET\n")
	return writeTestPDF(
		[][]byte{
			[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
			[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
			[]byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Rotate 90 " +
				"/Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>"),
			streamObject(content),
			[]byte("<< /Type /Font /Subtype /Type1 /BaseFont /ABCDEF+Helvetica >>"),
		},
		"",
	)
}

func buildImageOnlyTestPDF() []byte {
	content := []byte("q\n100 0 0 100 72 600 cm\n/Im0 Do\nQ\n")
	image := append(
		[]byte("<< /Type /XObject /Subtype /Image /Width 1 /Height 1 "+
			"/ColorSpace /DeviceRGB /BitsPerComponent 8 /Length 3 >>\nstream\n"),
		0xff, 0, 0,
	)
	image = append(image, []byte("\nendstream")...)
	return writeTestPDF(
		[][]byte{
			[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
			[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
			[]byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] " +
				"/Resources << /XObject << /Im0 5 0 R >> >> /Contents 4 0 R >>"),
			streamObject(content),
			image,
		},
		"",
	)
}

func buildEncryptedTestPDF() []byte {
	permissions := int32(-4)

	documentID := md5.Sum([]byte("TransmuteMD encrypted test PDF"))
	owner := paddedPassword("owner")
	user := paddedPassword("secret")

	ownerDigest := md5.Sum(owner)
	ownerValue := applyRC4(ownerDigest[:5], user)

	var permissionsBytes [4]byte
	binary.LittleEndian.PutUint32(permissionsBytes[:], uint32(permissions))
	keyInput := append(paddedPassword("secret"), ownerValue...)
	keyInput = append(keyInput, permissionsBytes[:]...)
	keyInput = append(keyInput, documentID[:]...)
	keyDigest := md5.Sum(keyInput)
	encryptionKey := keyDigest[:5]
	userValue := applyRC4(encryptionKey, pdfPasswordPadding())

	content := []byte("BT\n/F1 12 Tf\n72 720 Td\n(Encrypted text) Tj\nET\n")
	content = applyRC4(objectEncryptionKey(encryptionKey, 4), content)
	encryptDictionary := fmt.Sprintf(
		"<< /Filter /Standard /V 1 /R 2 /Length 40 /O <%s> /U <%s> /P %d >>",
		hex.EncodeToString(ownerValue),
		hex.EncodeToString(userValue),
		permissions,
	)
	id := hex.EncodeToString(documentID[:])
	return writeTestPDF(
		[][]byte{
			[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
			[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
			[]byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] " +
				"/Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>"),
			streamObject(content),
			[]byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
			[]byte(encryptDictionary),
		},
		fmt.Sprintf("/Encrypt 6 0 R /ID [<%s> <%s>]", id, id),
	)
}

func streamObject(content []byte) []byte {
	result := []byte(fmt.Sprintf("<< /Length %d >>\nstream\n", len(content)))
	result = append(result, content...)
	return append(result, []byte("\nendstream")...)
}

func writeTestPDF(objects [][]byte, trailerEntries string) []byte {
	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	offsets := make([]int, len(objects))
	for index, object := range objects {
		offsets[index] = pdf.Len()
		fmt.Fprintf(&pdf, "%d 0 obj\n", index+1)
		pdf.Write(object)
		pdf.WriteString("\nendobj\n")
	}

	xrefOffset := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n", len(objects)+1)
	pdf.WriteString("0000000000 65535 f \n")
	for _, offset := range offsets {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(
		&pdf,
		"trailer\n<< /Size %d /Root 1 0 R %s >>\nstartxref\n%d\n%%%%EOF\n",
		len(objects)+1,
		trailerEntries,
		xrefOffset,
	)
	return pdf.Bytes()
}

func paddedPassword(password string) []byte {
	padding := pdfPasswordPadding()
	result := make([]byte, 32)
	copied := copy(result, []byte(password))
	copy(result[copied:], padding)
	return result
}

func pdfPasswordPadding() []byte {
	return []byte{
		0x28, 0xbf, 0x4e, 0x5e, 0x4e, 0x75, 0x8a, 0x41,
		0x64, 0x00, 0x4e, 0x56, 0xff, 0xfa, 0x01, 0x08,
		0x2e, 0x2e, 0x00, 0xb6, 0xd0, 0x68, 0x3e, 0x80,
		0x2f, 0x0c, 0xa9, 0xfe, 0x64, 0x53, 0x69, 0x7a,
	}
}

func objectEncryptionKey(documentKey []byte, objectNumber int) []byte {
	input := append([]byte(nil), documentKey...)
	input = append(
		input,
		byte(objectNumber),
		byte(objectNumber>>8),
		byte(objectNumber>>16),
		0,
		0,
	)
	digest := md5.Sum(input)
	return digest[:min(len(documentKey)+5, len(digest))]
}

func applyRC4(key, content []byte) []byte {
	cipher, err := rc4.NewCipher(key)
	if err != nil {
		panic(err)
	}
	result := make([]byte, len(content))
	cipher.XORKeyStream(result, content)
	return result
}
