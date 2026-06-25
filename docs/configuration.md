# Configuration Reference

See the spec for the full annotated `dots.toml` schema. This document provides a quick reference.

## Sections

| Section | Purpose |
|---------|---------|
| `[meta]` | Schema version, default deploy mode |
| `[profiles.*]` | Per-platform/hostname/manual overrides |
| `[env]` | Shell environment variables |
| `[[env.when]]` | Conditional env vars (platform/tool guards) |
| `[shell]` | Shell managed mode, PATH, snippet dir |
| `[git]` | Git managed mode, user info, editor |
| `[ssh]` | SSH managed mode |
| `[[ssh.host]]` | SSH host entries |
| `[tools]` | Tool bin directory |
| `[[tool]]` | Tool definitions with install methods |
| `[[file]]` | Explicit file deployment entries |
| `[[repo]]` | Git repositories to clone |
| `[secrets]` | Age encryption config |
| `[presets]` | Opinionated preset bundles |

### Block vs. inline form

Any array-of-tables section accepts both TOML spellings interchangeably — use
whichever reads better:

```toml
# Block form
[[tool.install]]
method = "github"
repo = "jesseduffield/lazydocker"

# Inline form (identical meaning)
install = [
  { method = "github", repo = "jesseduffield/lazydocker" },
]
```

Unknown keys and structurally wrong values are rejected at load time with a
hint, rather than being silently ignored.

## Install Methods

| Method | Binary | Notes |
|--------|--------|-------|
| `pkg` | `pkg` | Termux only |
| `apt` | `apt-get` | Uses sudo if not root |
| `brew` | `brew` | macOS / Linux brew |
| `cargo` | `cargo` | Rust crate |
| `go` | `go` | Appends @latest |
| `pip` | `pip3` | Uses --user |
| `pipx` | `pipx` | Isolated Python apps |
| `npm` | `npm` | Global install |
| `github` | none | Download from GitHub releases |
| `script` | none | Raw shell command |
| `manual` | none | Print note, skip |

The `github` method installs the latest release by default. Pin a release with
`version = "1.2.3"` — the tag is matched as given, falling back to a `v` prefix
(`v1.2.3`). Setting `version = "latest"` is the same as leaving it unset: dots
tracks the newest release. See [Version tracking](#version-tracking) for how dots
asserts and updates the installed version.

When the matched asset is an archive, dots extracts it and locates the binary
(see `binary` / `binary_path`). Supported archive formats are `.tar.gz`/`.tgz`,
`.tar.bz2`, `.tar.xz`, `.tar.zst`, `.tar` and `.zip`. Any other asset is
installed verbatim. Path-traversal and out-of-archive symlink entries are
rejected as a safety measure.

## Asset Pattern Tokens

| Token | Values |
|-------|--------|
| `{version}` | Tag without v prefix |
| `{arch}` | x86_64, aarch64, armv7, i686 |
| `{os}` | linux, darwin, windows |
| `{goarch}` | amd64, arm64, 386 |
| `{name}` | Tool name from config |

### Handling mixed arch naming with `arch_map`

Some tools mix naming conventions across architectures — for example, using `x86_64`
(Linux naming) for Intel but `arm64` (Go naming) for ARM. Neither `{arch}` nor
`{goarch}` covers both cases.

Use `arch_map` to remap the detected architecture before it is substituted into the
asset pattern. Only architectures listed in the map are remapped; others pass through
unchanged.

```toml
[[tools.lazygit.install]]
method = "github"
repo = "jesseduffield/lazygit"
asset = "lazygit_{version}_Linux_{arch}.tar.gz"
arch_map = { aarch64 = "arm64" }
# x86_64 → x86_64  (no entry, unchanged) → lazygit_1.0_Linux_x86_64.tar.gz
# aarch64 → arm64  (remapped)            → lazygit_1.0_Linux_arm64.tar.gz
```

`arch_map` applies only to `{arch}`; `{goarch}` is always the canonical Go name.

## Version tracking

For tools installed with the `github` method, dots tracks which version is
installed and can assert a target version — either a pinned tag or the latest
release.

The version dots installs is recorded in a machine-local lockfile at
`~/.config/dots/installed.toml`. This file describes what is installed on *this*
machine, so it is not part of your dotfiles repo and should not be committed.
Package-manager installs (apt, brew, cargo, …) are not recorded — those managers
track their own versions.

```toml
[[tool]]
name = "lazygit"
[[tool.install]]
method = "github"
repo = "jesseduffield/lazygit"
version = "0.44.1"   # pinned: dots asserts exactly this release
# version = "latest" # or omit: dots tracks the newest release
```

| Command | What it does |
|---------|--------------|
| `dots tools status [names...]` | Show installed vs target version for each tracked tool. |
| `dots tools update [names...]` | Reinstall tools whose installed version differs from the target. |

`dots tools status` reports one of: up to date (`✓`), outdated (`↑ old → new`),
not installed (`✗`), or untracked (`·`, no github method). Resolving a `latest`
target queries the GitHub API; pinned targets are read straight from config.

`dots tools update` reinstalls only the tools that are outdated (or missing),
then records the new version in the lockfile. It is idempotent: a second run
reports everything up to date. Use `--dry-run` to preview and `--tag` to filter.
Tools installed by a package manager are reported as `untracked` and left alone.

Updates and installs run concurrently with a live, docker-style progress
display — one row per tool advancing through resolving → downloading →
installing. `-j/--jobs` sets how many run at once (default 4). The display is
shown only on a terminal; piped output degrades to plain lines and `--dry-run`
prints the predicted actions with no progress bars. `dots tools install` and the
install phase of `dots apply` share the same display and `-j` flag.

## Repositories — `[[repo]]`

Each `[[repo]]` clones a git repository to `dst`.

| Key | Meaning |
|-----|---------|
| `name` | Identifier used on the command line. |
| `repo` | URL or `user/repo` GitHub shorthand. |
| `dst` | Clone destination (supports `~`). |
| `shallow` | Clone with `--depth 1`. |
| `ref` | Target ref to assert — a tag, branch, or SHA. |
| `on_install` / `on_update` | Shell hooks run after clone / update. |

Like tools, repos track a version through `ref`:

```toml
[[repo]]
name = "tpm"
repo = "tmux-plugins/tpm"
dst  = "~/.tmux/plugins/tpm"
ref  = "v3.1.0"   # pinned: update asserts this tag
# ref = "latest"  # or omit: track the default branch tip
```

`dots repos update` brings each clone in line with its `ref`. A pinned tag or
SHA is checked out exactly (detached); a pinned branch is reset to its remote
tip so a moved branch is honoured. With `ref` unset or `"latest"`, dots tracks
the default branch as before. A repo with uncommitted changes is skipped so
local work is never discarded. Repos update concurrently with a live progress
display (`-j/--jobs`, default 4), the same as tools.

`dots repos status` shows `≠ ref <x>` when HEAD has drifted off a pinned ref,
alongside the existing `missing`, `dirty`, and `behind N` states.
