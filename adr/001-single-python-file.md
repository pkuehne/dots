# ADR 001: Single Python File

## Context
dots needs to be easy to bootstrap on any system — including bare servers, Termux, and fresh macOS installs.

## Decision
dots is implemented as a single Python file with no build step. It can be copied into a dotfiles repo and run directly with `python3 dots`.

## Consequences
- Portability: `curl | python3` works, no install step needed
- Fits naturally in a dotfiles repo
- No build system or packaging required
- Trade-off: harder to navigate a large file; mitigated by section comments and clear function grouping

## Alternatives Considered
- Python package with setup.py/pyproject.toml: requires pip install step
- Shell script: too limited for TOML parsing, template rendering, GitHub API
- Go/Rust binary: requires compilation, harder to embed in dotfiles repo
