# ADR 010: Profile Auto-Activation

## Context
Platform-specific and machine-specific config is extremely common. Requiring `--profile` every time is tedious.

## Decision
Profiles matching the current platform name (linux, darwin, termux) or hostname are auto-activated. Manual profiles via `--profile` have highest priority.

Layering order: global < platform < hostname < manual.

## Consequences
- Eliminates `--profile` for the common cases
- Trade-off: hostname matches may be unexpected; documented clearly
- Profile overrides replace (not extend) leaf values; lists are replaced entirely

## Alternatives Considered
- Only manual profiles: too much friction for common cases
- Environment variable detection: less predictable than platform/hostname
