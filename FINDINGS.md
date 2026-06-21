# Consistency review findings — 2026-06-11

Pass over the full Go source looking for things that work as coded but cut
against the tool's invariants (idempotency, visible dry-runs, no silent
config no-ops, never destroy user data). Checkboxes track fix progress.

## Config accepted but silently ignored

- [x] **F1. `[[repo]]` ignores `only` and `profile`.** Both fields parse
  (`internal/config/config.go:117`) but `repos.Filter` only filters by name.
  A repo scoped `only = ["darwin"]` is cloned on Linux. Every other subsystem
  honors `only`.
- [x] **F2. `Tool.Profile` is parsed and never checked.** `tools.Filter`
  handles `only` and tags but not profile, so profile-scoped tools install on
  every profile.
- [x] **F3. `ToolInstall.Version` is parsed and ignored** — `installGitHub`
  always fetches the latest release. Accepting `version = "1.2.3"` and doing
  something else is worse than rejecting it. Fix: honor pinned versions via
  the releases/tags API.
- [x] **F4. Dead fields.** `Tool.Desc` is never displayed (`tools list`
  should show it); `CheckResult.Version` is never populated (remove);
  `meta.version` is never validated (a `version = 99` repo loads silently).
  `[vars]` is stored but unused — deferred, may return with templating.

## Dry-run / output visibility

- [x] **F5. Half of `dots apply --dry-run` is silent.** `shell.WriteSnippets`,
  `git.WriteManaged`, `ssh.WriteManaged` produce zero output in dry-run mode
  (and rewrite identical content unconditionally in real mode). They should
  detect "unchanged" and print what they would write/wrote.
- [x] **F6. `repos.Clone`/`Update` print nothing, ever.** `cloneOne`/`updateOne`
  return status strings that every caller discards. A successful clone is
  completely silent, in dry-run and real mode alike.
- [x] **F7. `shell clean` is silent** — dry-run prints nothing, and real mode
  removes files without saying which.
- [x] **F8. `dots preview` over-reports.** `deploy.Apply` short-circuits on
  DryRun before looking at the destination, so preview says "would link" for
  files apply would report "unchanged". Preview and apply disagree on a clean
  system.

## Status vs apply disagreements

- [x] **F9. Asymmetric drift detection in `deploy.Status`.** A symlink where
  the entry wants a copy reports "diff", but a regular file with identical
  content where the entry wants a symlink reports "unchanged" — yet apply
  would relink it.
- [x] **F10. Secrets are invisible in `status`** (reported "skipped", which
  runStatus hides) even though apply deploys them. Also template-skips are
  excluded from the "N skipped" summary count.

## Hazards

- [ ] **F11. `099-custom.sh` recursion / repo-mutation trap.**
  `GenerateCustomSnippet` wraps `files/.zshrc` verbatim. Discovery also
  deploys `files/.zshrc` as a symlink to `~/.zshrc`, and `InsertSourceLine`
  writes the bootstrapper through that symlink into the repo file; the next
  apply wraps the marker block (including the snippet-sourcing loop) into
  `099-custom.sh`, which the loop itself sources → infinite recursion.
  Fix: strip the managed marker block when generating the custom snippet.
- [ ] **F12. `repos update` on a shallow repo runs `git reset --hard`**
  without checking the dirty state `repoState` already knows how to detect —
  local modifications destroyed silently. Fix: skip dirty repos with a
  warning.
- [ ] **F13. Invariant 4 is false as written.** "No operation modifies
  anything outside ~" — but apt/brew/pkg install methods (sudo auto-prepended
  for apt) write to system locations by design. Scope the invariant to file
  deployment.

## Smaller consistency nits

- [ ] **F14. fzf preset is zsh-only but sourced by bash.** `applyPresets`
  writes `030-fzf.sh` (sourced by both bootstrappers) with hardcoded
  `GenerateFzf("zsh")`. Fix: write per-shell `030-fzf.zsh`/`030-fzf.bash`
  like tool snippets, keep `shell.Clean` expectations in sync.
- [ ] **F15. Preset idempotency reporting differs**: tmux reports
  "unchanged"; fzf unconditionally rewrites and prints "wrote" every apply.
- [ ] **F16. `dots apply <typo>` succeeds silently** with "0 linked…"
  instead of "no files matched"; `tools check/install <unknown>` silently
  drops unknown names.
- [ ] **F17. `doctor` hardcodes `~/.local/bin`** for the PATH check instead
  of `cfg.ToolsConfig.BinDir`.
- [ ] **F18. Hygiene.** `gofmt -l` flags 7 files; ROADMAP.md still lists
  LICENSE, CHANGELOG, completions, Phase-7 apply wiring, and WSL detection
  as unchecked though all are shipped.
