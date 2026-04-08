# Architecture

## Overview

dots is a single-file Python tool for dotfile management, tool installation, and shell environment generation. It works identically on Linux, macOS, and Termux.

## Configuration Levels

### Level 0 — Zero Config: Mirror Mode
No `dots.toml` required. `dots apply` discovers all files under `files/` and `files.d/{platform}/`, strips the directory prefix, and symlinks into `~`.

Auto-detection rules (first match wins):
- `.age` suffix → secret; decrypt with age, write result
- `.j2` suffix → template; render with Jinja2, write result
- Everything else → symlink

### Level 1 — dots.toml: Additive Overrides
Discovery still runs for all files not explicitly listed. `[[file]]` entries override or extend: destination path, `only`/`profile` guards, permissions, copy vs symlink.

### Level 2 — Managed Subsystems (opt-in per subsystem)
Dots takes ownership of shell init, git config, or SSH config via `managed = true`. Each subsystem is independently opted into.

### Level 3 — Presets (opt-in opinionated defaults)
Named bundles of generated config. Always off by default. Ejectable to plain files.

## Discovery and Explicit Entry Merge Strategy

1. Walk `files/` recursively → build FileEntry list with `dst = ~/relative_path`
2. Walk `files.d/{platform}/` → add with `only = [platform]`
3. For each explicit `[[file]]`: if `src` matches a discovered entry, override it; otherwise append

## Profile Layering

Order (lowest → highest priority): global → platform profile → hostname profile → manual profile.

`deep_merge` replaces leaf values and lists; it does not concatenate.

## Managed Subsystem Lifecycle

```
init → apply → show → uninit
```

- **init**: Generate config file + insert marker-delimited include
- **apply**: Regenerate config, update include
- **show**: Print what would be generated (read-only)
- **uninit**: Remove marker-delimited include, leave generated file

## Generated File Map

```
~/.config/dots/
  shell.d/
    000-custom.sh         content of files/.zshrc (migration aid)
    010-env.sh            from [env] + [[env.when]]
    020-path.sh           from [shell] path + tool paths
    050-{name}.sh         per-tool shell integration
    {user snippets}       from shell/ directory
  git/
    managed.gitconfig     from [git] + tool contributions
  ssh/
    config                from [[ssh.host]]
  key.txt                 age identity (user-provided)
```

## Execution Order within `dots apply`

1. Deploy files/ and files.d/{platform}/ (discovery + explicit [[file]])
2. Generate and write 010-env.sh, 020-path.sh (if shell.managed)
3. Deploy user snippets from shell/ (if shell.managed)
4. Generate per-tool snippets (if shell.managed)
5. Generate 000-custom.sh from files/.zshrc (if shell.managed + file exists)
6. Write managed.gitconfig (if git.managed)
7. Write managed SSH config (if ssh.managed)
8. Clone missing repos

## Tool Contributions to Multiple Subsystems

A `[[tool]]` entry can contribute to:
- **shell**: `shell.env` → env vars in 050-{name}.sh; `shell.init` → eval commands; `shell.path` → PATH additions
- **git**: `git.pager`, `git.diff` → entries in managed.gitconfig

## Snippet Ordering

| Range   | Owner | Source |
|---------|-------|--------|
| 000     | dots  | files/.zshrc content (migration aid) |
| 010     | dots  | [env] + [[env.when]] |
| 020     | dots  | [shell] path + tool paths |
| 030–049 | user  | hand-written snippets |
| 050–079 | dots  | per-tool generated snippets |
| 080–089 | user  | post-tool hand-written snippets |
| 090+    | user  | completions, zsh-only |

## Idempotency Strategy

- **Content comparison**: sha256 hash before overwriting copied files
- **Symlink check**: resolve() comparison before recreating
- **Marker-delimited insertion**: regex replace between markers for updates
- **Backup-before-replace**: `.dots-bak` suffix on any overwritten file
