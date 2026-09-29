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
pages, positioned text, relevant style information, authored text
placeholders without extractable content, and visible axis-aligned ruling
edges. The PDFium adapter owns all PDFium initialization, handles, response
types, recursive form-object path traversal, coordinate transforms, and
cleanup.

Extraction does not decide whether text is a heading, paragraph, or list. It
reports observed layout information and diagnostics. Reliable external link
annotations are normalized to engine-neutral rectangles and absolute `http`,
`https`, or `mailto` destinations; backend annotation handles and unsupported
actions remain inside the adapter.

Non-fatal diagnostics are engine-neutral records with a stable code, optional
page number, and user-facing message. Extraction aggregates unsupported link
annotations by source page and omission category so repeated annotations
produce one deterministic warning with a count. Analysis preserves those
records and adds warnings for ambiguous link geometry and conservative
table-like or code-like text fallbacks.

Degenerate text objects are retained as engine-neutral physical placeholders
with a position and nominal font size. They are not controls by themselves.

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
horizontal gaps. Consistent small gaps between individually positioned glyphs
are treated as character tracking rather than word boundaries, while explicit
spaces and materially larger gaps still separate words. Nearby lines become
one plain paragraph, while larger vertical gaps and page boundaries start a
new paragraph. Longer tracked runs use upper-quartile gap evidence so normal
glyph-width variation does not split a word inside an isolated table cell.
Zero-area whitespace-only control runs are ignored.

Wrapped-line analysis also uses the observed page text margins. A new
first-line indent or a short sentence-ending line starts a paragraph even
when line spacing remains uniform. Otherwise adjacent lines remain in the
same paragraph, and an ASCII or soft hyphen at a line end is removed when the
next line begins with a lowercase letter. PDFium's U+0002 discretionary-break
marker is normalized to the same soft-hyphen behavior before line joining.
When any line in the wrapped group has a large internal gap that suggests
interleaved columns, visible hyphens are retained throughout that group rather
than risking a cross-column word join. Rotation and multi-column behavior
remain separate analysis work.

Heading inference compares each line's largest observed font size and weight
with page-level median body evidence. A materially larger line with boundary
or above-normal spacing becomes a heading; a bold-only line requires strong
spacing on both sides. Numbered headings use their section-number depth for a
consistent level from 1 through 6; other headings use conservative size ratios
for levels 1 through 3. Decorative-only lines and lines with large internal
gaps characteristic of document-control metadata are not promoted. Ordinary
emphasized lines remain paragraph text. A nearby, style-matched line following
a numbered heading is joined as a wrapped title continuation.

Contents entries are recognized after a contents heading when section
numbering and indentation agree with dotted-leader text ending in a positive
page number. A more deeply indented adjacent line can complete a wrapped
entry. All signals are required so numbered body text and ordinary periods
are not reclassified as navigation. Recognized entries become nested,
engine-neutral unordered lists whose item text retains the section number and
title while dropping dotted leaders and source page numbers.

List inference recognizes common ASCII and Unicode unordered markers and
decimal ordered markers followed by whitespace. Adjacent marker-aligned items
of the same kind form a semantic list; ordered items must increment without
gaps. Semantic list items can own nested child lists, which the Markdown
renderer indents beneath their parent. Ordinary body-list inference remains
flat: nearby lines indented beyond the marker become item continuations, and
differently indented markers start separate lists rather than guessing at
nesting. Ambiguous unmarked lines remain paragraphs.

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

Link annotations use engine-neutral typed targets for reliable external URIs,
one-based pages, and named destinations. PDFium direct destinations and GoTo
actions are resolved to page targets inside the adapter; the neutral model can
also retain names supplied by an engine. Annotations are associated with text
runs whose centers fall within an unambiguous annotation rectangle. Analysis
carries the resulting UTF-8 byte ranges into paragraphs, headings, and list
items. Overlapping annotations with different targets remain plain text rather
than guessing. Internal links are matched to headings on their target page by
normalized visible labels. Matched headings receive deterministic,
document-unique anchors derived from heading text; unmatched targets remain
typed but degrade to unlinked visible text.

### Tables and code-like text

