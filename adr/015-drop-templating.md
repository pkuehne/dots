# ADR 015: Drop templating — treat `.j2` as opaque files

## Context

The original Python implementation rendered `.j2` files with Jinja2: `files/.gitconfig.j2`
was rendered with variable substitution and written to `~/.gitconfig` (suffix stripped).
ADR 014 (Go rewrite) planned to preserve this via Go's `text/template`, but the renderer
was never implemented in the Go port. Instead the code special-cased `.j2`:

- discovery flagged `.j2` files (`Template = true`) and stripped the suffix from the destination;
- deploy refused to materialise them, returning `skipped (template — not supported)`;
- user shell snippets ending in `.j2` were likewise skipped with a visible message;
- a `[vars]` config section was documented but never parsed.

The net effect: a `.j2` file in the repo produced no output and a permanent "not supported"
warning on every `apply` — a call-out for a feature that did not exist. By contrast, any
other file type (an `.mp3`, a binary, a plain config) deploys verbatim without comment.
Singling out `.j2` only made sense while rendering was still on the roadmap.

## Decision

Drop templating entirely. `.j2` carries no special meaning; it is an opaque file like any
other.

- Remove the `.j2` branch from discovery — no `Template` flag, no suffix stripping. A
  `.j2` file deploys verbatim to `~/...j2`.
- Remove the template-skip branches from `deploy.Apply` / `deploy.Status`.
- Remove the `.j2` skip from user shell-snippet deployment (such files are copied
  verbatim; the bootstrapper only sources `[0-9]*.{sh,zsh,bash}`, so a `.j2` stays inert).
- Remove the `FileEntry.Template` field and its TOML key (`template`). Unknown keys are
  ignored by the parser, so existing configs that set `template = true` still load.
- Remove the undocumented-but-real gap: the `[vars]` section (never parsed) and its docs.

This supersedes the templating portions of ADR 003 and ADR 014.

## Consequences

- `apply` no longer emits a "not supported" line for `.j2` files; output reflects only
  real, supported actions.
- A user migrating from the Python version who still has `.j2` files will now get the
  literal template shipped (unrendered `{{ }}`), rather than nothing. The migration guide
  calls this out: render ahead of time, or use `.age` secrets / `files.d/{platform}/`
  scoping instead.
- The deploy/discovery/config surface shrinks: one fewer file field, two fewer skip paths.

## Alternatives Considered

- **Keep the call-out** (status quo): only defensible while rendering is genuinely planned.
  It is not — the feature was never built and there is no intent to add a Jinja2/`text/template`
  renderer.
- **Implement `text/template` rendering** as ADR 014 intended: real work, a new dependency
  on a variable-context model (`[vars]`, platform tokens), and a migration burden (Jinja2 →
  Go template syntax) for marginal benefit over platform scoping and pre-rendering.
