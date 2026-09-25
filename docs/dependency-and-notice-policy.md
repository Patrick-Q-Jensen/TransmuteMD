# Dependency and Notice Policy

## 1. Purpose

This policy defines how TransmuteMD selects, records, updates, and distributes
third-party software. It applies to Go modules, embedded binaries and
WebAssembly, generated code, build tools that contribute code to artifacts,
and redistributed test data.

The project is MIT-licensed, but third-party components retain their own
licenses. This document is a project compliance policy, not legal advice.

## 2. Principles

- Prefer the Go standard library and existing dependencies.
- Prefer actively maintained dependencies with permissive licenses.
- Review transitive and embedded components, not only direct Go modules.
- Preserve the single-download user experience: dependencies may be embedded
  in an archive or executable but must not require end-user installation.
- Apply the same notice standard to internally shared and public artifacts.
- Do not claim a notice set is complete until the exact artifact and pinned
  dependency versions have been audited.

## 3. Adding or updating a dependency

Every dependency change must record or verify:

1. the problem it solves and why existing code is insufficient;
2. the exact module version, artifact digest, or source commit;
3. maintenance activity, supported platforms, and security posture;
4. direct, transitive, embedded, and generated-code licenses;
5. required copyright, attribution, NOTICE, and source-offer obligations;
6. whether it adds CGo, dynamic libraries, subprocesses, network access, or
   another end-user runtime requirement;
7. binary-size, startup, memory, and build consequences where material;
8. relevant tests on Windows, Linux, and macOS.

Commit `go.mod` and `go.sum` changes together. Use Go modules without a
committed `vendor` directory unless a later ADR establishes a concrete need
for vendoring.

Run at least:

```console
go mod tidy
go mod verify
go test ./...
```

Also run the normal formatting and vetting checks, plus dependency-specific
integration tests.

## 4. License acceptance

Permissive licenses are acceptable by default after their conditions have been
reviewed. Common examples include:

- MIT;
- BSD-2-Clause and BSD-3-Clause;
- Apache-2.0;
- ISC;
- Zlib;
- similarly permissive licenses with attribution or notice requirements.

License names alone are insufficient; review the actual text and any
repository-level `NOTICE`, exception, or third-party file.

The following require explicit owner approval and an ADR before use:

- copyleft or weak-copyleft licenses;
- source-available or non-commercial terms;
- custom licenses or unclear licensing;
- dependencies requiring source disclosure, relinking facilities, royalties,
  registration, metering, or a network licensing service;
- dependencies whose transitive or embedded component licenses cannot be
  established.

Do not merge or distribute an artifact when required rights or obligations are
unclear.

## 5. Dependency inventory

When the first external dependency is added, create
`third_party/dependencies.json` as the machine-readable inventory. Each
runtime component should record:

- name and purpose;
- version and source commit where known;
- upstream URL;
- license identifier and local license-file path;
- required notice or attribution;
- whether it is direct, transitive, embedded, generated, or build-derived;
- artifact filename and SHA-256 digest for embedded binaries;
- audit status and relevant notes.

The inventory complements rather than replaces `go.mod`, `go.sum`, full
license texts, and an eventual software bill of materials.

## 6. Notice files

When the first external dependency is added, maintain:

```text
THIRD_PARTY_NOTICES
third_party/
|-- dependencies.json
`-- licenses/
    `-- <component>-LICENSE.txt
```

`THIRD_PARTY_NOTICES` is a concise index containing each component, pinned
version or commit, license, local license filename, copyright or attribution
required by the upstream license, and source URL.

Store complete, verbatim applicable license and NOTICE texts under
`third_party/licenses`. SPDX identifiers and links are useful metadata but do
not replace required license text. Preserve upstream filenames that carry
separate obligations, such as `NOTICE`, copyright files, or attribution
documents.

Generated notice files must be reproducible and reviewed. Automation may
collect candidate texts, but it must not silently discard an unknown license
or declare the result complete.

## 7. PDFium WebAssembly audit

The initial PDF engine requires an artifact-specific audit after a
`go-pdfium` version is pinned. At minimum, verify and record:

- the `go-pdfium` license and version;
- the SHA-256 digest of the embedded `pdfium.wasm`;
- the PDFium source revision and complete top-level license;
- the source and license of the project that built or patched the WASM module;
- licenses and notices for Go runtime dependencies such as the WebAssembly
  runtime;
- licenses for PDFium's incorporated third-party libraries;
- licenses for Emscripten or other toolchain runtime code incorporated into
  the module.

PDFium does not provide one universal notice file for every build. The
applicable dependency set depends on the exact WASM artifact and its build
configuration. A notice collection from a similarly numbered stock PDFium
build may be used as evidence, but not represented as definitive for a custom
embedded module.

Do not mark the PDFium licensing task complete until the pinned module,
embedded WASM digest, final executable dependency metadata, and accompanying
notice bundle have been reviewed together.

## 8. Internal artifact distribution

During the internal-use phase, no formal release automation, SBOM, or
automated dependency-update service is required.

Any binary shared beyond the developer's own workstation must still be
packaged as an archive containing:

```text
transmutemd[.exe]
LICENSE
THIRD_PARTY_NOTICES
third_party/licenses/
```

These supporting legal files do not introduce a runtime dependency; the
executable remains self-contained. Do not distribute a bare executable once
third-party code with notice obligations is included.

Before sharing an internal artifact:

- run `go version -m` on the executable and compare it with the inventory;
- verify embedded artifact digests where applicable;
- verify that required licenses and notices are present in the archive;
- run the relevant clean-environment smoke test.

## 9. Future public releases

Before the first public release, add:

- a machine-readable SBOM for every release artifact;
- automated vulnerability scanning;
- scheduled dependency-update proposals;
- reproducible notice and license collection with a fail-closed unknown-license
  check;
- checksums and provenance for distributed archives;
- an audit of the exact binaries built by release automation.

The implementation plan tracks these as deferred release work. Their deferral
does not waive notice requirements for internally shared binaries.

## 10. Security updates

Security fixes are not deferred by the internal-use phase. If a dependency
advisory affects reachable TransmuteMD behavior:

1. assess exploitability and affected versions promptly;
2. update or mitigate the dependency;
3. run the relevant extraction, regression, and portability tests;
4. replace previously shared internal artifacts when appropriate;
5. record material compatibility or architecture consequences.

