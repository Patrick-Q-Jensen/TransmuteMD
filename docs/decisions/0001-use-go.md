# ADR-0001: Use Go as the implementation language

- **Status:** Accepted
- **Date:** 2026-09-23

## Context

TransmuteMD is a command-line document converter intended to be easy to
download and run without installing a language runtime. The implementation
must support multiple desktop operating systems, process untrusted documents
reliably, and remain approachable for ongoing maintenance.

C, C#, Rust, and Go were considered. PDF parsing performance is expected to be
dominated by the extraction engine and document analysis rather than language
overhead.

## Decision

Implement TransmuteMD in Go. Use the module path
`github.com/Patrick-Q-Jensen/TransmuteMD` and initially require Go 1.27.1.

Prefer the standard library for CLI and infrastructure concerns until an
external dependency provides clear value.

## Consequences

- Release artifacts can be self-contained native executables.
- Cross-platform builds and concurrency are straightforward.
- The language and tooling are comparatively small and approachable.
- Garbage collection simplifies ownership but provides less deterministic
  memory control than Rust or C.
- Native PDF integrations may require CGo and complicate cross-compilation.
  The initial embedded WebAssembly engine avoids that constraint.
- The PDF extraction ecosystem is less comprehensive than ecosystems with
  established native engines, so extraction remains behind an interface.

## Alternatives considered

- **Rust:** strong safety and resource control, but greater implementation and
  contributor complexity.
- **C#:** productive and cross-platform, but self-contained deployments are
  generally larger and may complicate the single-download goal.
- **C:** direct access to mature PDF engines, but substantially greater memory
  safety, portability, and maintenance burden.

