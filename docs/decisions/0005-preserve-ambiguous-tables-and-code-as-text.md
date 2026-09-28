# ADR-0005: Preserve ambiguous tables and code as text

- **Status:** Superseded in part by
  [ADR-0007](0007-reconstruct-reliable-ruled-tables.md)
- **Date:** 2026-09-27

## Context

PDFs expose positioned glyphs rather than table cells, source code, or
Markdown structure. Alignment can suggest a table, while monospaced fonts and
regular indentation can suggest code, but the same evidence occurs in forms,
multi-column prose, financial layouts, and ordinary typography. Incorrect
structure is more damaging than readable plain text because it changes
relationships between values or gives whitespace semantic meaning.

Markdown pipe tables also cannot faithfully represent merged cells or many
multiline layouts. Code language cannot be inferred reliably from PDF layout
alone.

## Decision

Phase 2 does not add table or code block semantics. Content that resembles a
table or code remains ordinary text in deterministic reading order. Detection
may report an engine-neutral diagnostic, but it must not discard text, invent
cells, infer a programming language, or emit raw HTML.

Future table support must require repeated row and column alignment with a
rectangular cell assignment. It will introduce an engine-neutral semantic
table model before Markdown rendering. Layouts with merged cells, ambiguous
column membership, or multiline cells will continue to fall back to text
unless a lossless representation is designed.

ADR-0007 supersedes this table decision with acceptance criteria for reliable
ruled tables. The code-block decision remains accepted.

Future code-block support must require multiple adjacent lines with consistent
monospaced-font evidence, left alignment, line spacing, and reconstructable
indentation. It will introduce an engine-neutral code block with no inferred
language by default. Inline code inference remains separate work.

## Consequences

- Phase 2 preserves all extractable content even when structural confidence is
  low.
- Users do not receive misleading table cells, fabricated delimiters, guessed
  code languages, or unsafe raw HTML.
- Table-like and code-like documents remain less structured until dedicated
  semantic models and fixtures are implemented.
- Diagnostics can make the fallback visible without changing successful
  Markdown output.

## Alternatives considered

- **Emit Markdown tables from aligned text immediately:** rejected because
  alignment alone cannot reliably distinguish tables from columns or forms,
  and pipe tables cannot represent common PDF table features.
- **Emit fenced code for every monospaced line:** rejected because monospaced
  fonts are also common in identifiers, tables, and document templates.
- **Use raw HTML for complex tables:** rejected because it expands the output
  and security surface and does not solve uncertain structure detection.
- **Drop uncertain regions:** rejected because preserving readable source text
  is a core analysis rule.
