# ADR 008: age for Secrets

> **Update:** The original decision shelled out to the `age` CLI. dots now links
> the official [`filippo.io/age`](https://pkg.go.dev/filippo.io/age) library
> directly into the binary, so no external `age` binary is required — see
> [ADR 016](016-age-library-not-binary.md).

## Context
Dotfiles repos often contain secrets (SSH keys, API tokens). These need to be encrypted at rest in the repo.

## Decision
Use age for encryption. Simple CLI, no keyring dependency, works on Termux. `.age` files are auto-detected and decrypted during deploy.

## Consequences
- Single external binary dependency (only when secrets are used)
- `dots tools install age` handles bootstrapping
- Trade-off: requires age binary; graceful error if absent

## Alternatives Considered
- GPG: complex key management, harder on Termux
- SOPS: heavier dependency, more complex
- git-crypt: tied to git, doesn't work with plain file management