Engine-neutral page rulings preserve horizontal and vertical edges from
visible PDF path objects without exposing PDFium types. Table analysis uses
this evidence only when repeated horizontal boundaries and continuous
vertical boundaries establish at least two rows and two columns. Near-
coincident edges from narrow filled rectangles are normalized before text
runs are assigned by geometry. Every header cell must contain text, but a
complete stable grid may preserve intentionally empty body rows and cells.
Text must not cross an internal boundary. Runs within each accepted cell are
independently ordered into lines before wrapped text is joined, so content
cannot flow across neighboring columns. Composite ruled regions are
partitioned into maximal adjacent row bands with the same vertical-boundary
signature. A table carved from a mixed-signature region additionally requires
either a header distinct from its body or a complete repeated grid with both
populated body content and a header-only column. The latter accommodates
graphic marker columns without treating a merged title followed by ordinary
data rows as a header. A segmented grid with no body text still requires a
bold populated header. Full-width heading or separator rows remain outside
the table. Cells are then traversed in row-major order.
When internal vertical rules are absent, horizontal rules may define rows and
repeated word-start coordinates may define columns under ADR-0008. A ruled
region is partitioned into local, bold-header-led candidates so adjacent
horizontal-only tables do not dilute one another's column evidence. Inferred
starts must occur in the header and receive body-row support, with a distinct
header and both edge cells populated in each accepted body row. Candidates
with more than two body rows require support in at least 75 percent of those
rows. One- and two-body-row candidates may preserve sparse internal cells
when each inferred start occurs in at least one body row. A conflicting or
incomplete row can terminate a longer table, but at least two valid body rows
must precede that truncation.
A single physically ruled row band may be unfolded into a two-row Markdown
table when it has at least two complete vertical cells and every cell contains
exactly two non-empty text lines. The first lines must align across all cells
to form the header, and the second lines must align to form the value row.
Adjacent bands with different vertical-boundary signatures remain independent
tables. Missing, extra, crossing, or unsynchronized lines reject this pattern
rather than inventing field relationships.
Within an accepted horizontal table, a tall physical body band may be
flattened into multiple semantic rows only when separated line groups expose
two or more record starts aligned across at least two inferred columns.
Nearby aligned lines remain wrapped cell content. Text from a source cell
that visually spans the logical records is emitted once in the first
applicable row, with empty continuation cells rather than duplicated values.
Stacked header lines remain in their physical header cells. If separated
content cannot be synchronized across columns, or a logical boundary would
cross text, the table remains plain text.
Accepted table lines are removed from paragraph grouping without changing the
surrounding page order. Rejected ruled regions and separate runs of strongly
aligned table-like text remain ordinary text and produce one diagnostic per
non-overlapping region. The renderer does not fabricate Markdown delimiters
or emit raw HTML.

Analysis recognizes an empty table-control column only when it is narrow, has
at least two body rows, every body cell is otherwise empty and contains one
centered placeholder, and every row has non-empty content outside that
column. The cells become typed unchecked controls. An incomplete, off-center,
oversized, or mixed column remains ordinary empty cells.

A standalone ruled blank becomes a semantic field only when it is a complete,
empty rectangle outside an accepted table, contains one placeholder, and has
one nearby text line horizontally aligned immediately to its left. Unlabeled
ruled bands, signature areas, decorative rectangles, and ambiguous
associations remain unmodeled. Checked state is never inferred without
separate positive mark evidence.

A future semantic code block requires multiple adjacent lines with consistent
monospaced-font, alignment, spacing, and indentation evidence. Its language is
empty unless reliable source metadata becomes available. A single monospaced
line does not qualify, and inline code inference is separate work. These
constraints are recorded in
[ADR-0005](decisions/0005-preserve-ambiguous-tables-and-code-as-text.md);
ruled-table acceptance is governed by
[ADR-0007](decisions/0007-reconstruct-reliable-ruled-tables.md) and
[ADR-0008](decisions/0008-infer-columns-in-horizontally-ruled-tables.md), with
sparse-table extensions governed by
[ADR-0009](decisions/0009-recover-sparse-ruled-tables.md) and conservative
tall-band flattening governed by
[ADR-0010](decisions/0010-flatten-synchronized-table-records.md). Stacked
field-band unfolding is governed by
[ADR-0011](decisions/0011-unfold-stacked-field-bands.md). Conservative empty
form-control and labeled-field recovery is governed by
[ADR-0012](decisions/0012-recover-explicit-empty-fields.md).
The initial analyzer reports table-like text only after three adjacent lines
show multiple large intra-line gaps. It reports code-like text only after two
aligned adjacent lines consistently use recognized monospaced font names.
Both remain ordinary text.

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
single-blank-line block separation used for paragraphs. Referenced headings
receive an explicit HTML anchor immediately before the ATX heading. Semantic
lists render recursively with `-` markers or preserved decimal starting
numbers. Item text is escaped as plain text, and explicit item line breaks
receive Markdown continuation indentation.
Validated external semantic links render as Markdown links with escaped labels
and destinations. Internal links render only when their named target matches
an emitted anchor; unmatched page or named targets retain visible label text.
Validated semantic tables render as pipe tables with the first row as the
header. Cell text and links use the same escaping rules as other semantic
content, including escaped literal pipes; empty body cells remain empty.
Typed unchecked and checked table controls render as `[ ]` and `[x]`.
Labeled blank fields render as their escaped label followed by a visible
underscore blank.

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

