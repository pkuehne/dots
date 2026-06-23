# ADR 016: Link the age library instead of shelling out

## Status

Accepted (amends [ADR 008](008-age-for-secrets.md))

## Context

ADR 008 chose age for secret encryption, implemented by shelling out to the
`age` CLI via `exec.Command`. This contradicted the "one static binary, no
runtime dependencies" promise (ADR 014): users with `.age` files had to install
age separately, and `internal/secrets` carried three "install age" error hints
plus `exec.LookPath("age")` guards. Tests of the round-trip were skipped
whenever `age`/`age-keygen` were absent from `PATH`.

## Decision

Use the official Go library [`filippo.io/age`](https://pkg.go.dev/filippo.io/age)
directly. `internal/secrets` now calls `age.Encrypt`, `age.Decrypt`,
`age.ParseRecipients`, and `age.ParseIdentities` in-process. No external binary
is consulted.

Native (X25519) keys are the supported key type, matching age's own guidance for
integrating applications. Identity files keep the CLI-compatible format (one key
per line, `#`-comments ignored), so existing `age-keygen` key files continue to
work unchanged.

## Consequences

- No external dependency for secrets — fulfils the single-binary promise.
- The "install age" hints are gone; decryption errors now point at identity
  configuration instead. `dots doctor` checks that an identity is configured and
  present when `.age` files exist, rather than checking for the `age` binary.
- Round-trip tests generate keys in-process via `age.GenerateX25519Identity`, so
  they always run instead of skipping when age is absent.
- Adds `filippo.io/age` (and its transitive `golang.org/x/crypto`) to the
  dependency set — a deliberate, maintained addition per the dependency
  invariant in CLAUDE.md.

## Alternatives Considered

- Keep shelling out: rejected — defeats the single-binary goal.
- Vendor a minimal age implementation: rejected — the official library is small,
  maintained, and authoritative.
