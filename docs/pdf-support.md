# PDF support and degradation

TransmuteMD converts extractable PDF text into Markdown. PDF is a presentation
format, so many files do not contain enough reliable structure to reproduce
their visual appearance or original authoring semantics. The converter favors
complete plain text over invented structure and reports warnings only when it
can identify a specific non-fatal fallback.

## Supported

- Born-digital, unencrypted PDFs with extractable Unicode text.
- Page rotation and character-level position, font, weight, italic, and
  rotation evidence exposed by PDFium.
- Paragraphs and font/spacing-based headings.
- Flat body lists and hierarchical contents lists.
- Clear two-column regions separated by a center gutter.
- Repeated page headers and footers when repetition is sufficiently strong.
- Reliable external `http`, `https`, and `mailto` link annotations.
- Internal links that can be matched to a semantic heading on their target
  page.

These are heuristic conversions, not round-trip reproduction of the PDF.

## Unsupported or degraded

| PDF content or feature | Current behavior |
|---|---|
| Password-protected or encrypted documents | Conversion fails without prompting for a password. No output is committed. |
| Scanned or image-only pages | OCR is not performed. An entirely textless document fails with an actionable error; images in mixed documents are omitted. |
| Images, vector graphics, and page backgrounds | Visual content is omitted. Extractable text on the same page is still converted. |
| Tables | No table model is inferred. Text is preserved in geometric reading order; strongly table-like regions produce a warning. |
| Source code | No fenced-code model is inferred. Text is emitted as escaped plain Markdown; strongly code-like regions produce a warning. |
| Nested or interrupted body lists | Nested contents entries are modeled. Other indentation changes or uncertain body-list sequences become separate lists or paragraphs without a warning. |
| Ambiguous, overlapping, or three-or-more-column layouts | Only clear two-column regions are reordered. Other layouts retain conservative geometric order and may read incorrectly without a warning. |
| Unmatched internal links, remote-document links, file-launch actions, JavaScript actions, and unsafe URI schemes | Visible label text is retained. Unsupported actions and unsafe URIs produce a warning; valid internal targets without a matching semantic heading remain plain text. |
| Form fields, comments, non-link annotations, signatures, attachments, bookmarks, metadata, and accessibility tags | Their semantics are not exported. Text visible through ordinary page text extraction may remain, but interactive or document-level data is omitted without a warning. |
| Vertical writing, right-to-left text, and complex scripts | PDFium-decoded characters are retained, but analysis is optimized for horizontal left-to-right text. Reading order and joining are best effort and may be incorrect without a warning. |
| Missing or incorrect font character maps | Extraction depends on PDFium's decoded Unicode. Unmappable characters may be missing or incorrect; TransmuteMD does not reconstruct fonts or run OCR. |
| Corrupt or unsupported PDF structures | Conversion fails with PDFium context when available. No output is committed. |

## Safety limits

The default extractor rejects inputs above these boundaries:

| Resource | Maximum |
|---|---:|
| Source size | 256 MiB |
| Pages | 2,000 |
| Returned character runs | 1,000,000 per document |
| Annotations | 100,000 per document |
| Page width or height | 200,000 points |

A limit failure identifies the observed and maximum values, uses the conversion
failure exit code, and does not create or replace output.

Warnings and successful Markdown are separated as documented in the
[CLI contract](cli-contract.md). Unsupported features are candidates for
future work only when the semantic model and test corpus can represent them
reliably.
