# ADR-0011: Unfold synchronized label/value field bands

- **Status:** Accepted
- **Date:** 2026-09-28
- **Supersedes in part:**
  [ADR-0007](0007-reconstruct-reliable-ruled-tables.md) and
  [ADR-0009](0009-recover-sparse-ruled-tables.md)

## Context

Document title blocks, approval stamps, and metadata footers may use one
physical ruled row whose vertical cells each stack a field label above its
value. Adjacent bands can use different column boundaries. Requiring two
physical row bands leaves this content as interleaved plain text, while
combining the bands under one schema invents columns that do not exist.

Markdown pipe tables cannot represent a set of differently subdivided rows in
one table. Each complete local band can, however, become an independent table
when its repeated vertical alignment establishes one header row and one value
row.

## Decision

TransmuteMD may unfold one physical ruled band into a two-row semantic table
only when all of these conditions hold:

- the band has complete outer boundaries and at least two cells separated by
  continuous vertical rules;
- every physical cell contains exactly two non-empty text lines;
- the first line bounds align across every cell;
- the second line bounds align across every cell;
- no text crosses a physical or inferred boundary; and
- all text and links remain assignable to one resulting cell.

The first aligned lines become the Markdown header and the second aligned
lines become the value row. Adjacent bands with different vertical-boundary
signatures are emitted as independent tables. A single-cell band, missing or
extra line, unsynchronized line, or boundary crossing rejects the pattern.
Rejected content remains plain text under the existing regional fallback
policy.

Repeated page furniture remains governed by the existing suppression rules.
The analyzer does not assign interactive form or signature semantics to the
result; it preserves only visible label/value relationships.

## Consequences

- Regular title blocks, approval metadata, and document-control footers can
  become readable Markdown without HTML spans.
- Local schemas remain accurate instead of being widened to a page-level
  union of unrelated columns.
- The same rectangular semantic table model and Markdown renderer remain
  sufficient.
- Irregular stamps, signatures, and merged single-cell notices still degrade
  conservatively to text or repeated-furniture suppression.
- The behavior is engine-neutral because it uses only ruling and text
  geometry plus existing link assignments.

## Alternatives considered

- **Emit one table using the union of every boundary:** rejected because rows
  would acquire columns and relationships absent from the source.
- **Transpose labels and values into a two-column list:** rejected because it
  discards the source grouping between adjacent fields.
- **Emit raw HTML with row and column spans:** rejected because raw HTML
  remains outside the renderer contract and expands the validation surface.
- **Recognize approval or document-control labels:** rejected because
  label-specific behavior would not generalize.
