# Architecture

## 1. Purpose

TransmuteMD converts source documents into readable Markdown. Its first input
format is PDF, using PDFium through the `go-pdfium` WebAssembly backend.

The architecture separates document decoding from interpretation and output
generation. This allows the extraction engine, layout heuristics, and Markdown
formatting to evolve independently.

## 2. Goals

- Distribute a portable executable that requires no separately installed
  runtime or PDF engine.
- Produce useful Markdown from born-digital PDFs.
- Allow extraction engines to be replaced or added without affecting the rest
  of the application.
- Make conversion behavior testable independently of PDFium.
- Report unsupported, encrypted, malformed, and image-only documents clearly.
- Keep the initial implementation small while leaving explicit extension
  points for other document formats.

## 3. Initial non-goals

- Optical character recognition for scanned PDFs.
- Perfect reproduction of every PDF's visual layout.
- Editing or generating PDFs.
- A stable public Go library API.
- Remote conversion services or a graphical interface.

These constraints may change through documented decisions and plan updates.

## 4. System context

```text
                 +------------------+
PDF file/stdin ->| TransmuteMD CLI  |-> Markdown file/stdout
                 +------------------+
                          |
                          v
                 PDFium WASM embedded
                 in the executable
```

The CLI is the initial user-facing interface. The PDFium WebAssembly module is
part of the distributed executable and is not an end-user prerequisite.

## 5. Conversion pipeline

```text
Input
  |
  v
Format selection
  |
  v
PDF extractor --------> engine-neutral layout document
  |
  v
Layout analyzer ------> semantic document
  |
  v
Markdown renderer ----> Markdown
  |
  v
Output
```

### Input and format selection

The CLI validates arguments and opens the source. Format selection determines
which extractor to create. Initially only PDF is supported, but orchestration
must not contain PDFium-specific behavior.

### Extraction

An extractor decodes a source into an engine-neutral layout model containing
pages, positioned text, and relevant style information. The PDFium adapter
owns all PDFium initialization, handles, response types, and cleanup.

Extraction does not decide whether text is a heading, paragraph, or list. It
reports observed layout information and diagnostics.

### Analysis

The analyzer converts physical layout into semantic content. Responsibilities
include:

- determining reading order;
- joining glyphs or text runs into lines and paragraphs;
- identifying likely headings and lists;
- suppressing repeated headers and footers where confidence is sufficient;
- preserving uncertain content instead of silently discarding it.

Analysis should be deterministic and operate entirely on engine-neutral
models, allowing focused unit tests without loading a PDF.

### Rendering

The renderer converts the semantic document to Markdown. It owns Markdown
escaping, whitespace rules, and syntax choices, but no PDF-specific behavior.

## 6. Package boundaries

```text
cmd/transmutemd
        |
        v
  internal/cli
        |
        v
  internal/app
   /     |      \
  v      v       v
extract analyze render
   |      |       |
   +------+-------+
          v
 internal/document
```

Dependencies point inward toward contracts and models:

- `cmd/transmutemd` performs process setup and dependency wiring.
- `internal/cli` translates command-line input into an application request and
  application results into user-facing output and exit codes.
- `internal/app` coordinates extraction, analysis, and rendering.
- `internal/extract` defines extraction contracts.
- `internal/extract/pdf/pdfium` implements the contracts using `go-pdfium`.
- `internal/analyze` defines the analysis contract and interprets layout
  without importing an extraction engine.
- `internal/render` defines the rendering contract.
- `internal/render/markdown` renders semantic content without importing PDF
  packages.
- `internal/document` contains engine-neutral data structures shared by the
  pipeline.

Neither `internal/document`, `internal/analyze`, nor
`internal/render/markdown` may import PDFium packages.

## 7. Core contracts

Every pipeline stage accepts a context, returns errors rather than terminating
the process, and communicates through engine-neutral document models.

```go
type Source interface {
	io.ReaderAt
	Size() int64
}

type Extractor interface {
	Name() string
	Extract(
		ctx context.Context,
		source Source,
	) (*document.Layout, error)
}

type Analyzer interface {
	Analyze(
		ctx context.Context,
		layout *document.Layout,
	) (*document.Document, error)
}

type Renderer interface {
	Render(
		ctx context.Context,
		doc *document.Document,
		output io.Writer,
	) error
}
```

Sources remain owned by the caller and must provide immutable random access
during extraction. Stages do not mutate their input models; callers own
successful results. Inputs and successful results must pass their model
validation. Implementations check cancellation at meaningful boundaries,
release acquired resources before returning, and add operation context to
errors.

