# ADR-0010: Flatten synchronized records in tall horizontal table bands

- **Status:** Accepted
- **Date:** 2026-09-28
- **Supersedes in part:**
  [ADR-0007](0007-reconstruct-reliable-ruled-tables.md),
  [ADR-0008](0008-infer-columns-in-horizontally-ruled-tables.md), and
  [ADR-0009](0009-recover-sparse-ruled-tables.md)

## Context

Some horizontally ruled tables use one tall physical body band for several
logical records. Values in leading columns may visually span those records,
while later columns repeat aligned descriptions, dates, or other values.
Treating each ruled band as exactly one semantic row collapses distinct
records into a single Markdown cell.

Markdown pipe tables cannot represent row spans. Duplicating a spanning value
into every logical row would invent source content, while refusing every tall
band loses structure that repeated cross-column geometry can establish
without guessing.

## Decision

After a horizontal table has satisfied the local-schema and column-assignment
criteria from ADR-0008 and ADR-0009, TransmuteMD may flatten a tall body band
into multiple semantic rows only when all of these conditions hold:

- the band contains line groups separated by materially more than ordinary
  wrapped-line spacing;
- at least two separated logical record starts are each aligned across two or
  more inferred columns;
- nearby aligned lines are retained as continuation text rather than new
  records;
- every source text line belongs unambiguously to one logical interval;
- every resulting logical row has content in at least two columns; and
- no logical boundary crosses source text.

Content from a cell that visually spans several logical records is emitted
only in the first logical row where it occurs. Later rows use empty cells;
the analyzer does not duplicate the value. Stacked header lines remain within
their physical header cells and are flattened as ordinary wrapped cell text.
Links remain attached to their assigned cell text.

If separated content cannot be synchronized across columns, the entire
candidate remains plain text and receives the existing regional fallback
diagnostic. The engine-neutral semantic table model and Markdown renderer do
not gain row-span metadata or raw HTML behavior.

## Consequences

- Repeated logical records inside a tall ruled band no longer collapse into
  one Markdown row when their geometry is independently synchronized.
- Blank continuation cells preserve source text exactly without pretending
  that Markdown supports row spans.
- Ordinary wrapped body text and stacked headers remain in one semantic row.
- Irregular or one-column-only record sequences continue to degrade
  conservatively to plain text.
- The decision remains extraction-engine-neutral because it uses only ruled
  bands, inferred columns, text-line bounds, and existing link assignments.

## Alternatives considered

- **Duplicate spanning values into every row:** rejected because the source
  does not repeat those values and Markdown readers could treat the copies as
  independent records.
- **Collapse every physical band into one row:** rejected because it combines
  separately aligned records and destroys source relationships.
- **Emit HTML row spans:** rejected because raw HTML remains outside the
  renderer contract and would expand the output and validation surface.
- **Recognize known labels or revision formats:** rejected because the
  behavior must generalize to unrelated tables.
