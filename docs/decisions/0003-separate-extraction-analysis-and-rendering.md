# ADR-0003: Separate extraction, semantic analysis, and rendering

- **Status:** Accepted
- **Date:** 2026-09-24

## Context

PDFs describe visual content rather than Markdown semantics. A PDF engine can
decode text and geometry, but decisions such as reading order, paragraphs,
headings, lists, and tables require separate heuristics. Combining these
concerns with PDFium calls would make engine replacement and focused testing
difficult.

TransmuteMD may also support additional input formats and output options in
the future.

## Decision

Implement conversion as three explicit stages:

1. **Extraction** maps a source document into an engine-neutral physical
   layout model.
2. **Analysis** maps physical layout into an engine-neutral semantic document.
3. **Rendering** maps the semantic document into Markdown.

Application orchestration coordinates the stages. PDFium types and lifecycle
operations remain inside the PDFium adapter. Analysis and rendering must not
import PDFium packages.

## Consequences

- Layout heuristics and Markdown generation can be unit tested without
  starting PDFium.
- Extractor implementations can share contract tests.
- PDF engines can be replaced without rewriting analysis and rendering.
- Future formats can target the same semantic model where their capabilities
  overlap.
- Explicit intermediate models introduce mapping code and require careful
  decisions about coordinates, optional capabilities, and information loss.
- Engine-specific features may require neutral optional fields rather than
  leaking backend types.

## Alternatives considered

- **Render Markdown directly from PDFium output:** initially shorter, but
  tightly couples every heuristic and test to one engine.
- **Combine extraction and analysis:** avoids one model boundary, but makes it
  difficult to distinguish observed layout from inferred semantics.
- **Use one universal document model:** simpler in appearance, but mixes
  physical and semantic concepts and obscures stage ownership.