Renderers own neither the semantic document nor the writer and do not close
the writer. Because a renderer may write before encountering an error,
application orchestration renders into a private buffer before committing
bytes to a file or standard output. This preserves the CLI's transactional
output contract without forcing every renderer to buffer independently.

Stage-specific option types will be introduced only when a concrete behavior
requires them.

The layout model represents observed facts rather than inferred semantics.
Page dimensions and coordinates use PDF points (1/72 inch) in a top-left
coordinate system: X increases rightward and Y increases downward. Extractor
adapters normalize their engine coordinates to this convention.

```go
type Layout struct {
	Pages []Page
}

type Page struct {
	Number   int
	Width    float64
	Height   float64
	TextRuns []TextRun
}

type TextRun struct {
	Text            string
	Bounds          Rectangle
	Style           TextStyle
	RotationDegrees float64
}
```

Text runs are consecutive text sharing placement and style evidence. They
retain extraction order; the analyzer, rather than the extractor, determines
reading order. Style fields use zero values when unavailable so engines are
not required to expose backend-specific font data.

The semantic model is an ordered set of blocks owned by `internal/document`.
The initial block is a plain paragraph. Later heading and list work will add
block types without exposing extraction details to renderers.

## 8. Engine selection and lifecycle

The first engine identifier is `pdfium-wasm`. Engine construction belongs in a
small factory or composition layer, not in conversion logic. A future
`pdfium-native` or pure-Go implementation must satisfy the same extractor
contract.

The adapter is responsible for:

- creating and closing the PDFium pool or instance;
- opening and closing documents and pages;
- checking cancellation between meaningful operations;
- translating PDFium output into internal coordinates and units;
- wrapping errors with operation and page context;
- detecting password-protected and likely image-only documents where possible.

The `go-pdfium` WebAssembly pool is process-scoped and limited to one worker
for the initial single-document CLI. Each conversion borrows one instance.
Pages and the document close before the instance, and the instance closes
before the process-scoped pool. Cleanup errors are preserved alongside the
primary operation error.

The adapter supplies an empty wazero filesystem configuration rather than
accepting `go-pdfium`'s default host-filesystem mount. Input is provided
through `extract.Source`, adapted from `io.ReaderAt` to `io.ReadSeeker` with an
`io.SectionReader`; PDFium therefore needs neither a host path nor an
application-level copy of the complete file.

Wazero is configured to terminate active WebAssembly execution when its
worker context is cancelled. Acquiring an instance uses the conversion
context, and cancellation kills that instance rather than returning it for
reuse. PDFium and wazero output streams are discarded so only the CLI writes
user-visible diagnostics.

## 9. Errors and diagnostics

Expected error categories include:

- invalid command-line input;
- unsupported input format;
- unreadable input or unwritable output;
- encrypted PDF or incorrect password;
- malformed or unsupported PDF;
- extraction failure;
- no extractable text;
- analysis or rendering failure.

Library packages return wrapped errors; only the CLI formats them for users
and selects exit codes. Partial output must not be presented as successful
unless an explicit future option permits best-effort conversion and reports
the omissions. The initial error categories and process exit codes are defined
in the [CLI contract](cli-contract.md).

## 10. Testing strategy

- **Unit tests:** contracts, model validation, semantic analysis, coordinate
  normalization, Markdown escaping, and command-line validation.
- **Contract tests:** run every extractor implementation against common
  expectations.
- **Backend smoke tests:** initialize the embedded PDFium WebAssembly module,
  open a generated licensed fixture, inspect it, and verify cleanup.
- **Golden tests:** compare Markdown output for stable fixture PDFs.
- **Integration tests:** execute the complete CLI with temporary input and
  output paths.
- **Corpus tests:** evaluate multi-column text, unusual fonts, ligatures,
  rotation, tables, forms, encryption, malformed files, and image-only pages.

Test fixtures must have known redistribution rights and should be small enough
to keep the repository practical.

## 11. Portability and distribution

Initial releases target Windows, Linux, and macOS. CPU architectures must be
selected and verified in the implementation plan. Each target receives one
executable containing the PDFium WebAssembly module. Build metadata and
third-party notices should accompany releases where required.

Dependency selection, embedded-component auditing, and notice packaging follow
the [dependency and notice policy](dependency-and-notice-policy.md). The same
notice bundle is required for internally shared binaries even while formal
release automation is deferred.

Native PDFium may later be offered as a distinct build or backend. It must not
become a hidden runtime dependency of the default portable release.

## 12. Architectural decision process

Material decisions should be captured as short Architecture Decision Records
under `docs/decisions/`. Each record should state the context, decision,
consequences, and status. The record index and authoring process are documented
in [`docs/decisions/README.md`](decisions/README.md).

This document describes the current architecture. Decision records preserve
why it changed.
