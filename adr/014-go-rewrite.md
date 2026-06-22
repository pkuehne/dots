# ADR 014: Go Rewrite

## Status

Accepted (supersedes ADR 001, ADR 011, ADR 012)

> **Note:** The `.j2` templating plan described in the migration note below was never
> implemented and has since been abandoned — see [ADR 015](015-drop-templating.md).
> `.j2` files are now opaque and deploy verbatim.

## Context

dots grew from a single Python file (ADR 001) to a pip-installable package (ADR 011) with a Python 3.10+ minimum (ADR 012). The Python implementation worked well but carried inherent limitations:

- **Runtime dependency**: Users must have Python 3.10+ installed. On some systems (e.g., fresh Termux, minimal server images) this is a non-trivial setup step.
- **Distribution friction**: `pipx install` requires pipx; the bootstrap `install.sh` had to create a venv. Neither is as simple as "download one binary."
- **Cross-platform build**: Producing a single static binary for Linux/macOS/Termux from Python requires PyApp or similar tooling. From Go it is the default.
- **Developer experience**: The Python codebase accumulated vibe-code cruft that was easier to remove in a clean rewrite than to refactor incrementally.

## Decision

Rewrite dots in Go. The Go implementation lives under `cmd/` (CLI entry point) and `internal/` (packages). It is compiled to a single static binary with no runtime dependencies.

Key Go-specific decisions:

- **CLI**: [cobra](https://github.com/spf13/cobra) for command parsing and `--help` generation.
- **Config parsing**: [BurntSushi/toml](https://github.com/BurntSushi/toml) for TOML; no other mandatory third-party deps.
- **External tools**: `age` and `git` are invoked via `os/exec`, matching the Python approach.
- **GitHub API**: stdlib `net/http` + `encoding/json`; no SDK required.
- **Templates**: stdlib `text/template` for `.j2` files (syntax differs from Jinja2 — see migration note below).
- **Error model**: All user-facing errors are `*errs.DotsError` with a `Hint` field, so the output is always `error: ... \n  hint: ...` rather than a raw stack trace.

## Migration note for .j2 template users

The Python implementation rendered `.j2` files with Jinja2 (`{{ var }}`). The Go implementation uses Go's `text/template` (`{{ .var }}`). Existing `.j2` files using Jinja2 syntax need to be updated. Template variables come from `[vars]` in `dots.toml` and the platform context.

## Consequences

**Good:**
- Single static binary; `install.sh` downloads it from GitHub releases.
- No Python runtime required.
- `go build ./...` is reproducible; `go test ./...` is fast.
- Package-level tests with `t.TempDir()` replace pytest fixtures; no mocking framework needed.

**Neutral:**
- The `internal/tools` package is a known stub pending implementation (tracked in ROADMAP.md).
- Several CLI sub-commands (`shell show/clean`, `git init/show/uninit`, `ssh init/show/uninit`, `repos clone/update/status`, `tools check/install/list`, `presets show/eject`, `env show/check`) return a "not yet implemented" error until Phase 5/7 of the migration plan are completed.

**Trade-off:**
- `.j2` template syntax changed (Jinja2 → Go templates). Existing template users must update their files.
- Jinja2 features (filters, macros, `{% if %}` blocks) are not available; Go template equivalents must be used.

## Alternatives Considered

- **Keep Python, just improve packaging**: Would not eliminate the runtime dependency and would still require PyApp for binary distribution.
- **Rust**: Similar static-binary story but steeper learning curve; cobra/toml ecosystem is more mature in Go.
- **Shell script**: Too limited for TOML parsing, GitHub API, archive extraction, and idempotent file management.
