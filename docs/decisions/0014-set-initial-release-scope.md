# ADR-0014: Set the initial release scope

- **Status:** Accepted
- **Date:** 2026-10-13

## Context

ADR-0004 selected Windows, Linux, and macOS, leaving CPU architectures for a
separate decision. The converter and document-fidelity work are complete in
the implementation plan, but release engineering needs a bounded target
matrix, versioning policy, and publication process.

Binary distribution remains blocked by the audit of the pinned `go-pdfium`
dependency and its embedded PDFium WebAssembly artifact. Selecting a release
scope does not resolve or waive that blocker.

## Decision

### Supported target matrix

Select five operating-system and architecture pairs for the initial release:

| Operating system | Go architecture | User-facing name |
|---|---|---|
| Windows | `amd64` | x64 |
| Linux | `amd64` | x64 |
| Linux | `arm64` | ARM64 |
| macOS | `amd64` | Intel |
| macOS | `arm64` | Apple Silicon |

Native Windows ARM64, 32-bit architectures, and other Go targets are deferred.
Each selected target must pass native, clean-environment executable checks
before shipping; successful cross-compilation alone does not establish
support. Checks must exercise PDF conversion and transactional output without
an installed PDF engine or runtime. Record the tested OS versions and any
minimum OS requirements with the release documentation.

This is an accepted target scope, not a claim that the complete matrix has
already been verified.

### Versioning and compatibility

Use Semantic Versioning with `vX.Y.Z` Git tags. The first public release is
`v0.1.0`.

During the `0.x` series:

- Patch releases contain compatible bug and security fixes. Corrected PDF
  interpretation or Markdown generation may change output.
- Minor releases introduce capabilities or intentional compatibility
  changes, which must be identified in release notes.
- Preserve CLI options, exit codes, and transactional-output behavior where
  practical. Any intentional CLI-contract change still requires the
  corresponding documentation and tests.

Reserve `v1.0.0` for an explicit stable CLI compatibility commitment. It does
not promise perfect PDF fidelity, byte-identical Markdown across versions, or
a stable public Go library API.

### Release process and packaging

Release on demand, initiated by the owner rather than on a fixed schedule:

1. Select a reviewed commit and create its version tag.
2. Tag-triggered automation builds and checks the five targets and prepares a
   draft GitHub Release containing release archives, checksums, a software
   bill of materials for each artifact, and release notes.
3. The owner reviews the artifacts and results and explicitly publishes the
   draft only after all distribution gates pass.

Each archive contains the self-contained executable, project license, and
required third-party license and notice bundle. Follow the dependency and
notice policy for artifact auditing, provenance, vulnerability scanning, and
scheduled dependency updates.

The PDFium audit and applicable distribution checks must pass before binaries
are shared or uploaded to publicly accessible locations. A draft release is
not an exemption from those requirements.

Defer Windows code signing and macOS code signing/notarization for the initial
release. Document unsigned-binary trust warnings and launch limitations in the
installation instructions. Signing may be added through a later decision.
Package-manager distribution and automatic in-application updates are outside
the initial release scope.

## Consequences

- Five explicit targets cover x64 Windows, x64/ARM64 Linux, and both Intel and
  Apple Silicon Macs without committing to every Go target.
- CI and release checks must explicitly cover architectures as well as
  operating systems; an OS-only matrix is insufficient release evidence.
- Starting at `0.1.0` permits deliberate evolution while preserving documented
  CLI behavior and explaining meaningful output changes.
- Draft releases retain owner approval between automation and publication.
- Unsigned binaries may encounter Windows or macOS trust restrictions;
  passing build and conversion checks does not guarantee frictionless launch.
- Release automation and target verification remain implementation work, and
  binary distribution remains blocked pending the PDFium audit.

## Alternatives considered

- **x64 and ARM64 on all three operating systems:** adds native Windows ARM64
  coverage, but expands the initial verification and maintenance commitment.
- **Windows x64, Linux x64, and macOS ARM64 only:** smaller matrix, but excludes
  Intel Macs and ARM64 Linux users.
- **Start at `1.0.0`:** implies an immediate stable CLI compatibility
  commitment before the first public release has been exercised.
- **Require a public release candidate first:** possible later, but not a
  prerequisite for the selected `0.1.0` release.
- **Automatically publish after a tag:** removes the explicit owner review
  of final artifacts and distribution gates.
- **Require signing and notarization before launch:** improves platform trust
  behavior but adds certificate, credential, and pipeline work to the first
  release.
