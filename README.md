# TransmuteMD

TransmuteMD is a portable command-line tool for converting documents to
Markdown. The first supported input format will be PDF.

The initial implementation will use Go and PDFium through the
[`go-pdfium`](https://github.com/klippa-app/go-pdfium) WebAssembly backend.
PDF extraction is kept behind an internal interface so a native PDFium backend
or another Go PDF library can be introduced without changing the conversion
pipeline.

## Project status

The converter, robustness, and document-fidelity work are complete in the
[implementation plan](docs/implementation-plan.md). The project is preparing
for its first public release. Five-target cross-build and native executable
smoke coverage is implemented and has passed on GitHub-hosted runners.
Minimum OS requirements, release packaging, and publication automation remain
pending.

The selected initial release targets are Windows x64, Linux x64/ARM64, and
macOS Intel/Apple Silicon. The first public version will be `v0.1.0`, using
Semantic Versioning, with automated draft releases and owner-approved
publication. Windows signing and macOS signing/notarization are deferred;
unsigned binaries may encounter platform trust warnings or launch
restrictions. See [release scope](docs/decisions/0014-set-initial-release-scope.md)
for the compatibility policy and distribution gates. These targets are not
yet a claim that release artifacts have been verified.

## Usage

```console
transmutemd document.pdf
transmutemd document.pdf --output document.md
transmutemd document.pdf --output -
```

Without `--output`, TransmuteMD creates `document.md` beside the input. It
refuses to overwrite an existing file unless `--force` is supplied.

TransmuteMD should be distributed as a single executable with no separately
installed PDF engine or runtime. Binary distribution remains blocked by the
embedded PDFium provenance and notice audit described below; development
builds are functional.

## PDF support

TransmuteMD currently targets born-digital, unencrypted PDFs with extractable
text. It does not perform OCR, extract images, or accept passwords. Tables,
code-like regions, uncertain layouts, and unsupported annotations degrade
conservatively to plain text or geometric reading order rather than invented
structure. See [PDF support and degradation](docs/pdf-support.md) for the
complete behavior and default safety limits.

## Planned project layout

```text
transmutemd/
|-- cmd/
|   `-- transmutemd/             # Executable entry point
|-- .github/
|   `-- copilot-instructions.md  # Copilot-specific repository guidance
|-- internal/
|   |-- app/                     # Conversion use cases and orchestration
|   |-- cli/                     # Arguments, user output, and exit codes
|   |-- document/                # Engine-neutral layout and semantic models
|   |-- extract/                 # Extractor contracts and implementations
|   |   `-- pdf/
|   |       `-- pdfium/          # PDFium adapter and backend setup
|   |-- analyze/                 # Reading order and semantic inference
|   `-- render/
|       `-- markdown/            # Markdown generation
|-- docs/
|   |-- architecture.md          # Design, boundaries, and data flow
|   |-- cli-contract.md          # Initial user-visible CLI behavior
|   |-- dependency-and-notice-policy.md
|   |                            # Dependency and distribution compliance
|   |-- decisions/               # Architecture Decision Records
|   `-- implementation-plan.md   # Living roadmap and progress record
|-- testdata/
|   `-- pdf/                     # Representative PDF fixtures
|-- AGENTS.md                    # AI development and workflow instructions
|-- go.mod
`-- README.md
```

The directories are created when their corresponding implementation work
begins. Packages remain under `internal` until a stable public Go API is
required.

## Documentation

- [Architecture](docs/architecture.md)
- [Architecture decisions](docs/decisions/README.md)
- [Initial CLI contract](docs/cli-contract.md)
- [PDF support and degradation](docs/pdf-support.md)
- [Dependency and notice policy](docs/dependency-and-notice-policy.md)
- [Implementation plan](docs/implementation-plan.md)

## Development

Development currently requires Go 1.27.1 or later.

```console
go fmt ./...
go vet ./...
go test ./...
go run ./testdata/pdf/validate.go
go test -run=^$ -bench=BenchmarkGeneratedPDFConversion -benchmem ./internal/app
go run ./cmd/transmutemd --help
```

GitHub Actions runs formatting checks and native quality checks (vetting,
tests, fixture validation, and builds) on Windows, Linux, and macOS. A separate
Linux-hosted job cross-compiles the CLI for all five selected release targets
with CGo disabled and `-trimpath`, then verifies that each executable is
non-empty and its Go build metadata matches the requested target and flags.
Cross-compilation does not establish native runtime support.

Separate native smoke jobs build and execute the CLI on matching
OS/architecture runners:

| Target | Configured GitHub-hosted runner |
|---|---|
| Windows x64 | `windows-2025` |
| Linux x64 | `ubuntu-24.04` |
| Linux ARM64 | `ubuntu-24.04-arm` |
| macOS Intel | `macos-15-intel` |
| macOS Apple Silicon | `macos-15` |

The smoke suite copies the executable and licensed fixtures into temporary
directories and launches the executable with an allowlisted environment,
empty tool search path, and isolated home and temporary directories. It
checks exact Markdown goldens and warnings, file and stdout output, CLI error
codes, overwrite protection, and output/input preservation and temporary-file
cleanup after failures. Executable metadata must match the native platform,
with CGo disabled and trimmed source paths.

Run the same suite locally with:

```console
go test -count=1 -run '^TestExecutableSmoke$' -v ./cmd/transmutemd
```

By default it builds the CLI; to check an already built executable, set the
test-only `TRANSMUTEMD_SMOKE_EXECUTABLE` environment variable to its absolute
path. The normal `go test ./...` command also includes this suite.

Native jobs record their actual OS versions, architecture, and available
runner-image metadata in logs and the workflow summary. Configured runners
are test baselines, not minimum supported OS guarantees. The first hosted run
passed all five native targets; evidence is linked from the
[implementation plan](docs/implementation-plan.md). A minimum OS support
policy remains pending. Fresh GitHub runners include
development tools; the restricted subprocess environment checks standalone
conversion but does not emulate a completely pristine OS or downloaded-binary
SmartScreen/Gatekeeper behavior.

CI runs on pushes and pull requests. Once the workflow is on the default
branch, it can also be started from GitHub's **Actions** tab by selecting
**CI**, then **Run workflow**. The build and smoke jobs do not upload or cache
executables or publish releases; binary distribution remains blocked by the
PDFium audit.

AI-assisted changes follow [`AGENTS.md`](AGENTS.md). The file identifies the
authoritative project context, architectural constraints, required checks, and
documentation workflow.

## Development principles

- Keep PDF engine types inside their adapter.
- Separate extraction, semantic analysis, and Markdown rendering.
- Prefer deterministic output and actionable errors.
- Test against representative real-world PDFs, not only synthetic examples.
- Preserve the single-download user experience.

## License

TransmuteMD is licensed under the [MIT License](LICENSE).

Third-party licensing information is recorded in
[`THIRD_PARTY_NOTICES`](THIRD_PARTY_NOTICES) and
[`third_party/dependencies.json`](third_party/dependencies.json). Binary
distribution remains blocked until the embedded PDFium WebAssembly audit
identified there is complete.
