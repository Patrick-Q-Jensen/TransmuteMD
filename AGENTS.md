# TransmuteMD Agent Instructions

These instructions apply to all AI-assisted work in this repository.

## Project context

TransmuteMD is a portable Go CLI that converts documents to Markdown. The
first input format is PDF, extracted with `go-pdfium` and embedded PDFium
WebAssembly. The default release must remain a single executable with no
separately installed runtime or PDF engine.

Read the relevant project documents before changing behavior:

- `README.md` for project scope, layout, and development commands;
- `docs/architecture.md` for boundaries and dependency direction;
- `docs/cli-contract.md` for user-visible CLI behavior;
- `docs/dependency-and-notice-policy.md` before dependency or distribution
  changes;
- `docs/implementation-plan.md` for current status and planned work;
- `docs/decisions/` for accepted architectural reasoning.

Do not treat a task description as permission to contradict an accepted ADR or
the CLI contract silently. Surface the conflict and update the governing
document as part of an intentionally approved change.

## Architecture rules

Preserve the extraction, analysis, and rendering pipeline:

1. extraction produces an engine-neutral physical layout;
2. analysis converts layout into an engine-neutral semantic document;
3. rendering converts the semantic document into Markdown.

PDFium packages and types belong only in `internal/extract/pdf/pdfium`.
Packages under `internal/document`, `internal/analyze`, and
`internal/render/markdown` must not import PDFium.

The CLI owns argument parsing, user-facing diagnostics, and process exit codes.
Application and library packages return contextual errors and must not
terminate the process.

Keep behavior portable across Windows, Linux, and macOS. Use Go path,
filesystem, signal, and process APIs rather than shell-specific assumptions.
Markdown is UTF-8 with LF line endings on every platform.

## Implementation rules

- Use Go 1.27.1 or later and follow standard Go conventions.
- Prefer the standard library until an external dependency provides clear
  value.
- Keep changes focused and avoid speculative abstractions.
- Preserve context cancellation through long-running operations.
- Release PDFium, document, page, file, and temporary resources on every
  success and error path.
- Preserve transactional output: failures must not create, truncate, or
  replace user output.
- Return explicit, contextual errors. Do not hide failures behind empty
  output, silent defaults, or broad recovery.
- Keep PDF engine selection behind internal interfaces; do not expose backend
  types in shared models.
- Do not add OCR, batch conversion, stdin input, or password handling as an
  incidental extension of another task.

## Dependencies

Before adding or updating a dependency:

1. demonstrate why the standard library and existing dependencies are
   insufficient;
2. review maintenance activity, supported platforms, and security posture;
3. verify that its license and transitive licenses are compatible with MIT
   distribution;
4. verify that it does not introduce an unintended end-user runtime
   dependency;
5. document material architectural or release consequences.

Follow the review, inventory, license, and notice requirements in
`docs/dependency-and-notice-policy.md`. A dependency with copyleft, custom, or
unclear terms requires explicit owner approval and an ADR.

Pin dependencies through Go modules and commit both `go.mod` and `go.sum`
changes together.

## Testing

Add tests at the lowest useful layer:

- unit tests for analysis, rendering, validation, and error mapping;
- extractor contract tests for engine behavior;
- golden tests for stable layout and Markdown output;
- CLI integration tests for arguments, streams, files, and exit codes;
- regression fixtures for corrected PDF-specific bugs.

PDF fixtures must follow the provenance and licensing policy in
`docs/implementation-plan.md`. Never commit private user documents or
unverified files downloaded from the internet.

Before completing a code change, run:

```console
go fmt ./...
go vet ./...
go test ./...
```

Run narrower tests during iteration and add race, fuzz, corpus, benchmark, or
cross-platform checks when the affected behavior requires them. Review golden
file changes as behavior changes, not as automatic snapshots to accept.

## Documentation and decisions

Update `docs/implementation-plan.md` in the same change when work materially
completes, adds, removes, blocks, or changes a planned item. Do not update it
for typo-only or mechanically equivalent refactors.

Update user-facing documentation whenever CLI behavior changes. Update
`docs/architecture.md` when the current design changes.

Create a new ADR for a material decision with meaningful alternatives or
portability, security, dependency, or compatibility consequences. Do not
rewrite an accepted ADR to change history; supersede it with a new record.

## Git workflow

- Preserve unrelated working-tree changes.
- Make focused changes that can be reviewed independently.
- Do not create a commit unless the user explicitly requests one.
- When asked to commit, use small logical commits and include directly related
  tests and documentation in the same commit.
- AI-created commits include:

  ```text
  Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
  ```

- Do not amend, rebase, force-push, or discard changes unless explicitly
  requested.

## Completion checklist

Before reporting a task complete:

1. verify the requested behavior rather than a proxy;
2. run the relevant checks;
3. inspect the final diff for unrelated or generated changes;
4. update material plan, architecture, ADR, and user documentation;
5. state any incomplete or unverified work plainly.
