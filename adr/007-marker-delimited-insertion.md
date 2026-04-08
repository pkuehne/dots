# ADR 007: Marker-Delimited Insertion

## Context
dots needs to inject config blocks into existing files (.zshrc, .gitconfig, .ssh/config) without replacing the whole file.

## Decision
Use `# >>> dots managed >>>` / `# <<< dots managed <<<` markers. Content between markers is owned by dots and can be updated in place. Content outside markers is never modified.

## Consequences
- Safe for files with existing manual config
- Idempotent: re-running updates the block without duplication
- `uninit` removes only the marked block
- Trade-off: markers are visible in the file; acceptable given they're clearly labelled

## Alternatives Considered
- Replace entire file: destructive, loses user customizations
- Separate include file only: some tools (SSH) need Include in the main file anyway
- Comment-based tracking without markers: fragile, hard to update
