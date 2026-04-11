# Configuration Reference

See the spec for the full annotated `dots.toml` schema. This document provides a quick reference.

## Sections

| Section | Purpose |
|---------|---------|
| `[meta]` | Schema version, default deploy mode |
| `[vars]` | Template variables (available in .j2 files) |
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
