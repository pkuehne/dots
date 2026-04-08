# ADR 002: TOML not YAML

## Context
dots needs a configuration format that is human-readable, unambiguous, and ideally available in Python's stdlib.

## Decision
Use TOML with `.toml` extension. `tomllib` is in stdlib since Python 3.11; `tomli` is a zero-dep fallback for older versions.

## Consequences
- No surprise type coercions (no Norway problem, no `yes`/`on` → bool)
- Whitespace-insensitive
- Unambiguous types
- Trade-off: `[[array of tables]]` syntax is unfamiliar to some users; inline tables must be single-line

## Alternatives Considered
- YAML: ambiguous types, requires PyYAML dependency, indentation-sensitive
- JSON: no comments, verbose
- INI: too limited for nested config
