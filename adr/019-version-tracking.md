# ADR 019: Version tracking for tools and repos

## Status

Accepted

## Context

[Issue #25](https://github.com/pkuehne/dots/issues/25) asked dots to assert a
*target version* for installed tools, including the sentinel `latest`. Until now
`[[tool]]` handling was **presence-based**: `tools check` only verified the
binary existed (via the `check` command or `PATH`), and the `github` method's
`version` field merely pinned which release to download on first install. A tool
installed at an old version was never noticed, and a `latest` tool was installed
once and never refreshed.

The hard part is knowing the *installed* version. Parsing each binary's
`--version` output is brittle (every tool formats it differently and needs
per-tool regexes). dots already resolves a concrete release tag at install time
via `internal/ghrelease`, so the version it installs is known precisely — it
just was not recorded.

The same idea extends to `[[repo]]`: the `ref` field already existed but was
only used as `--branch` at clone time; `update` blindly pulled the default
branch and never asserted the configured ref.

## Decision

**Tools.** Record what dots installs in a machine-local lockfile at
`~/.config/dots/installed.toml` (`internal/lockfile`). Only the `github` method
writes entries — package managers track their own versions. The lockfile is
local state describing this machine, so it lives under `~/.config/dots` rather
than in the dotfiles repo, and is never committed.

- `installGitHub` now returns the installed version; `Install` records it via an
  optional `InstallOptions.Lock`, and the caller persists the batch with
  `Lock.Save()`.
- `version = "latest"` (or unset) means "track the newest release"; a concrete
  value pins a tag. `tools status` compares the lockfile version against the
  resolved target; `tools update` reinstalls only outdated/missing tools and
  re-records them, so it is idempotent.

**Repos.** Treat `ref` as the tracked version. `update` asserts it: a pinned
branch is reset to its remote tip (`checkout -B <ref> origin/<ref>`), a tag or
SHA is checked out detached. `ref` unset or `"latest"` keeps the previous
default-branch pull. `repos status` reports `≠ ref <x>` when HEAD has drifted off
a pinned ref. The existing dirty-repo guard still skips repos with uncommitted
changes so local work is never discarded.

## Consequences

- New `dots tools status` and `dots tools update` commands; `apply` and
  `tools install` now write the lockfile as a side effect of github installs.
- A new `internal/lockfile` package (uses the existing BurntSushi/toml dep; no
  new dependencies).
- Version tracking covers the `github` method only. Other install methods report
  as `untracked` — a deliberate boundary, since their package managers own
  versioning.
- Resolving a `latest` target costs one GitHub API call per tracked tool in
  `status`/`update`; pinned targets need no network. A resolve failure degrades
  to a per-tool "unknown"/error rather than aborting the whole report.
- `repos update` is now ref-aware; pre-existing configs with no `ref` behave
  exactly as before.

## Alternatives Considered

- **Parse `tool --version`**: rejected as the primary mechanism — fragile and
  needs per-tool configuration. The lockfile is deterministic for everything
  dots installs. (A `--version` fallback could be added later for
  externally-installed binaries without changing this design.)
- **Store the lockfile in the repo**: rejected — installed versions are
  per-machine state, not declarative config, and would cause spurious diffs
  across machines.
