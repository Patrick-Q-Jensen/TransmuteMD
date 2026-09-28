# Architecture Decision Records

Architecture Decision Records (ADRs) preserve why material technical choices
were made. They complement the current-state description in
[`architecture.md`](../architecture.md).

## Index

| ADR | Decision | Status |
|---|---|---|
| [0001](0001-use-go.md) | Use Go as the implementation language | Accepted |
| [0002](0002-use-embedded-pdfium-wasm.md) | Use embedded PDFium WebAssembly for initial PDF extraction | Accepted |
| [0003](0003-separate-extraction-analysis-and-rendering.md) | Separate extraction, semantic analysis, and rendering | Accepted |
| [0004](0004-target-windows-linux-and-macos.md) | Target Windows, Linux, and macOS | Accepted |
| [0005](0005-preserve-ambiguous-tables-and-code-as-text.md) | Preserve ambiguous tables and code as text | Superseded in part by ADR-0007 |
| [0006](0006-emit-explicit-heading-anchors.md) | Emit explicit heading anchors for resolved internal links | Accepted |
| [0007](0007-reconstruct-reliable-ruled-tables.md) | Reconstruct reliable ruled tables | Accepted |

## Process

Create an ADR when a decision:

- materially constrains the architecture or dependencies;
- has meaningful alternatives or tradeoffs;
- affects portability, security, compatibility, or operations; or
- would otherwise cause future contributors to revisit settled reasoning.

Copy [`template.md`](template.md), assign the next four-digit number, and use a
short imperative filename. ADRs use one of these statuses:

- **Proposed:** under discussion and not yet binding;
- **Accepted:** current decision;
- **Deprecated:** retained for history but no longer recommended;
- **Superseded:** replaced by another ADR, which must be linked.

Accepted ADRs are immutable historical records except for typo corrections and
clarifying links. A changed decision receives a new ADR that supersedes the
old one. Update this index and the implementation plan in the same change.
