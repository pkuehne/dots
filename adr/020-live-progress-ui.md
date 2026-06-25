# ADR 020: Concurrent live progress for tools/repos

## Status

Accepted

## Context

After version tracking (ADR 019) added `tools update`, `tools install`, and a
ref-aware `repos update`, the UX of those commands was silent. Each did all of
its network and install work in one synchronous loop and printed nothing until
the whole batch succeeded or failed, then emitted a single result line. For a
`latest` tool that meant a GitHub API round-trip plus a multi-megabyte download
with no feedback, and the work ran serially even though it is independent and
almost entirely network-bound per tool/repo.

The ask ([issue #25 follow-up]) was a docker-style experience: one live row per
unit of work advancing through its stages, with the independent items running
concurrently — while keeping output clean when not attached to a terminal and
keeping `--dry-run` side-effect-free.

## Decision

Add a live-progress layer and parallelise the per-item work.

**Renderer (`internal/ui/progress.go`).** A small `Progress`/`Task` interface
pair the work packages depend on, with three implementations chosen by
`NewProgress(dryRun)`:

- a terminal renderer backed by [`github.com/vbauerster/mpb/v8`] — a stack of
  bars, one per task, each showing its stage (resolving → downloading →
  installing) and, for downloads, a byte bar;
- a plain line logger for non-terminals (pipes, CI) so captured output stays
  ANSI-free;
- a no-op for piped dry-run, where the predicted-action table is the whole
  output.

On a terminal, dry-run still uses the bar renderer but in **transient** mode:
resolving a `latest` target is a read-only GitHub round-trip, so the resolve
phase is shown live (one short row per tool) and each row clears on completion,
leaving no residue before the command prints its predicted-action table. Only
the writes (download, install, lockfile) are suppressed in dry-run, never the
read-only resolve feedback.

`Task` implements `io.Writer` so it doubles as the download byte sink. Work
packages never import mpb directly.

**Download progress (`internal/ghrelease`).** `DownloadAsset` takes an optional
`DownloadSink` (`SetTotal(int64)` + `io.Writer`); a nil sink preserves the old
behaviour. `ui.Task` satisfies it structurally.

**Concurrency.** `tools.Update`, `tools.InstallAll` (new, shared by the install
command and `apply`), and `repos.Update` each run a bounded worker pool
(buffered-channel semaphore + `sync.WaitGroup`; default 4, overridable with
`-j/--jobs`). Results are collected into a position-indexed slice so output
stays in config order regardless of completion order. The lockfile gained a
mutex so a parallel batch of github installs can record versions into one shared
`Lock`.

## Consequences

- New dependency `vbauerster/mpb/v8` (and its small transitive deps:
  go-runewidth, uniseg, ewma, stripansi). Chosen over hand-rolling terminal
  cursor control — it is the purpose-built, concurrency-safe library for this,
  consistent with the project's "real maintained packages, added with intent"
  stance. `mattn/go-isatty` (already an indirect dep of `fatih/color`) is now
  direct, used for TTY detection.
- `tools.Update` and `repos.Update` changed signature to take a `ui.Progress`
  and a `jobs` count; `repos.Update` now returns `[]UpdateResult` and the
  command prints them (matching the existing `repos.Clone` pattern). The old
  `repos.updateOne` was folded into `updateRepo`.
- `-j/--jobs` is available on `tools update`, `tools install`, `repos update`,
  and `apply` (default 4).
- Dry-run performs zero side effects but, on a terminal, shows the read-only
  resolve phase as transient rows that clear before the predicted-action table
  (piped dry-run stays table-only). Resolve/download/install failures abort that
  item's row (or log a failure line) while the rest of the batch continues; the
  first error is returned so its hint still surfaces, and the predicted-action
  table reports each failure rather than claiming everything is up to date.
- `dots upgrade` (selfupdate) passes a nil sink for now — a progress bar there is
  a possible follow-up.

## Alternatives Considered

- **Hand-rolled ANSI renderer in `internal/ui`**: rejected — correct
  concurrent multi-line rendering (repaint, width sync, resize, abort) is
  exactly what mpb already solves; reimplementing it is error-prone for no gain.
- **`golang.org/x/sync/errgroup` for the pools**: a channel semaphore plus
  `WaitGroup` is a few lines and avoids another dependency, so errgroup was not
  added.
