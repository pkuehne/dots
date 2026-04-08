# ADR 012: Minimum Python Version

## Status
Accepted (supersedes the implicit 3.8 floor)

## Context
The initial implementation targeted Python 3.8 as the minimum version. This was overly conservative — 3.8 reached end-of-life in October 2024 and 3.9 in October 2025. Targeting EOL interpreters means carrying compatibility workarounds (e.g., `typing.List` instead of `list`, no `match` statements) while those Python versions no longer receive security patches.

dots targets Linux, macOS, and Termux. The relevant system Python versions as of 2026:

| System | Python version |
|--------|---------------|
| Ubuntu 22.04 LTS (supported until 2027) | 3.10 |
| Ubuntu 24.04 LTS | 3.12 |
| Debian 12 (bookworm) | 3.11 |
| RHEL 9 / Rocky 9 | 3.9 (EOL, but RHEL backports security fixes) |
| macOS (Homebrew) | latest (3.12+) |
| Termux | 3.12+ |

## Decision
Set `requires-python = ">=3.10"` as the minimum supported version.

3.10 is the oldest CPython release still receiving upstream security fixes (until October 2026) and ships on the oldest actively-supported Ubuntu LTS (22.04). Users on older systems can install a newer Python via pyenv, Homebrew, or deadsnakes PPA.

## Consequences
- Can use modern type syntax: `list[str]`, `dict[str, Any]`, `X | None` instead of `typing.List`, `typing.Dict`, `typing.Optional`
- `from __future__ import annotations` is kept for consistency and to avoid runtime evaluation of annotations
- `tomli` backport is still needed for Python 3.10 (`tomllib` was added in 3.11)
- RHEL 9 users on stock Python 3.9 will need to install a newer Python — this is standard practice for CLI tools installed via pipx
- The minimum version should be reviewed annually and bumped as older versions reach EOL
