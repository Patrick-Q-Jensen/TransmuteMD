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

The initial CLI recognizes `%PDF-` within the first 1,024 input bytes instead
of relying on the filename extension. It resolves default and explicit output
paths, prevents input/output aliasing, maps stable error categories to the
documented exit codes, and translates process interruption into context
cancellation. The executable entry point owns concrete PDFium, analyzer, and
renderer construction.

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

The initial analyzer uses page order and single-column geometry. It clusters
text runs into lines using vertical overlap, sorts each line from left to
right, preserves explicit whitespace, and infers missing word spaces from
horizontal gaps. Nearby lines become one plain paragraph, while larger
vertical gaps and page boundaries start a new paragraph. Zero-area
whitespace-only control runs are ignored.

Wrapped-line analysis also uses the observed page text margins. A new
first-line indent or a short sentence-ending line starts a paragraph even
when line spacing remains uniform. Otherwise adjacent lines remain in the
same paragraph, and an ASCII or soft hyphen at a line end is removed when the
next line begins with a lowercase letter. Rotation and multi-column behavior
remain separate analysis work.

Heading inference compares each line's largest observed font size and weight
with page-level median body evidence. A materially larger line with boundary
or above-normal spacing becomes a heading; a bold-only line requires strong
spacing on both sides. Conservative size ratios map detected headings to
levels 1 through 3, while ordinary emphasized lines remain paragraph text.

List inference recognizes common ASCII and Unicode unordered markers and
decimal ordered markers followed by whitespace. Adjacent marker-aligned items
of the same kind form a flat semantic list; ordered items must increment
without gaps. Nearby lines indented beyond the marker become item
continuations. Differently indented markers start separate lists rather than
inferring unsupported nesting, and ambiguous unmarked lines remain
paragraphs.

Two-column reading order is inferred only where a page region has a clear
center gutter, at least two lines on each side, and vertically overlapping
column content. Full-width lines delimit regions and remain in page order;
within a detected region, the left column precedes the right column. Explicit
flow boundaries prevent paragraphs and lists from merging across columns.
Ambiguous layouts retain geometric row order rather than forcing a column
interpretation.

Repeated page furniture is detected across documents with at least three
pages. Exact case-normalized text signatures in the outer 12 percent of a
page, including digit-normalized page numbers, are suppressed only when they
occur in the same header or footer alignment on at least three pages and
two-thirds of the document. Repeated text in the body is preserved.

### Rendering

The renderer converts the semantic document to Markdown. It owns Markdown
escaping, whitespace rules, and syntax choices, but no PDF-specific behavior.
The initial Markdown renderer validates the complete semantic document and
its UTF-8 text before writing. It escapes plain paragraph text so Markdown
syntax is not inferred accidentally, normalizes embedded line endings to LF,
separates paragraphs with one blank line, and terminates non-empty output with
LF. Cancellation, short writes, and writer failures are returned with block
context; transactional publication remains the application's responsibility.
Detected headings render as escaped ATX headings followed by the same
single-blank-line block separation used for paragraphs. Flat semantic lists
render with `-` markers or preserved decimal starting numbers. Item text is
escaped as plain text, and explicit item line breaks receive Markdown
continuation indentation.

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

The application converter validates each stage result and commits the
rendered buffer through an engine-neutral destination only after extraction,
analysis, and rendering succeed. Writer destinations support standard output
without taking ownership of the writer. File destinations create a temporary
file in the destination directory, write and synchronize its complete
contents, close it, then rename it into place. Existing files are preserved
unless replacement is explicitly enabled, and temporary files are removed on
all reported failure paths.

The CLI closes the input and process-scoped PDFium runtime before delegating
the final output commit. Cleanup failure therefore prevents both file and
standard-output publication rather than reporting failure after visible
output has already been produced.

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
It defines plain paragraphs, validated level 1 through 6 headings, and flat
ordered or unordered lists of plain-text items. Ordered lists retain their
starting number. The model contains no extraction-engine details.

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

Extraction requests page dimensions and character-level structured text.
Each non-empty Unicode character becomes an initial text run in extraction
order; joining characters into lines and paragraphs remains analysis work.
PDFium's bottom-left-origin point coordinates are normalized to the shared
top-left-origin convention. Character angles are converted from radians to
degrees, rendered font size is preferred over nominal size, negative unknown
font weights become zero, and the PDF font italic flag becomes neutral style
evidence.

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

The extraction boundary exposes engine-neutral invalid-document and
encrypted-document sentinels. The PDFium adapter maps incorrect-format,
unreadable-structure, password, and unsupported-encryption failures to those
sentinels without exposing backend error types. Application orchestration
rejects semantic documents with no blocks as textless before rendering or
committing output, and its error explains that scanned documents require OCR.

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

The initial golden test converts the self-authored generated PDF through the
real embedded PDFium runtime, basic analyzer, Markdown renderer, and buffered
writer destination. Its committed Markdown golden is UTF-8 with an enforced
LF checkout policy and is referenced by the fixture provenance manifest.

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
