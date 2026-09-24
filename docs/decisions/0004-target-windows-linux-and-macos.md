# ADR-0004: Target Windows, Linux, and macOS

- **Status:** Accepted
- **Date:** 2026-09-24

## Context

TransmuteMD is intended to be a portable end-user CLI. Supporting only the
development platform would limit adoption and delay discovery of path,
terminal, filesystem, and packaging differences.

Go supports cross-compilation for the major desktop operating systems. The
embedded PDFium WebAssembly backend avoids target-specific native PDF
libraries, leaving CI verification and release packaging as the primary
cross-platform work.

## Decision

Target Windows, Linux, and macOS for the initial release family. Distribute a
self-contained executable for each supported operating-system and CPU
architecture pair.

Select and verify the initial CPU architecture matrix separately before
release automation is implemented.

Use platform-independent application behavior:

- UTF-8 Markdown with LF line endings;
- Go path APIs rather than manually constructed separators;
- no reliance on shell-specific features;
- the same CLI options and exit-code categories on every platform.

## Consequences

- Cross-platform behavior is tested from the beginning.
- Users on the three major desktop operating systems receive the same basic
  experience.
- CI and release packaging require an operating-system matrix.
- Platform-specific filesystem replacement, signal, and executable behaviors
  need integration tests.
- Code signing and macOS notarization may require additional release work but
  do not change the conversion architecture.
- Supporting additional operating systems remains possible but is not an
  initial release commitment.

## Alternatives considered

- **Windows only initially:** lowest immediate setup cost, but risks embedding
  Windows-specific assumptions.
- **Linux only initially:** simplifies automation, but does not serve the
  development platform or many desktop users.
- **Best-effort builds for every Go target:** creates an unsupported matrix
  that cannot be tested adequately.

