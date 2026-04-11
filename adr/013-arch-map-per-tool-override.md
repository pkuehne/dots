# ADR 013: arch_map for per-tool architecture name overrides

## Context

GitHub release assets use inconsistent architecture naming across tools. Some tools
follow Linux kernel naming (`x86_64`, `aarch64`); others follow Go's naming
(`amd64`, `arm64`); and some mix conventions within a single project — using
`x86_64` for Intel but `arm64` for ARM.

dots provides two template tokens:

- `{arch}` — Linux naming: `x86_64`, `aarch64`
- `{goarch}` — Go naming: `amd64`, `arm64`

Neither covers tools that mix the two. For example, lazygit and neovim both publish
`linux_x86_64` for Intel and `linux_arm64` for ARM. On an aarch64 host, `{arch}`
produces `aarch64` (no match) and `{goarch}` produces `arm64` (no match for x86_64).

## Decision

Add an optional `arch_map` field to `ToolInstall`. Before `{arch}` is substituted
into the asset pattern, the detected arch is looked up in the map. If a match is
found the mapped value is used; otherwise the detected arch passes through unchanged.

`{goarch}` is unaffected — it always returns the canonical Go name.

```toml
[[tools.lazygit.install]]
method = "github"
repo = "jesseduffield/lazygit"
asset = "lazygit_{version}_Linux_{arch}.tar.gz"
arch_map = { aarch64 = "arm64" }
```

## Consequences

- Users can express mixed-convention tools without workarounds or duplicate entries.
- The common case (no `arch_map`) is unaffected — the field defaults to an empty dict.
- `arch_map` is intentionally narrow: it only remaps `{arch}`, not `{goarch}` or
  other tokens. This keeps the mental model simple.

## Alternatives Considered

- **New per-tool token (e.g. `{lazygit_arch}`)**: requires hard-coding tool-specific
  knowledge in dots itself. `arch_map` is data-driven instead.
- **Duplicate install entries with `only`**: users could write one entry filtered to
  `linux` with `{arch}` and another to `termux`/`darwin` with `{goarch}`. Verbose
  and fragile if the tool later changes naming conventions.
- **Glob fallback (wildcard arch)**: using `*` in the asset pattern works but is
  ambiguous when multiple architectures are present in the release.
