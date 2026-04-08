# ADR 006: GitHub Releases First

## Context
Tools like ripgrep, bat, and fzf are available through many channels. Version freshness matters.

## Decision
Install methods are tried in declaration order. The recommended order is: pkg (Termux) → github → apt → brew → cargo. GitHub releases are always the latest version; apt is often 1-2 major versions behind.

## Consequences
- Users get the latest versions by default
- Trade-off: GitHub downloads may be blocked by proxies; apt/brew fallback handles this
- GITHUB_TOKEN hint handles rate limits

## Alternatives Considered
- System package manager first: often outdated
- Always compile from source: slow, requires build tools
