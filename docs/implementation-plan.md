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

## Open decisions

- [ ] Select the first supported CPU architectures.
- [ ] Choose a versioning and release strategy.
- [ ] Confirm dependency licenses and required release notices.

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
- [ ] Add repository-level AI instructions defining commands, architectural
      constraints, and the requirement to update this plan.
- [ ] Add formatting, vetting, and test commands.
- [ ] Add a `Makefile`, `Taskfile.yml`, or small cross-platform build script
      only if plain Go commands become insufficient.
- [x] Create initial Architecture Decision Records.
- [ ] Add contributor guidance covering development commands and documentation
      update expectations.
- [ ] Add CI for formatting, tests, vetting, and builds on selected platforms.
- [ ] Add dependency update and vulnerability scanning automation.
- [x] Document the layered testing and PDF fixture-acquisition strategies.
- [ ] Create the PDF fixture directories, provenance manifest, and validation
      helper when adding the first fixture.

## Phase 1: Vertical-slice converter

**Outcome:** one command converts a simple born-digital PDF into plain
Markdown text using the embedded PDFium WebAssembly backend.

- [x] Define the initial CLI contract and exit codes.
- [ ] Create the package skeleton described in `docs/architecture.md`.
- [ ] Define engine-neutral layout and semantic document models.
- [ ] Define extractor, analyzer, and renderer contracts.
- [ ] Integrate `go-pdfium` WebAssembly with explicit lifecycle management.
- [ ] Extract page text and geometry into the neutral layout model.
- [ ] Implement basic reading order and paragraph grouping.
- [ ] Implement plain paragraphs in the Markdown renderer.
- [ ] Support writing to stdout and to a specified output file.
- [ ] Return actionable errors for invalid, encrypted, and textless input.
- [ ] Add one licensed simple-PDF fixture and an end-to-end golden test.
- [ ] Build and manually exercise one self-contained executable.

## Phase 2: Useful document structure

**Outcome:** common born-digital documents produce readable, stable Markdown.

- [ ] Preserve paragraph boundaries across wrapped lines.
- [ ] Detect headings using font and spacing evidence.
- [ ] Detect ordered and unordered lists.
- [ ] Handle multi-column reading order.
- [ ] Detect and reduce repeated page headers and footers.
- [ ] Preserve links where reliable source information is available.
- [ ] Define an initial strategy for tables and code-like text.
- [ ] Add diagnostics for uncertain or omitted structures.
- [ ] Expand the fixture corpus to cover each supported behavior.
- [ ] Add deterministic golden tests for Markdown output.

## Phase 3: Robustness and engine independence

**Outcome:** conversion fails safely on difficult inputs, and the PDFium
adapter can be replaced without redesigning the pipeline.

- [ ] Add extraction contract tests independent of concrete engines.
- [ ] Add limits for file size, page count, memory-sensitive operations, and
      pathological content where the backend permits.
- [ ] Verify cancellation and cleanup on success and failure paths.
- [ ] Test malformed, encrypted, rotated, font-subset, and image-only PDFs.
- [ ] Add fuzz tests for project-owned parsing and transformation boundaries.
- [ ] Add an explicit engine-selection mechanism if a second backend is
      implemented.
- [ ] Evaluate native PDFium against the same corpus and contract tests.
- [ ] Document unsupported PDF features and expected degradation.
- [ ] Benchmark representative documents for time and peak memory.

## Phase 4: Portable releases

**Outcome:** users can download a release artifact and run it without
installing dependencies.

- [ ] Build for every selected operating system and architecture.
- [ ] Verify each artifact in a clean environment.
- [ ] Add version information to the CLI.
- [ ] Generate checksums and a software bill of materials.
- [ ] Include license and third-party notices.
- [ ] Automate tagged GitHub releases.
- [ ] Add installation and upgrade instructions to the README.

## Phase 5: Future formats and capabilities

This phase is intentionally uncommitted. Candidates should be prioritized from
user feedback and corpus results.

- OCR for scanned PDFs.
- Additional source formats.
- Metadata and image extraction.
- Configurable Markdown style.
- Batch and recursive conversion.
- A stable library API.

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
