# ADR-0012: Recover explicitly evidenced empty fields

- **Status:** Accepted
- **Date:** 2026-09-28

## Context

Forms often represent an empty control as a blank ruled cell or a degenerate
authored text position rather than an interactive PDF widget. Other empty
cells, signature bands, decorative rectangles, and result grids can have
similar geometry. Treating every blank region as a checkbox or named field
would invent semantics, while omitting every region loses useful, explicit
document structure.

Markdown can reliably represent an empty checkbox and a labeled blank, but it
cannot recover an unknown label, signature role, or checked state from
absence alone.

## Decision

Extraction may preserve a degenerate text object as an engine-neutral text
placeholder containing only its position and nominal font size. PDF adapters
may also traverse nested form objects to expose their ordinary path rulings.
Neither observation carries form semantics at extraction time.

Analysis may emit unchecked controls in an accepted table only when:

- one column occupies no more than one quarter of the table width and no more
  than three body-row heights;
- the table has at least two body rows;
- every body cell in that column contains no text or links and exactly one
  centered placeholder of a plausible size; and
- every body row contains non-empty text in another column.

The complete column is accepted or rejected as one pattern. Accepted cells
become typed unchecked controls and render as `[ ]`.

Analysis may emit a labeled blank field only when a complete, bounded
rectangle:

- is outside an accepted table and contains no text;
- contains exactly one authored placeholder;
- is limited in size relative to the page; and
- has one nearby text line vertically aligned immediately to its left.

The label and its links become a typed blank-field block, rendered as the
escaped label followed by an underscore blank. Checked state requires
separate positive mark evidence and is not inferred by this rule. Unlabeled
signature bands, ambiguous rectangles, ordinary empty table cells, and
incomplete control columns remain unchanged.

## Consequences

- Repeated empty choices and explicit labeled blanks remain useful in
  Markdown without relying on known labels or PDF widget APIs.
- Empty result grids continue to render as ordinary empty cells.
- Signature roles and checked state are not invented.
- Degenerate text placement and nested path traversal remain isolated from
  PDFium behind the engine-neutral layout model.
- Additional page-object inspection costs time proportional to the bounded
  page-object count.

## Alternatives considered

- **Use PDF widget annotations only:** rejected because visually meaningful
  controls are commonly authored as ordinary page content without widgets.
- **Treat every blank ruled cell as a checkbox:** rejected because result
  grids, metadata, and decorative layout would produce false controls.
- **Infer field meaning from labels or section names:** rejected because
  document-specific vocabulary does not generalize.
- **Emit unlabeled signature lines as named fields:** rejected because the
  source does not establish which line belongs to which signer or value.
- **Render raw HTML form elements:** rejected because interactive HTML is
  outside the Markdown renderer contract and would imply behavior absent from
  the source.
