# ADR-0009: Recover sparse ruled tables from local structural evidence

- **Status:** Superseded in part by
  [ADR-0010](0010-flatten-synchronized-table-records.md) and
  [ADR-0011](0011-unfold-stacked-field-bands.md)
- **Date:** 2026-09-28
- **Supersedes in part:**
  [ADR-0007](0007-reconstruct-reliable-ruled-tables.md) and
  [ADR-0008](0008-infer-columns-in-horizontally-ruled-tables.md)

## Context

The initial ruled-table detector intentionally required populated body rows,
visually distinct headers in composite regions, and repeated column anchors
across at least two body rows. Those safeguards recover dense data tables but
reject several structures that remain unambiguous in PDF geometry:

- complete ruled grids whose body cells are intentionally blank;
- complete grid segments whose header typography matches the body;
- short horizontal-only tables with one body row; and
- adjacent horizontal-only tables whose local column schemas differ.

Applying one column schema to an entire ruled region is too coarse, while
relaxing the evidence requirements for every aligned layout would confuse
prose and forms with tables.

## Decision

TransmuteMD may reconstruct these sparse ruled tables using local structural
evidence:

- A complete, stable rectangular grid with a populated header may retain
  intentionally empty body rows. Empty cells remain empty and no content is
  invented.
- A complete grid segment may use its repeated physical lattice as header
  evidence when typography is not distinct and at least one header column is
  intentionally graphic or blank throughout the body. A segmented grid with
  no body text still requires a bold populated header.
- A horizontal-only ruled region is evaluated as local candidate schemas
  rather than one page-wide schema. Visually distinct header or title bands
  may delimit candidates.
- A one-body-row horizontal table may be accepted only when at least two
  precise column starts occur in both the header and body, the header is
  visually distinct, edge cells are populated, and no text crosses an
  inferred boundary.
- Short sparse horizontal tables may leave internal body cells empty. Column
  evidence may scale with the number of body rows, but every inferred boundary
  must originate in the header and receive body support.
- Multi-line content remains confined to its physical row band and inferred
  cell. Merged cells, ambiguous boundary crossings, and unsupported form
  semantics continue to fall back to plain text.

These rules use only engine-neutral rulings, text bounds, row bands, and style
evidence. They do not inspect known labels, identifiers, or section numbers.

## Consequences

- Empty template tables can retain their columns and blank Markdown rows.
- Short reference tables and adjacent horizontal schemas can be recovered
  without document-specific rules.
- Complete grid geometry can compensate for regular-weight headers.
- Local candidate evaluation avoids diluting strong anchors across unrelated
  tables that share the same outer borders.
- Blank checkboxes, signatures, and interactive field meaning are not
  reconstructed; only reliable surrounding table structure is preserved.
- Conservative fallback remains necessary when row or column assignment is
  ambiguous.

## Alternatives considered

- **Require populated body cells:** rejected because a complete ruled lattice
  already establishes intentionally blank table structure.
- **Require every segmented header to be bold:** rejected because complete
  repeated grid geometry can provide stronger evidence than typography.
- **Lower the existing threshold for every horizontal region:** rejected
  because unrelated schemas would still be combined and false positives would
  increase.
- **Recognize known labels such as `Ref`, `Revision`, or `Checkbox`:** rejected
  because label-specific behavior would not generalize to other documents.
- **Treat blank cells as interactive Markdown controls:** rejected because the
  PDF geometry does not establish field state or interaction semantics.
