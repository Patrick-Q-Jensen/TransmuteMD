package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
)

func main() {
	output := flag.String("output", "testdata/pdf/generated/simple.pdf", "generated PDF path")
	fixture := flag.String("fixture", "simple", "fixture to generate: simple or phase2")
	flag.Parse()

	var content []byte
	switch *fixture {
	case "simple":
		content = buildPDF()
	case "phase2":
		content = buildPhase2PDF()
	default:
		fmt.Fprintf(os.Stderr, "generate PDF: unknown fixture %q\n", *fixture)
		os.Exit(2)
	}
	if err := os.WriteFile(*output, content, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "generate PDF: %v\n", err)
		os.Exit(1)
	}
}

func buildPhase2PDF() []byte {
	page1 := "BT\n/F1 10 Tf\n72 770 Td\n(TransmuteMD Phase 2) Tj\nET\n" +
		"BT\n/F2 24 Tf\n72 730 Td\n(Structured document) Tj\nET\n" +
		"BT\n/F1 12 Tf\n72 690 Td\n(A wrapped paragraph begins here and) Tj\n" +
		"0 -16 Td\n(continues on the following line.) Tj\nET\n" +
		"BT\n/F1 12 Tf\n72 630 Td\n(- First unordered item) Tj\n" +
		"0 -16 Td\n(- Second unordered item) Tj\nET\n" +
		"BT\n/F2 20 Tf\n72 570 Td\n(Two-column section across the page) Tj\nET\n" +
		"BT\n/F1 12 Tf\n72 530 Td\n(Left column begins) Tj\n" +
		"0 -16 Td\n(Left column continues.) Tj\nET\n" +
		"BT\n/F1 12 Tf\n330 530 Td\n(Right column begins) Tj\n" +
		"0 -16 Td\n(Right column continues.) Tj\nET\n" +
		"BT\n/F1 12 Tf\n72 460 Td\n(Visit the project site for more information about TransmuteMD.) Tj\nET\n" +
		"BT\n/F1 10 Tf\n280 20 Td\n(Page 1) Tj\nET\n"
	page2 := "BT\n/F1 10 Tf\n72 770 Td\n(TransmuteMD Phase 2) Tj\nET\n" +
		"BT\n/F2 18 Tf\n72 730 Td\n(Numbered steps) Tj\nET\n" +
		"BT\n/F1 12 Tf\n72 690 Td\n(3. Prepare the input.) Tj\n" +
		"0 -16 Td\n(4. Convert the document.) Tj\nET\n" +
		"BT\n/F1 12 Tf\n72 630 Td\n(The sequence keeps its original starting number.) Tj\nET\n" +
		"BT\n/F1 10 Tf\n280 20 Td\n(Page 2) Tj\nET\n"
	page3 := "BT\n/F1 10 Tf\n72 770 Td\n(TransmuteMD Phase 2) Tj\nET\n" +
		"BT\n/F2 18 Tf\n72 730 Td\n(Preserved fallbacks) Tj\nET\n" +
		"BT\n/F1 12 Tf\n72 680 Td\n(Name) Tj\n78 0 Td\n(Value) Tj\n80 0 Td\n(Status) Tj\nET\n" +
		"BT\n/F1 12 Tf\n72 664 Td\n(Alpha) Tj\n78 0 Td\n(One) Tj\n80 0 Td\n(Ready) Tj\nET\n" +
		"BT\n/F1 12 Tf\n72 648 Td\n(Beta) Tj\n78 0 Td\n(Two) Tj\n80 0 Td\n(Done) Tj\nET\n" +
		"BT\n/F3 11 Tf\n72 570 Td\n(func main\\(\\) {) Tj\n" +
		"0 -14 Td\n(    return) Tj\n" +
		"0 -14 Td\n(}) Tj\nET\n" +
		"BT\n/F1 10 Tf\n280 20 Td\n(Page 3) Tj\nET\n"

	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R 6 0 R 8 0 R] /Count 3 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] " +
			"/Resources << /Font << /F1 10 0 R /F2 11 0 R /F3 12 0 R >> >> " +
			"/Contents 4 0 R /Annots [5 0 R] >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(page1), page1),
		"<< /Type /Annot /Subtype /Link /Rect [72 454 405 474] " +
			"/Border [0 0 0] /A << /S /URI /URI (https://example.test/docs) >> >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] " +
			"/Resources << /Font << /F1 10 0 R /F2 11 0 R /F3 12 0 R >> >> " +
			"/Contents 7 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(page2), page2),
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] " +
			"/Resources << /Font << /F1 10 0 R /F2 11 0 R /F3 12 0 R >> >> " +
			"/Contents 9 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(page3), page3),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>",
	}
	return writePDF(objects)
}

func buildPDF() []byte {
	content := "BT\n/F1 12 Tf\n72 720 Td\n(TransmuteMD PDFium smoke test) Tj\nET\n"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] " +
			"/Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(content), content),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}

	return writePDF(objects)
}

func writePDF(objects []string) []byte {
	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	offsets := make([]int, len(objects))
	for i, object := range objects {
		offsets[i] = pdf.Len()
		fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}

	xrefOffset := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n", len(objects)+1)
	pdf.WriteString("0000000000 65535 f \n")
	for _, offset := range offsets {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(
		&pdf,
		"trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		len(objects)+1,
		xrefOffset,
	)
	return pdf.Bytes()
}
