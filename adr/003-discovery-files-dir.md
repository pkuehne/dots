# ADR 003: Discovery from files/ Directory

## Context
Most dotfiles are simple files that should be symlinked to `~` with no configuration needed.

## Decision
`files/` mirrors `~`. Any file placed there is automatically discovered and symlinked. `.j2` files are rendered, `.age` files are decrypted. No TOML entry required.

## Consequences
- Zero-config experience for the common case
- Trade-off: implicit behaviour may surprise; mitigated by `dots status` showing all discovered files
- Explicit `[[file]]` entries can override any discovered file's deployment config

## Alternatives Considered
- Require every file in dots.toml: verbose, discourages adoption
- Convention-free with only explicit entries: loses the zero-config level
