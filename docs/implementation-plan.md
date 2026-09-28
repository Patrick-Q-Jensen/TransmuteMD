# Implementation Plan

This is the living implementation plan for TransmuteMD. Update it in the same
change as implementation work that completes, adds, removes, or materially
changes a task.

## Status conventions

- `[ ]` Not started
- `[~]` In progress
- `[x]` Complete
- `[!]` Blocked, with the reason recorded beside the item

Keep completed items in place so the document remains a lightweight progress
record. Add significant design outcomes to `docs/architecture.md` or an
Architecture Decision Record rather than placing detailed design notes here.

## Current decisions

| Area | Decision | Status |
|---|---|---|
| Implementation language | Go | Accepted |
| Initial input format | PDF | Accepted |
| Initial PDF engine | `go-pdfium` with embedded WebAssembly PDFium | Accepted |
| Distribution goal | One executable with no separately installed runtime | Accepted |
| Architecture | Extraction, analysis, and rendering are separate stages | Accepted |
| Project maturity | Internal packages; no stable Go library API initially | Accepted |
| Go module | `github.com/Patrick-Q-Jensen/TransmuteMD` | Accepted |
| Minimum Go version | Go 1.27.1 | Accepted |
| Project license | MIT | Accepted |
| Initial operating systems | Windows, Linux, and macOS | Accepted |
| Initial CLI | Direct, single-file command defined in `docs/cli-contract.md` | Accepted |
| Encrypted PDFs | Unsupported initially; return a conversion error without prompting | Accepted |
| Failed or textless conversion | Do not create or replace output | Accepted |
| Dependency licensing | Permissive by default; explicit approval and ADR for exceptions | Accepted |
| Dependency storage | Go modules without vendoring by default | Accepted |
| Internal distribution | Self-contained executable plus license and notice files in an archive | Accepted |

## Open decisions

- [ ] Select the first supported CPU architectures.
- [ ] Choose a versioning and release strategy.

## Testing strategy

Testing is layered so most behavior can be checked quickly without starting
PDFium, while a smaller representative corpus verifies extraction and the
complete executable.

| Layer | Scope | When it runs |
|---|---|---|
| Unit | Layout analysis, semantic inference, Markdown escaping, CLI validation, and error mapping | Every change |
| Extractor contract | Shared behavioral expectations for each PDF engine | Every extractor change |
| Golden | Stable layout-model and Markdown output for representative PDFs | Every change |
| CLI integration | Arguments, files, stdout/stderr, exit codes, and cancellation | Every change |
| Corpus/regression | Complex layouts, fonts, rotation, encryption, malformed files, and image-only pages | CI where practical and before releases |
| Fuzz | Parsers owned by this project, layout analysis, and rendering boundaries | Short CI runs and longer scheduled runs |
| Benchmark | Conversion time and peak-memory indicators for representative documents | Before releases and performance-sensitive changes |
| Release smoke | Built artifacts execute in clean target environments without installed dependencies | Every release |

Engine-neutral unit tests should make up most of the suite. PDFium should be
used in contract, golden, corpus, and end-to-end tests rather than mocked in
detail. A fixed regression test must accompany every corrected conversion
bug.

The default local quality gate will be:

```console
go fmt ./...
go vet ./...
go test ./...
```

CI will add race detection where supported, bounded fuzz smoke tests, and the
selected operating-system and architecture matrix.

## PDF fixture strategy

Test PDFs are executable inputs from a licensing and security perspective.
Every committed fixture must be small, reviewed, and have documented
provenance and redistribution rights.

Fixtures will be acquired in this order:

1. **Self-authored generated fixtures.** Create minimal source documents with
   original test text, export them with representative PDF producers, and
   retain the source or generation script. These are preferred for focused
   behavior and regression tests.
2. **Permissively licensed upstream test suites.** Select individual files
   from projects such as PDFium, Apache PDFBox, or PDF.js only after verifying
   that the specific file may be redistributed. Public availability alone is
   not sufficient.
3. **Public-domain or explicitly licensed real documents.** Use these to add
   realistic combinations of layout features that synthetic fixtures miss.
4. **Reduced bug reproductions.** Do not commit private user documents. Rebuild
   the relevant characteristic in a minimal self-authored PDF, or keep the
   original in a non-public local corpus when reduction is impossible.

`testdata/pdf/manifest.json` will record, for each PDF:

- path and SHA-256 digest;
- purpose and features covered;
- whether it is generated or third-party;
- source document or generation instructions;
- source URL and version when applicable;
- author, copyright status, license, and required attribution;
- expected result or expected error category.

Generated, third-party, and regression fixtures should remain distinguishable
under `testdata/pdf/`. Large corpora should not be committed by default; use a
versioned download script with verified hashes when repository size becomes a
concern.

