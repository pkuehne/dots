# ADR 017: Use mholt/archives for tool archive extraction

## Status

Accepted

## Context

The `github` install method downloads a release asset and, when it is an
archive, extracts the binary from it. `internal/tools` hand-rolled this:
`safeTarExtractAll` (gzip + `archive/tar`) and `safeZipExtractAll`
(`archive/zip`), each with its own manual Zip-Slip guard and absolute-symlink
check.

This had three problems:

- **Limited formats.** Only `.tar.gz`/`.tgz` and `.zip` were handled. Assets
  shipped as `.tar.xz`, `.tar.bz2` or `.tar.zst` were explicitly rejected
  because adding each decompressor by hand was more code than it was worth.
- **Silent symlink loss.** The extractors materialised directories and regular
  files only; in-archive symlinks were skipped, so tools that ship a binary as a
  symlink into a versioned directory installed nothing.
- **Security-sensitive code, hand-maintained.** Path-traversal and symlink
  guards are exactly the kind of code that is easy to get subtly wrong and
  costly to review repeatedly.

## Decision

Use [`mholt/archives`](https://github.com/mholt/archives) for extraction. A
single `extractArchive` function identifies the format from the asset
(`archives.Identify`) and walks entries through one handler that materialises
directories, regular files and symlinks. `.tar.gz`/`.tgz`, `.tar.bz2`,
`.tar.xz`, `.tar.zst`, `.tar` and `.zip` are all supported uniformly; `isExtractableArchive`
gates which downloaded assets are routed to extraction versus installed verbatim.

The traversal and symlink safety checks are kept in-tree, not delegated:
`archives.FileInfo.NameInArchive` is documented as untrusted, so `sanitizeArchivePath`
still verifies every member resolves within the destination, and symlink targets
are rejected when absolute or when they escape the destination directory.

## Consequences

- `.tar.xz`, `.tar.bz2` and `.tar.zst` assets now install without a workaround.
- In-archive symlinks are extracted instead of silently dropped.
- The bespoke `safeTarExtractAll`/`safeZipExtractAll` pair is replaced by one
  format-agnostic path, shrinking the security-sensitive surface we maintain.
- Adds `mholt/archives` and its transitive decompressors to the dependency set.
  This is a deliberate, maintained addition per the dependency invariant in
  CLAUDE.md: it pulls in several compression libraries, but each is the standard
  Go implementation for its format and replacing hand-rolled extraction with a
  reviewed library is the intended trade.

## Alternatives Considered

- **Keep hand-rolling, add xz/zstd/bzip2 by hand:** rejected — more
  security-sensitive code to maintain for each format, the opposite direction
  from this ADR.
- **Reject non-gzip/zip archives permanently:** rejected — it pushes the problem
  onto users, who must find an alternative asset that may not exist.
