# ADR-0008: Infer columns in horizontally ruled tables

- **Status:** Accepted
- **Date:** 2026-09-28
- **Supersedes:** The complete-vertical-grid requirement in
  [ADR-0007](0007-reconstruct-reliable-ruled-tables.md)

## Context

Some PDF tables draw a horizontal rule around every row but omit internal
vertical rules. Their columns may still be explicit in the positioned text:
headers and body rows repeatedly begin at the same horizontal coordinates.
Rejecting all such tables loses useful structure even when the row and column
assignment is deterministic.

Alignment alone remains insufficient because ordinary columns, forms, and
prose can share text positions. Column inference therefore needs the row
boundaries and stronger repeated-anchor evidence together.

## Decision

TransmuteMD may infer columns in a horizontally ruled region only when all of
these conditions hold:

- horizontal rules establish a header and at least two body rows;
- the header is visually distinct from the body;
- at least two text columns are present;
- each inferred column start occurs in the header and in at least 75 percent
  of the candidate body rows;
- the first and last inferred columns contain text in every accepted body row;
- wrapped continuation lines remain inside one ruled row band;
- every glyph belongs unambiguously to one inferred cell and crosses no
  inferred boundary; and
- a row that no longer satisfies the edge-column requirement terminates the
  table rather than being absorbed into it.

Column starts are inferred from repeated word-start coordinates, not labels or
document-specific text. The analyzer retains the same engine-neutral semantic
table model and Markdown renderer used for fully ruled tables.

If the evidence is incomplete or conflicting, the region remains plain text
and receives the existing regional fallback diagnostic.

## Consequences

- Horizontally ruled tables with stable positioned-text columns can render as
  Markdown without document-specific rules.
- Headers, titles, and following sections are less likely to be absorbed
  because body rows must populate both edge columns.
- The stricter support threshold excludes short, sparse, and irregular tables
  even when a human can interpret them.
- The heuristic remains backend-neutral because it consumes only neutral row
  rulings, text bounds, and style evidence.

## Alternatives considered

- **Continue requiring every vertical rule:** rejected because reliable row
  boundaries and repeated text anchors can provide equivalent evidence.
- **Infer every aligned borderless table:** rejected because it would confuse
  columns, forms, and ordinary aligned prose with tables.
- **Use known header labels or identifier patterns:** rejected because that
  would be document-specific and fail on unrelated content.
- **Assign columns by equal widths:** rejected because source column widths
  are not necessarily uniform and equal partitioning can move text between
  cells.