## Phase 0: Project foundation

**Outcome:** contributors and AI development tools have a consistent project
context and repeatable quality checks.

- [x] Create the root project README.
- [x] Document the initial architecture and dependency boundaries.
- [x] Create this living implementation plan.
- [x] Initialize the Git repository.
- [x] Initialize the Go module.
- [x] Add `LICENSE` after selecting the project license.
- [x] Add `.gitignore` and `.editorconfig`.
- [x] Add repository-level AI instructions defining commands, architectural
      constraints, and the requirement to update this plan.
- [x] Add formatting, vetting, and test commands.
- [x] Keep the plain Go commands while they remain sufficient; no additional
      cross-platform build script is currently needed.
- [x] Create initial Architecture Decision Records.
- [x] Add CI for formatting, tests, vetting, and builds on selected platforms.
- [x] Define the dependency review, license, and notice policy.
- [x] Document the layered testing and PDF fixture-acquisition strategies.
- [x] Create the PDF fixture directories, provenance manifest, and validation
      helper when adding the first fixture.

## Phase 1: Vertical-slice converter

**Outcome:** one command converts a simple born-digital PDF into plain
Markdown text using the embedded PDFium WebAssembly backend.

- [x] Define the initial CLI contract and exit codes.
- [x] Create the package skeleton described in `docs/architecture.md`.
- [x] Define engine-neutral layout and semantic document models.
- [x] Define extractor, analyzer, and renderer contracts.
- [x] Integrate `go-pdfium` WebAssembly with explicit lifecycle management.
- [!] Complete the audit of pinned `go-pdfium` v1.21.0 and its embedded
      PDFium WASM. The Go dependency inventory, license texts, notices, and
      WASM digest are recorded, but binary distribution remains blocked
      because upstream does not identify the exact PDFium revision, build
      tool versions, or complete incorporated third-party notice set.
- [x] Extract page text and geometry into the neutral layout model.
- [x] Implement basic reading order and paragraph grouping.
- [x] Implement plain paragraphs in the Markdown renderer.
- [x] Support writing to stdout and to a specified output file.
- [x] Return actionable errors for invalid, encrypted, and textless input.
- [x] Add one licensed simple-PDF fixture and an end-to-end golden test.
- [x] Build and manually exercise one self-contained executable.

## Phase 2: Useful document structure

**Outcome:** common born-digital documents produce readable, stable Markdown.

- [x] Preserve paragraph boundaries across wrapped lines.
- [x] Detect headings using font and spacing evidence.
- [x] Detect ordered and unordered lists.
- [x] Handle multi-column reading order.
- [x] Detect and reduce repeated page headers and footers.
- [x] Preserve links where reliable source information is available.
- [x] Define an initial strategy for tables and code-like text.
- [x] Add diagnostics for uncertain or omitted structures.
- [x] Expand the fixture corpus to cover each supported behavior.
- [x] Add deterministic golden tests for Markdown output.

## Phase 3: Robustness and engine independence

**Outcome:** conversion fails safely on difficult inputs, and the PDFium
adapter can be replaced without redesigning the pipeline.

- [x] Add extraction contract tests independent of concrete engines.
- [x] Add limits for file size, page count, memory-sensitive operations, and
      pathological content where the backend permits.
- [x] Verify cancellation and cleanup on success and failure paths.
- [x] Test malformed, encrypted, rotated, font-subset, and image-only PDFs.
- [x] Add fuzz tests for project-owned parsing and transformation boundaries.
- [x] Add an explicit engine-selection mechanism if a second backend is
      implemented. No second backend was accepted, so no user-facing selector
      is exposed.
- [x] Evaluate native PDFium against the same corpus and contract tests.
- [x] Document unsupported PDF features and expected degradation.
- [x] Benchmark representative documents for time and peak memory.

## Phase 4: Document fidelity

**Outcome:** structured business documents preserve readable text, hierarchy,
navigation, and tabular relationships before release engineering begins.

Implement these groups in order because later groups depend on evidence and
semantic models established by earlier work.

### Group 1: Text fidelity

- [x] Normalize PDF discretionary-break control characters, including U+0002,
      and rejoin words split across lines.
- [x] Improve word-space inference using font scale, observed glyph geometry,
      and consistent character tracking so letterspaced text is not split
      into false words.

### Group 2: Heading and contents structure

- [x] Detect hierarchical numbered headings and derive consistent heading
      levels while excluding decorative glyphs and document-control metadata.
- [x] Recognize contents entries using numbering, indentation, dotted leaders,
      and trailing page numbers.
- [x] Add engine-neutral nested-list semantics and render hierarchical
      contents without leader dots.

### Group 3: Internal document navigation

- [x] Extend engine-neutral links to represent internal page and named
      destinations.
