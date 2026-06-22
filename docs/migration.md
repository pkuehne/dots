# Migration Guide

## From the Python version of dots

The Go rewrite is config-compatible — `dots.toml`, `files/`, `files.d/`, and
`shell/` work unchanged. Two behavioral differences:

1. The custom snippet generated from `files/.zshrc` is now `099-custom.sh`
   (sourced last) instead of `000-custom.sh` (sourced first). Run
   `dots apply` then `dots shell clean` to regenerate and remove the
   Python-era `000-custom.sh`.
2. Templating is not supported. A `.j2` file is no longer special-cased — it
   deploys verbatim (suffix and all), so unrendered `{{ }}` placeholders would
   ship as-is. Render templates ahead of time, or use `.age` secrets or
   platform scoping (`files.d/`) instead.

## From manual dotfiles (no tool)

1. `dots init ~/dotfiles`
2. Copy your dotfiles into `files/`: `cp ~/.zshrc ~/dotfiles/files/`
3. `cd ~/dotfiles && dots apply --dry-run` to preview
4. `dots apply` to deploy symlinks
5. `git init && git add -A && git commit -m "initial dots setup"`

Or use the automated scanner: `dots migrate --write`

## From chezmoi

1. Create `files/` and copy your chezmoi source files (strip `dot_` prefixes, `private_` prefixes)
2. Templates are not supported — replace `.tmpl` files with plain files, platform-scoped variants in `files.d/{platform}/`, or `.age` secrets
3. Move platform-specific files to `files.d/{platform}/`
4. Create `dots.toml` with `[env]`, `[shell]`, etc. as needed

## From yadm

1. yadm stores files at their real paths. Copy them to `files/` preserving relative structure
2. yadm alternates → `files.d/{platform}/`
3. yadm encrypt → `.age` files with `[secrets]` config

## From bare git repo

1. Your files are already at their real paths. Use `dots add` to adopt each one:
   ```
   dots add ~/.zshrc
   dots add ~/.gitconfig
   ```
2. This copies them to `files/` and adds `[[file]]` entries to `dots.toml`
