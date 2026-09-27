# Initial CLI Contract

## 1. Purpose

This document defines the user-visible command-line behavior for the first
TransmuteMD release. Implementation and integration tests should treat it as
the source of truth. Changes to the contract require corresponding updates to
this document, tests, the README, and the implementation plan.

## 2. Goals

- Make conversion of one local PDF convenient.
- Remain predictable and composable in scripts.
- Never overwrite a file without explicit permission.
- Never report partial conversion as success.
- Keep the initial interface small enough to remain stable.

## 3. Command synopsis

```text
transmutemd [options] <input>
```

Exactly one input path is required. The first release does not use a
`convert` subcommand because conversion is the program's primary operation.

Examples:

```console
transmutemd report.pdf
transmutemd report.pdf --output report.md
transmutemd report.pdf --output -
transmutemd report.pdf --force
```

## 4. Options

| Option | Meaning |
|---|---|
| `-o, --output <path>` | Write Markdown to the given path. Use `-` for standard output. |
| `-f, --force` | Permit replacement of an existing output file. |
| `-h, --help` | Print usage information and exit successfully. |
| `--version` | Print version information and exit successfully. |

Unknown options, missing option values, no input, or more than one input are
usage errors.

`--force` is invalid when output is standard output because there is no file
to replace.

## 5. Input behavior

- Input is one local file path.
- Directories are rejected.
- Standard input is not supported initially.
- Format recognition is content-based rather than extension-based. A valid PDF
  may therefore use an extension other than `.pdf`.
- A file that is not recognized as PDF is rejected before output is created.
- Password-protected PDFs are not supported initially and produce a clear
  conversion error without prompting for a password.
- A PDF with no extractable text produces a clear conversion error. This
  commonly indicates a scanned document that will require future OCR support.

## 6. Output behavior

When `--output` is omitted, output is written beside the input:

```text
C:\documents\report.pdf -> C:\documents\report.md
/documents/report.pdf   -> /documents/report.md
```

The final filename extension is replaced with `.md`; `.md` is appended when
the input has no extension. If replacing the extension would make the default
output resolve to the input itself, `.md` is appended instead.

When `--output <path>` is supplied:

- relative paths are resolved from the current working directory;
- the parent directory must already exist;
- an existing file is rejected unless `--force` is present;
- a directory is rejected;
- the output path must not resolve to the input file.

`--output -` writes Markdown to standard output. Successful file conversion is
otherwise quiet unless the converter reports a non-fatal structure warning.

Markdown output is UTF-8 with LF line endings on every platform.

## 7. Transactional output

Conversion must complete before user-visible output is committed:

- File output is first written to a temporary file in the destination
  directory. The destination is replaced only after extraction, analysis,
  rendering, writing, and closing succeed.
- A failed conversion must not create a new destination or alter an existing
  destination.
- Standard output must not receive Markdown until conversion and rendering
  have completed successfully.

Temporary files must be removed after both successful and failed operations.

## 8. Diagnostics

- Markdown is written only to the selected output.
- Errors and diagnostics are written to standard error.
- Successful conversion prints no confirmation message.
- A successful conversion may print `warning:` diagnostics for uncertain or
  omitted structure. Warnings identify the page when known, do not alter
  Markdown, and retain exit code `0`.
- Errors identify the relevant path and operation without exposing document
  content or internal stack traces.
- The initial CLI has no interactive prompts and no progress display.

## 9. Exit codes

| Code | Category | Examples |
|---:|---|---|
| `0` | Success | Conversion completed, help shown, or version shown |
| `2` | Usage | Invalid option, missing input, multiple inputs, invalid option combination |
| `3` | Input | Missing/unreadable input, directory input, unrecognized format |
| `4` | Conversion | Malformed or encrypted PDF, no extractable text, extraction/analysis/rendering failure |
| `5` | Output | Missing/unwritable destination directory, existing output without `--force`, write/replace failure |
| `130` | Interrupted | Conversion cancelled by Ctrl+C or the equivalent interrupt signal |

An error belongs to one category even if its underlying cause could fit
several. The CLI maps typed application errors to these stable codes; internal
packages do not choose process exit codes.

## 10. Signals and cancellation

The executable translates process interruption into context cancellation.
Long-running stages check cancellation at meaningful boundaries, release
PDFium resources, remove temporary output, and return exit code `130`.

## 11. Initial non-goals

- Multiple input files or recursive conversion.
- Reading a PDF from standard input.
- Password entry or password files.
- Interactive overwrite confirmation.
- Progress bars, color configuration, or structured diagnostics.
- Config files or environment-variable configuration.

These features can be added later without changing the basic one-input
contract.

## 12. Required integration cases

The initial CLI test suite must cover:

- help and version output;
- missing, extra, and unknown arguments;
- default sibling output naming;
- explicit file output and explicit standard output;
- existing output with and without `--force`;
- missing output directory;
- input and output resolving to the same file;
- valid PDF content with a non-`.pdf` extension;
- unreadable, non-PDF, malformed, encrypted, and textless input;
- no partial file or standard output after conversion failure;
- interruption cleanup and exit code;
- separation of Markdown on stdout from diagnostics on stderr.
- successful output accompanied by non-fatal structure warnings on stderr.
