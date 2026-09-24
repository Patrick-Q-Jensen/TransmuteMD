# ADR-0002: Use embedded PDFium WebAssembly for initial PDF extraction

- **Status:** Accepted
- **Date:** 2026-09-24

## Context

PDF syntax, font decoding, malformed-file handling, and text geometry are too
complex to implement safely and accurately as part of the initial project.
TransmuteMD needs a mature extraction engine while preserving the requirement
that users install only one executable.

Pure-Go libraries are portable but currently provide less mature extraction
for difficult PDFs. Native PDFium offers strong extraction but normally adds
platform-specific build and runtime concerns.

## Decision

Use `go-pdfium` with its embedded PDFium WebAssembly backend for the initial
extractor. Package the WebAssembly module inside each TransmuteMD executable so
PDFium is not an end-user prerequisite.

Contain `go-pdfium` and PDFium types within
`internal/extract/pdf/pdfium`. Translate extraction results into
engine-neutral document models before returning them to the application.

## Consequences

- Users receive one executable with no separately installed PDF engine.
- TransmuteMD benefits from PDFium's mature parsing and text extraction.
- Go builds avoid CGo and native shared-library deployment.
- The embedded module increases artifact size.
- WebAssembly execution is slower and may retain more memory than native
  PDFium.
- PDFium and its third-party license notices must be reviewed and included in
  releases as required.
- A future native PDFium backend can improve performance without changing
  analysis or rendering.

## Alternatives considered

- **Native PDFium:** potentially faster, but requires more platform-specific
  build and distribution work.
- **Pure-Go extraction:** simpler binary construction, but less proven
  extraction quality for the initial requirements.
- **MuPDF:** mature and portable, but its AGPL or commercial licensing model is
  less suitable for the chosen MIT-licensed project.
- **External tools such as `pdftotext`:** mature, but violate the
  no-additional-installation requirement unless separately bundled.
- **A custom PDF parser:** excessive scope and security risk for the project.

