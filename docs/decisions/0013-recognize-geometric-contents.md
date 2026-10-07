# ADR-0013: Recognize geometric contents rows before column ordering

- **Status:** Accepted
- **Date:** 2026-10-07

## Context

Some born-digital PDFs place contents titles and page numbers in separate text
runs without retaining dotted leaders as text. Short titles can end before the
page center while their page numbers sit at the far-right margin. Generic
two-column inference then treats the titles and page numbers as independent
columns and changes their row relationships. Longer titles may cross the page
center and remain joined, producing inconsistent behavior on the same page.

Accepting any numbered text followed by an integer would reclassify ordinary
body text, quantities, and true two-column layouts. Link annotations are useful
evidence but are not present in every authored contents list. Contents may also
continue onto a following page without repeating the contents heading.

## Decision

Before generic column ordering, analysis may recognize a leaderless contents
cohort after a contents heading when all of the following hold:

- at least two rows have a valid hierarchical number and semantic title;
- each title is vertically matched with a positive page-number run in the
  far-right page zone, either on a separate physical line or after a
  substantial within-line gap;
- the page-number runs share a scale-tolerant right edge; and
- the cohort contains a root-level entry.

The complete cohort is validated before modification. Each accepted title and
page number becomes one atomic analysis line carrying contents metadata, so
generic column ordering cannot separate the pair. Existing dotted-leader
recognition remains supported.

A heading-free cohort is accepted only on the immediately following page when
the preceding accepted contents cohort reaches the bottom content band. It
must independently satisfy the repeated geometric evidence. Continuation
entries are merged into the preceding semantic list using their numbered
depth; if the required parent hierarchy is unavailable, the lists remain
separate rather than inventing a relationship.

## Consequences

- Leaderless contents retain title/page relationships and are not promoted to
  document headings.
- Mixed short and long entries use one semantic path despite different
  center-gutter splitting.
- True two-column ordering remains unchanged outside validated contents
  cohorts.
- Multi-page contents can preserve hierarchy without requiring repeated
  headings.
- PDFs using unnumbered contents, Roman-numeral page labels, inconsistent
  alignment, or fewer than two rows remain plain text.
- The decision remains engine-neutral and adds no renderer or semantic-model
  dependency.

## Alternatives considered

- **Disable column ordering on every page containing a contents heading:**
  rejected because a contents page can contain other genuine columns, and it
  would not establish title/page relationships.
- **Accept any numbered line with a trailing integer:** rejected because it
  would misclassify ordinary numbered prose and quantities.
- **Require dotted leaders:** rejected because the visual leaders may not be
  represented as extractable text.
- **Require internal link annotations:** rejected because useful authored
  contents lists do not consistently contain links.
- **Recognize heading-free cohorts on any page:** rejected because aligned
  numbered body data is otherwise indistinguishable from a contents
  continuation without preceding-page context.
