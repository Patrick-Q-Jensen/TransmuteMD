# ADR-0007: Reconstruct reliable ruled tables

- **Status:** Superseded in part by
  [ADR-0008](0008-infer-columns-in-horizontally-ruled-tables.md) and
  [ADR-0009](0009-recover-sparse-ruled-tables.md), and
  [ADR-0010](0010-flatten-synchronized-table-records.md)
- **Date:** 2026-09-28
- **Supersedes:** The table portion of
  [ADR-0005](0005-preserve-ambiguous-tables-and-code-as-text.md)

## Context

PDFs expose positioned text and drawing operations rather than table rows,
columns, or cells. Repeated alignment alone also occurs in multi-column prose,
forms, and page furniture, so converting every aligned region into a table can
silently change relationships between values. The first real-world fidelity
sample contains both regular ruled tables and complex layouts with merged
cells, incomplete borders, and form fields.

Markdown pipe tables require one header row and a rectangular matrix. They do
not represent row spans or column spans portably. Reliable conversion
therefore needs stricter acceptance criteria than table-like appearance.

## Decision

TransmuteMD reconstructs a table only when engine-neutral geometry and text
placement establish all of these conditions:

- a connected ruled region defines at least two rows and two columns;
- every expected horizontal and vertical boundary is present across the
  region, producing a complete rectangular lattice;
- every non-empty text run in the region belongs unambiguously to one cell;
- text does not cross an internal boundary;
- the first row contains text and can serve as the Markdown header;
- every column contains text somewhere in the table; and
- no merged, split, nested, or otherwise spanning cell is required.

The extractor may expose horizontal and vertical ruling segments, but no
PDFium type crosses the adapter boundary. Analysis normalizes near-coincident
segments, finds connected rectangular regions, assigns text by geometry, and
orders wrapped cell text from top to bottom and left to right. The semantic
model stores only the resulting rectangular rows and cells.

Tables that fail any criterion remain in ordinary reading order. Analysis
emits one engine-neutral fallback diagnostic for each rejected ruled region,
not one diagnostic per source line or cell. Unruled aligned text remains plain
text because alignment alone is not sufficient evidence for table semantics.

ADR-0008 supersedes the requirement for every internal vertical boundary when
horizontal rules and repeated text anchors provide equivalent unambiguous
column evidence. The remaining acceptance and fallback rules stay in force.

The Markdown renderer emits validated pipe tables. It escapes cell pipes and
normalizes embedded line breaks to spaces. It does not emit raw HTML or invent
content for merged cells. Code-like text remains governed by ADR-0005.

## Consequences

- Regular bordered tables become useful Markdown tables without coupling the
  semantic model or renderer to PDFium.
- False positives are constrained by requiring complete ruling geometry and
  unambiguous text assignment.
- Borderless tables and complex ruled layouts continue to render as text with
  a specific warning.
- A visually blank header row cannot be represented as a table under this
  decision.
- Backends that cannot expose ruling geometry can still convert text but
  cannot produce semantic tables under this policy.

## Alternatives considered

- **Infer tables from aligned text alone:** rejected because columns, forms,
  and metadata layouts frequently provide the same evidence.
- **Represent merged cells with duplicated or empty content:** rejected
  because either choice invents relationships not present in the source.
- **Emit raw HTML tables:** rejected because it expands the output and
  security surface and still does not resolve uncertain structure.
- **Keep all tables as text:** rejected because complete ruling lattices and
  unambiguous text placement provide sufficient evidence for a conservative
  semantic table.
