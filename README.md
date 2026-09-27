# TransmuteMD

TransmuteMD is a portable command-line tool for converting documents to
Markdown. The first supported input format will be PDF.

The initial implementation will use Go and PDFium through the
[`go-pdfium`](https://github.com/klippa-app/go-pdfium) WebAssembly backend.
PDF extraction is kept behind an internal interface so a native PDFium backend
or another Go PDF library can be introduced without changing the conversion
pipeline.

## Project status

The project is in the vertical-slice implementation phase. See the
[implementation plan](docs/implementation-plan.md) for current progress and
planned work.

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

GitHub Actions runs formatting checks, vetting, tests, and builds on Windows,
Linux, and macOS.

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
