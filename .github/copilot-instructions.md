# GitHub Copilot instructions

Follow the repository-wide instructions in `AGENTS.md`; it is the authoritative
source for architecture, implementation, testing, documentation, dependency,
and Git workflow rules.

Before making changes:

1. read the relevant documents linked from `AGENTS.md`;
2. inspect existing code and tests for established patterns;
3. identify affected implementation-plan items and architectural boundaries.

Keep PDFium isolated inside its adapter, preserve transactional CLI output, and
maintain portability across Windows, Linux, and macOS. Update the living plan
for material progress or scope changes. Do not commit unless the user
explicitly requests it.

