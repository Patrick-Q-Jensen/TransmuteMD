# Native PDFium evaluation

- **Status:** Completed; not adopted
- **Date:** 2026-09-27
- **Evaluated binding:** `github.com/klippa-app/go-pdfium` `v1.21.0`
  (`33eb7a1a810f0e7dd48a4f3c438b5f1d811bd127`)

## Objective

Determine whether a native PDFium backend should supplement or replace the
embedded WebAssembly backend during Phase 3 without weakening the portable,
single-executable distribution goal or the dependency and notice policy.

## Evaluated options

| Option | Execution model | Build/runtime requirements | Isolation |
|---|---|---|---|
| `go-pdfium/single_threaded` | In-process CGo calls | CGo toolchain, `pkg-config`, PDFium headers and library at build time, and a discoverable PDFium library at runtime | A native PDFium fault can terminate TransmuteMD |
| `go-pdfium/multi_threaded` | Native worker subprocesses over `go-plugin` | The same CGo and PDFium requirements plus a separately built and deployed worker executable | Worker failures are isolated from TransmuteMD |
| Existing `go-pdfium/webassembly` | Embedded PDFium module in wazero | Go toolchain only; no separately installed PDF engine | WebAssembly instance is isolated and cancellation can terminate it |

The upstream `v1.21.0` documentation states that both CGo implementations
require PDFium during compilation and runtime. Its CGo implementation links
through `pkg-config: pdfium`. The multi-threaded implementation additionally
starts an external worker process. The same documentation estimates the
single-threaded native implementation at roughly twice the speed of
WebAssembly, but that is an upstream generalization rather than a
TransmuteMD benchmark.

The evaluation host was Windows/amd64 with CGo disabled and no `pkg-config`
available, so a meaningful local native benchmark could not be run without
installing unapproved build and runtime components. This limitation does not
affect the architectural findings.

## Distribution and compliance findings

- The supported upstream native setup depends on a separately installed
  PDFium library and therefore does not preserve the default one-executable
  runtime contract.
- Native builds add platform-specific C/C++ toolchains, library discovery,
  cross-compilation, and dynamic-loader behavior across Windows, Linux, and
  macOS.
- The subprocess implementation also requires packaging and locating a
  platform-specific worker executable.
- Statically linking PDFium could potentially restore a single executable,
  but `go-pdfium` does not document that as its standard setup. It would
  require a reproducible per-platform PDFium build and a verified static-link
  design.
- Any native PDFium artifact would require its own pinned revision, digest,
  build provenance, third-party inventory, and notice audit. Using a
  precompiled binary does not satisfy those requirements by itself.

## Decision

Do not add a native backend in Phase 3. The potential performance improvement
does not currently justify a runtime prerequisite, a second executable,
reduced process isolation, or an unaudited custom static-link pipeline. Keep
`pdfium-wasm` as the only backend and do not expose an engine-selection option
with only one valid choice.

Reconsider this decision only when all of the following are available:

1. representative TransmuteMD benchmarks show a material user-facing
   bottleneck;
2. reproducible native PDFium builds exist for every supported target;
3. packaging preserves the approved distribution contract;
4. the exact native artifacts pass the dependency and notice audit; and
5. the native adapter passes the shared extractor contract and corpus.

## Sources

- [`go-pdfium` `v1.21.0` README](https://github.com/klippa-app/go-pdfium/blob/v1.21.0/README.md)
- [`single_threaded` implementation](https://github.com/klippa-app/go-pdfium/blob/v1.21.0/single_threaded/single_threaded.go)
- [CGo `pkg-config` linkage](https://github.com/klippa-app/go-pdfium/blob/v1.21.0/internal/implementation_cgo/link.go)
- [`multi_threaded` implementation](https://github.com/klippa-app/go-pdfium/blob/v1.21.0/multi_threaded/multi_threaded.go)
- [ADR-0002: Use embedded PDFium WebAssembly](../decisions/0002-use-embedded-pdfium-wasm.md)
- [Dependency and notice policy](../dependency-and-notice-policy.md)
