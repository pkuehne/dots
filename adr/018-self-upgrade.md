# ADR 018: Self-upgrade via a dedicated `dots upgrade` command

## Status

Accepted

## Context

Upgrading dots meant re-running the curl installer or rebuilding from source.
[Issue #22](https://github.com/pkuehne/dots/issues/22) asked dots to manage
itself. One option was to route this through the existing `[tools]` machinery by
treating dots as just another tool. That path has problems:

- Tool installation is **presence-based** and short-circuits when the binary is
  already on `PATH` (`internal/tools/tools.go`), so dots would never upgrade
  until version tracking ([issue #25](https://github.com/pkuehne/dots/issues/25))
  also existed.
- It would require the user to opt in via config/preset; self-upgrade should
  work straight after the installer, before any `dots.toml` exists.
- The tools path overwrites with a plain `os.WriteFile`, which is unsafe for a
  binary that is currently executing.

dots also uniquely knows its own version (compiled in via `main.version`) and
its own release-asset naming (`dots_{goos}_{goarch}`), so it does not need the
generic installed-version detection that makes #25 hard.

## Decision

Add a dedicated top-level `dots upgrade` command (annotated `skipConfig`, so it
needs no repo). It downloads the matching release asset for the running
OS/arch, backs up the current binary to `<exe>.dots-bak`, and **atomically
replaces** the running binary by writing a temp file in the same directory and
`os.Rename`-ing over it (valid on Linux/macOS/termux — the running process keeps
the old inode). Flags: `--check` (report only), `--dry-run`, `--force`
(reinstall same/dev), and `--version` to pin a release.

The GitHub-release client (resolve latest/by-tag, download asset, token + rate
limit handling) is factored out of `internal/tools` into a shared
`internal/ghrelease` package, used by both tool installation and self-upgrade.
Version comparison uses `golang.org/x/mod/semver`.

Releases are also switched to plain `vX.Y.Z` tags by setting
`include-component-in-tag: false` in `release-please-config.json`. Previously
release-please tagged releases `dots-vX.Y.Z` and baked that into
`dots --version`. The first binary that ships `dots upgrade` is the next release,
which already carries a plain tag, so no upgrade-capable binary ever sees the old
`dots-` scheme — the self-updater treats a tag as a plain `vX.Y.Z` version and
carries no legacy-prefix handling.

## Consequences

- `dots upgrade` works with no config and replaces the live binary safely with a
  rollback copy at `<exe>.dots-bak`.
- `internal/ghrelease` is reusable; a future #25 (tool version tracking) can
  build on it rather than duplicating the release client.
- Adds `golang.org/x/mod` to the dependency set — a small, Go-team-maintained
  module, per the dependency invariant in CLAUDE.md.
- New release tags are plain `vX.Y.Z`; `dots --version` no longer shows a
  `dots-` prefix.

## Alternatives Considered

- **dots as a `[tools]` entry**: rejected — blocked on #25, needs config opt-in,
  and overwrites the live binary unsafely.
- **Hand-rolled version comparison**: rejected — `golang.org/x/mod/semver` is
  small, authoritative, and handles pre-release ordering correctly.