- [x] Associate internal destinations with contents labels and emit stable
      Markdown anchors where possible.
- [x] Aggregate repeated unsupported-link diagnostics by page and category.

### Group 4: Table reconstruction

- [x] Supersede ADR-0005 with acceptance criteria for reliable table
      reconstruction and conservative fallback.
- [x] Add engine-neutral ruling-line or equivalent geometry evidence where
      the backend exposes it.
- [x] Detect table regions, rows, columns, and cells from geometry and aligned
      text.
- [x] Order wrapped text within each cell before traversing cells by row and
      column.
- [x] Add validated semantic table blocks and render compatible tables as
      Markdown.
- [x] Preserve complex or ambiguous tables with a controlled plain-text
      fallback and one diagnostic per table region.
- [x] Partition composite ruled regions into independently validated,
      schema-consistent table bands around full-width separators.
- [ ] Reconstruct horizontally ruled tables only when repeated text anchors
      provide unambiguous column evidence.

### Group 5: Forms and document fields

- [ ] Define which checkboxes, signature fields, blank result cells, and ruled
      fields carry useful document semantics.
- [ ] Detect supported field geometry without treating decorative lines as
      content.
- [ ] Represent supported empty checkboxes and meaningful blank fields in
      Markdown-compatible form.

Image and general graphic extraction remains deferred to Phase 6. Decorative
graphics should continue to be omitted unless a future semantic image model
can distinguish meaningful figures and define asset and alt-text behavior.

## Phase 5: Public releases

**Outcome:** users can download a release artifact and run it without
installing dependencies.

- [ ] Build for every selected operating system and architecture.
- [ ] Verify each artifact in a clean environment.
- [ ] Add version information to the CLI.
- [ ] Generate checksums and a software bill of materials.
- [ ] Verify license and third-party notice bundles against each final
      artifact.
- [ ] Add scheduled dependency updates and automated vulnerability scanning
      before public releases.
- [ ] Automate tagged GitHub releases.
- [ ] Add installation and upgrade instructions to the README.

## Phase 6: Future formats and capabilities

This phase is intentionally uncommitted. Candidates should be prioritized from
user feedback and corpus results.

- OCR for scanned PDFs.
- Additional source formats.
- Metadata and image extraction.
- Configurable Markdown style.
- Batch and recursive conversion.
- A stable library API.
- A human-focused `CONTRIBUTING.md` when external contributions are expected.

## Definition of done for each implementation task

A task is complete when:

1. behavior is implemented through the appropriate architectural boundary;
2. relevant automated tests pass;
3. errors are explicit and actionable;
4. user-facing behavior is documented;
5. this plan and affected architecture documentation are updated;
6. no new end-user runtime dependency has been introduced unintentionally.

## Change log