After a successful commit, the converter returns semantic diagnostics to the
CLI. The CLI writes each as a warning on standard error; diagnostics never
enter Markdown, alter the success exit code, or become visible before output
has committed.

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
	Pages       []Page
	Diagnostics []Diagnostic
}

type Page struct {
	Number           int
	Width            float64
	Height           float64
	TextRuns         []TextRun
	TextPlaceholders []TextPlaceholder
	Links            []LinkAnnotation
	Rulings          []Ruling
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
not required to expose backend-specific font data. Text placeholders preserve
authored positions that have no extractable content; they carry no field or
control semantics until analysis combines them with surrounding geometry.

The semantic model is an ordered set of blocks and non-fatal diagnostics owned
by `internal/document`.
It defines plain paragraphs, validated level 1 through 6 headings, ordered or
unordered lists, rectangular tables, and labeled blank fields. Ordered lists
retain their starting number. Paragraphs, headings, list items, fields, and
table cells may contain validated, non-overlapping link ranges. Table cells
may instead contain a typed checkbox state. The model contains no
extraction-engine details.

## 8. Engine selection and lifecycle

The first engine identifier is `pdfium-wasm`. Engine construction belongs in a
small factory or composition layer, not in conversion logic. A future
`pdfium-native` or pure-Go implementation must satisfy the same extractor
contract. The Phase 3 [native PDFium evaluation](evaluations/native-pdfium.md)
did not accept a native backend because the supported CGo modes add a runtime
library or worker process and platform-specific build requirements. The CLI
therefore has no engine option while only one backend is available; selection
will remain an internal composition concern until a second backend is
accepted.

The adapter is responsible for:

- creating and closing the PDFium pool or instance;
- opening and closing documents and pages;
- checking cancellation between meaningful operations;
- translating PDFium output into internal coordinates and units;
- wrapping errors with operation and page context;
- detecting password-protected and likely image-only documents where possible;
- enforcing configured source, page, geometry, text-run, and annotation limits
  at the earliest backend operation that exposes each measurement.

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
evidence. Link annotations are closed on every path after their URI action and
rectangle have been inspected.

The CLI uses validated default extraction limits: 256 MiB source size, 2,000
pages, 1,000,000 returned character runs, 100,000 annotations, and a 200,000
point maximum page dimension. Source size is checked before acquiring PDFium;
page count and dimensions are checked before page text work; cumulative
character and annotation counts are checked before mapping backend responses
into shared models. Exceeding a limit returns the engine-neutral
`extract.ErrLimitExceeded` category with the observed and maximum values.

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
- **Fuzz tests:** exercise owned PDF-header recognition, physical-to-semantic
  analysis, and semantic-to-Markdown rendering boundaries with bounded inputs
  and output invariant checks.

Small malformed, encrypted, rotated, subset-font, and image-only PDFs are
constructed deterministically in tests. Keeping these edge-case inputs
self-authored and generated in memory avoids provenance ambiguity while
exercising the real embedded PDFium backend.

End-to-end benchmarks run the generated simple and Phase 2 documents through
extraction, analysis, rendering, and an in-memory transactional destination.
They report elapsed time, Go allocations, allocated bytes, and a sampled peak
increase in Go heap usage. The peak metric is an in-process regression
indicator, not total process RSS or a cross-machine absolute guarantee.

Test fixtures must have known redistribution rights and should be small enough
to keep the repository practical.

The initial golden test converts the self-authored generated PDF through the
real embedded PDFium runtime, basic analyzer, Markdown renderer, and buffered
writer destination. Its committed Markdown golden is UTF-8 with an enforced
LF checkout policy and is referenced by the fixture provenance manifest.
The Phase 2 golden adds three-page coverage for headings, wrapped paragraphs,
lists, two-column ordering, repeated page furniture, external links, and
table/code fallback diagnostics through the same real pipeline.

The reusable `internal/extract/contracttest` suite verifies every registered
backend's stable name, neutral validated layout, source ownership,
invalid-document categorization, and pre-cancelled behavior. Backend packages
supply only representative valid and invalid sources plus expected neutral
facts.

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
