# ADR 004: files.d/{platform}/ for Platform Scoping

## Context
Some files should only be deployed on specific platforms (e.g., systemd configs on Linux, aerospace.toml on macOS).

## Decision
`files.d/{platform}/` directories mirror `~` but are only deployed on the matching platform. Platform is auto-detected.

## Consequences
- Per-platform files with no TOML entry needed
- Clearer than `only = [...]` on every `[[file]]`
- Trade-off: less visible than explicit entries; mitigated by `dots status` and docs

## Alternatives Considered
- Only use `only` guards on `[[file]]` entries: requires TOML for every platform-specific file
- Filename conventions (e.g., `.linux.zshrc`): fragile, doesn't work for all file types