| Date | Change |
|---|---|
| 2026-09-22 | Created the initial architecture, project README, and implementation plan. |
| 2026-09-23 | Initialized Git and the Go module, selected Go 1.27.1, and added baseline repository configuration. |
| 2026-09-24 | Added the layered testing strategy and licensed PDF fixture-acquisition policy. |
| 2026-09-24 | Selected MIT and the initial operating systems, then defined the initial CLI contract. |
| 2026-09-24 | Added the ADR process and records for the four foundational architecture decisions. |
| 2026-09-24 | Added tool-neutral and Copilot-specific AI development instructions. |
| 2026-09-24 | Defined dependency acceptance, embedded-component auditing, and internal notice packaging. |
| 2026-09-25 | Confirmed the plain Go formatting, vetting, and test commands as the default local quality gate. |
| 2026-09-25 | Added the package skeleton and cross-platform CI for formatting, vetting, tests, and builds. |
| 2026-09-26 | Defined validated engine-neutral physical layout and semantic paragraph models. |
| 2026-09-26 | Defined context-aware extraction, analysis, and rendering contracts with explicit ownership and validation rules. |
| 2026-09-26 | Integrated the embedded PDFium WebAssembly runtime with isolated filesystem access, cancellation, deterministic cleanup, and a generated smoke-test fixture. |
| 2026-09-26 | Added the initial dependency inventory and notice bundle; blocked binary distribution pending the complete embedded PDFium provenance and license audit. |
| 2026-09-26 | Implemented PDFium page, character geometry, rotation, and font evidence extraction into the neutral layout model. |
| 2026-09-26 | Added deterministic single-column reading order, whitespace reconstruction, and basic paragraph grouping. |
| 2026-09-27 | Implemented UTF-8 plain-paragraph Markdown rendering with escaping, LF normalization, cancellation, and explicit write errors. |
| 2026-09-27 | Added engine-neutral conversion orchestration with buffered stdout and transactional temporary-file output. |
| 2026-09-27 | Added stable invalid, encrypted, and textless conversion errors that preserve transactional output. |
| 2026-09-27 | Added an exact Markdown golden test spanning the embedded PDFium runtime through transactional writer output. |
| 2026-09-27 | Wired and exercised the single-file CLI with content recognition, transactional destinations, resource cleanup, diagnostics, and exit codes. |
| 2026-09-27 | Refined wrapped-line analysis with indentation, short-line, spacing, and soft-hyphen paragraph heuristics. |
| 2026-09-27 | Added conservative heading detection from relative font and spacing evidence with semantic and Markdown support. |
| 2026-09-27 | Added flat ordered and unordered list detection with semantic validation, wrapped-item analysis, and Markdown rendering. |
| 2026-09-27 | Added conservative two-column reading order with full-width region boundaries and ambiguous-layout fallback. |
| 2026-09-27 | Added cross-page detection and suppression of repeated headers, footers, and digit-varying page labels. |
| 2026-09-27 | Preserved reliable external PDF link annotations through neutral layout and semantic models into escaped Markdown links. |
| 2026-09-27 | Adopted ADR-0005: preserve ambiguous table-like and code-like content as text until dedicated high-confidence semantic models exist. |
| 2026-09-27 | Added engine-neutral structure diagnostics and successful CLI warnings for unsupported links and conservative text fallbacks. |
| 2026-09-27 | Added a self-authored three-page Phase 2 PDF fixture covering semantic structure, links, repeated furniture, columns, and conservative fallbacks. |
| 2026-09-27 | Added exact Markdown and diagnostic golden coverage for the Phase 2 fixture through the embedded PDFium pipeline. |
| 2026-09-27 | Added a backend-independent extractor contract suite and registered the PDFium WASM adapter against it. |
| 2026-09-27 | Added validated extraction limits for source size, pages, page geometry, character runs, and annotations. |
| 2026-09-27 | Verified instance, document, annotation, and output cleanup across success, failure, and cancellation paths. |
| 2026-09-27 | Added deterministic difficult-input tests for malformed, encrypted, rotated, subset-font, and image-only PDFs. |
| 2026-09-27 | Added bounded fuzz targets for PDF recognition, layout analysis, and Markdown rendering. |
| 2026-09-27 | Evaluated native PDFium and retained the embedded WASM backend because native modes violate current build or distribution constraints. |
| 2026-09-27 | Closed the conditional engine-selection item without adding a one-choice CLI option; selection remains internal until another backend is accepted. |
| 2026-09-27 | Documented supported PDF content, unsupported features, conservative degradation, warnings, and extraction limits. |
| 2026-09-27 | Added end-to-end benchmarks for representative generated PDFs with timing, allocation, and sampled peak Go-heap metrics. |
| 2026-09-27 | Added an ordered document-fidelity phase covering text normalization, heading and contents structure, internal navigation, tables, and form fields before public release work. |
| 2026-09-27 | Normalized PDFium U+0002 discretionary breaks and rejoined affected wrapped words. |
| 2026-09-27 | Distinguished consistent positioned-glyph tracking from larger inferred word gaps. |
| 2026-09-28 | Preserved discretionary hyphens across ambiguous column groups to prevent cross-column word corruption. |
| 2026-09-28 | Derived numbered heading levels from section depth and excluded decorative and multi-column metadata lines. |
| 2026-09-28 | Recognized contents entries from section numbering, indentation, dotted leaders, and trailing page numbers. |
| 2026-09-28 | Added recursive semantic lists and rendered recognized contents as an indented Markdown hierarchy without leader dots. |
| 2026-09-28 | Added typed external, page, and named link targets and resolved PDF GoTo destinations inside the PDFium adapter. |
| 2026-09-28 | Accepted ADR-0006, matched internal links to destination headings, and emitted deterministic Markdown anchors for resolved navigation. |
| 2026-09-28 | Aggregated unsupported-link diagnostics by source page and omission category while preserving deterministic warning order and counts. |
| 2026-09-28 | Accepted ADR-0007 with strict ruled-table acceptance criteria and conservative regional fallback. |
| 2026-09-28 | Added bounded engine-neutral ruling geometry extracted from visible axis-aligned PDF path edges. |
| 2026-09-28 | Detected complete ruled-table lattices and assigned text unambiguously to physical cells. |
| 2026-09-28 | Ordered and joined wrapped text independently within each detected cell before row-major traversal. |
| 2026-09-28 | Added validated semantic table blocks and rendered accepted grids as escaped Markdown pipe tables. |
| 2026-09-28 | Preserved rejected and strongly aligned table regions as text with one non-overlapping diagnostic per region. |
| 2026-09-28 | Partitioned mixed ruled regions by stable column signatures and recovered independently headed table bands. |
