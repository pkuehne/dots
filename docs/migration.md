# Migration Guide

## From manual dotfiles (no tool)

1. `dots init ~/dotfiles`
2. Copy your dotfiles into `files/`: `cp ~/.zshrc ~/dotfiles/files/`
3. `cd ~/dotfiles && dots apply --dry-run` to preview
4. `dots apply` to deploy symlinks
5. `git init && git add -A && git commit -m "initial dots setup"`

Or use the automated scanner: `dots migrate --write`

## From chezmoi

1. Create `files/` and copy your chezmoi source files (strip `dot_` prefixes, `private_` prefixes)
2. Rename `.tmpl` → `.j2` and update template syntax (chezmoi uses Go templates, dots uses Jinja2)
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
