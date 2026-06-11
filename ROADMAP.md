# Roadmap

Planned improvements for dots, roughly in priority order.
Contributions welcome — open an issue to discuss before starting larger items.

## Near-term

- [ ] Add LICENSE file (MIT)
- [ ] Add CHANGELOG.md skeleton (release-please will populate it on first release)
- [ ] Implement `internal/tools` — GitHub release download, apt/brew/script install methods
- [ ] Wire subsystem commands (shell/git/ssh/tools/repos/presets) into `apply` orchestration
- [ ] `--quiet` / `--verbose` flags for scripting and debugging
- [ ] Shell completions (`dots completion bash/zsh/fish` via cobra)
- [ ] Post-install hooks for tools (e.g. rebuild bat cache, run plugin install)
- [ ] `files` list in `ToolInstall` — install multiple binaries from one archive (e.g. `cmake` + `cpack` + `ctest`), inspired by the [aqua registry](https://github.com/aquaproj/aqua-registry) `files[].src` pattern
- [ ] Explicit shell selection — `[shell] shells = ["zsh"]` to opt in to specific shells rather than managing all detected shells; users wanting an unmanaged bash fallback should not have their `.bashrc` modified
- [ ] Native `chsh` support (manage default shell)
- [ ] `dots export <dir>` — copy all rendered/managed files into a plain directory (no symlinks, no dots dependency)
- [ ] `dots remove` — unlink a managed file and restore original from backup
- [ ] `dots update` — pull dotfiles repo and re-apply in one shot (useful for cron/login)
- [ ] `dots self-update` — upgrade dots itself (detect install method: direct binary, package manager)
- [ ] Add `is_wsl` to platform detection and template context

## Later

- [ ] `dots apply` should pull updates to already-cloned repos (currently only clones missing ones; `dots repos update` is the workaround)
- [ ] `dots apply` should run `shell clean` to prune stale tool snippets when tools are removed from `dots.toml`
- [ ] Golden repo fixture for realistic smoke testing
- [ ] Man page generation (cobra supports `docs.GenManTree`)
- [ ] Coloured/syntax-highlighted diff output in `dots diff`
- [ ] `dots log` — append-only log of what `apply` changed
- [ ] XDG base directory support — respect `$XDG_CONFIG_HOME` for dots' own config
- [ ] More git config options
