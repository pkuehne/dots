package ui

// Live progress rendering for the network-bound, parallelisable commands
// (`tools update`, `tools install`, `repos update`, and the install phase of
// `apply`). On a terminal this is a docker-style stack of bars — one row per
// unit of work, each advancing through its stages (resolving → downloading →
// installing) concurrently. Off a terminal (pipes, CI) it degrades to plain,
// ANSI-free line logging so captured output stays clean. Dry-run keeps the
// read-only resolve feedback: on a terminal the bars are transient (each row
// clears on completion) so nothing lingers before the command prints its
// predicted-action list; off a terminal dry-run renders nothing (the list is
// enough).
//
// Work packages depend only on the small Progress/Task interfaces below, never
// on mpb directly, so the renderer stays swappable and the packages stay
// UI-agnostic.

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

// Task is one live row of work. A worker drives a single Task from creation to
// exactly one terminal call (Done or Fail); leaving a Task unterminated would
// block Progress.Wait. Task implements io.Writer so it can be handed to a
// download as a byte sink — each Write advances the bar by len(p).
type Task interface {
	io.Writer
	// Stage sets the short status shown while the work is in flight
	// (e.g. "resolving", "downloading", "installing").
	Stage(msg string)
	// SetTotal declares the work size once known so the bar can fill: a byte
	// count for a download, or a step count for a discrete multi-step op. A
	// non-positive total leaves the bar indeterminate.
	SetTotal(total int64)
	// Advance moves the bar forward by n units of a step-counted task (its total
	// set via SetTotal). It is the discrete-step counterpart to Write, which
	// advances by a number of transferred bytes.
	Advance(n int64)
	// Done marks the work finished, showing detail (e.g. "14.1.0 → 14.1.1").
	Done(detail string)
	// Fail marks the work failed, showing err.
	Fail(err error)
}

// Progress is a container of live Tasks. Wait blocks until every Task created
// from it has terminated, leaving the finished rows on screen.
type Progress interface {
	Task(name string) Task
	Wait()
}

// NewProgress picks the renderer for the current context. On a terminal it is a
// live mpb container — and in dry-run the bars are transient (each row clears on
// completion) so the read-only resolve phase shows live without leaving residue
// before the command prints its predicted-action table. Off a terminal it is
// plain line logging, except dry-run which stays silent (the table is enough).
func NewProgress(dryRun bool) Progress {
	tty := isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
	switch {
	case tty:
		return newBarProgress(dryRun)
	case dryRun:
		return discardProgress{}
	default:
		return &lineProgress{}
	}
}

// ── mpb-backed live bars ───────────────────────────────────────────────────────

type barProgress struct {
	p         *mpb.Progress
	transient bool // remove each bar once it completes (dry-run resolve phase)
}

func newBarProgress(transient bool) *barProgress {
	return &barProgress{
		transient: transient,
		p: mpb.New(
			mpb.WithWidth(60),
			mpb.WithRefreshRate(120*time.Millisecond),
			mpb.WithAutoRefresh(),
		),
	}
}

func (b *barProgress) Wait() { b.p.Wait() }

func (b *barProgress) Task(name string) Task {
	t := &barTask{stage: "queued"}
	opts := []mpb.BarOption{
		mpb.BarWidth(24),
		mpb.PrependDecorators(
			decor.Name(name, decor.WC{C: decor.DSyncSpaceR}),
			decor.Any(t.statusText, decor.WC{C: decor.DSyncSpaceR}),
		),
		mpb.AppendDecorators(
			decor.Any(t.rightText),
		),
	}
	if b.transient {
		opts = append(opts, mpb.BarRemoveOnComplete())
	}
	t.bar = b.p.New(0,
		mpb.BarStyle().Lbound(" ").Filler("━").Tip("━").Padding("─").Rbound(" "),
		opts...,
	)
	return t
}

// barTask renders one mpb bar. The mutable fields are read from mpb's render
// goroutine (via the decorator closures) and written from the worker goroutine,
// so every access is guarded by mu.
type barTask struct {
	bar *mpb.Bar

	mu     sync.Mutex
	stage  string
	detail string
	failed bool
	total  int64
}

// statusText is the left-hand status: the current stage while in flight, a
// green check with detail once complete, or a red cross with the error.
func (t *barTask) statusText(st decor.Statistics) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch {
	case t.failed:
		return Colorize(Red, "✗ ") + t.detail
	case st.Completed:
		return Colorize(Green, "✓ ") + t.detail
	default:
		return t.stage
	}
}

// rightText shows download counters while a sized download is in flight, and
// nothing otherwise (queued, indeterminate, complete or failed).
func (t *barTask) rightText(st decor.Statistics) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.failed || st.Completed || t.total <= 0 {
		return ""
	}
	return fmt.Sprintf("%s / %s", humanBytes(st.Current), humanBytes(t.total))
}

func (t *barTask) Stage(msg string) {
	t.mu.Lock()
	t.stage = msg
	t.mu.Unlock()
}

func (t *barTask) SetTotal(total int64) {
	t.mu.Lock()
	t.total = total
	t.mu.Unlock()
	if total > 0 {
		t.bar.SetTotal(total, false)
	}
}

func (t *barTask) Write(p []byte) (int, error) {
	t.bar.IncrBy(len(p))
	return len(p), nil
}

func (t *barTask) Advance(n int64) { t.bar.IncrBy(int(n)) }

func (t *barTask) Done(detail string) {
	t.mu.Lock()
	t.detail = detail
	t.mu.Unlock()
	// Complete at whatever has been transferred so the bar fills to 100%
	// regardless of whether the size was known.
	t.bar.SetTotal(t.bar.Current(), true)
}

func (t *barTask) Fail(err error) {
	t.mu.Lock()
	t.failed = true
	t.detail = errText(err)
	t.mu.Unlock()
	t.bar.Abort(false) // keep the aborted row on screen
}

// ── plain line logging (non-terminal) ──────────────────────────────────────────

type lineProgress struct{ mu sync.Mutex }

func (l *lineProgress) Wait() {}

func (l *lineProgress) Task(name string) Task { return &lineTask{name: name, p: l} }

type lineTask struct {
	name string
	p    *lineProgress
}

func (t *lineTask) log(s string) {
	t.p.mu.Lock()
	defer t.p.mu.Unlock()
	fmt.Printf("  %s: %s\n", t.name, s)
}

func (t *lineTask) Stage(msg string)            { t.log(msg) }
func (t *lineTask) SetTotal(int64)              {}
func (t *lineTask) Advance(int64)               {}
func (t *lineTask) Write(p []byte) (int, error) { return len(p), nil }
func (t *lineTask) Done(detail string)          { t.log("done " + detail) }
func (t *lineTask) Fail(err error)              { t.log("failed: " + errText(err)) }

// ── no-op (dry-run / tests) ─────────────────────────────────────────────────────

// discardProgress renders nothing. DiscardProgress is exported for callers and
// tests that need a Progress but no output.
type discardProgress struct{}

// DiscardProgress is a Progress that renders nothing.
func DiscardProgress() Progress { return discardProgress{} }

func (discardProgress) Task(string) Task { return discardTask{} }
func (discardProgress) Wait()            {}

type discardTask struct{}

func (discardTask) Stage(string)                {}
func (discardTask) SetTotal(int64)              {}
func (discardTask) Advance(int64)               {}
func (discardTask) Write(p []byte) (int, error) { return len(p), nil }
func (discardTask) Done(string)                 {}
func (discardTask) Fail(error)                  {}

// ── helpers ─────────────────────────────────────────────────────────────────────

// errText renders an error for a status row, tolerating a nil error (the Task
// interface does not forbid Fail(nil)) so a stray nil never panics the renderer.
func errText(err error) string {
	if err == nil {
		return "failed"
	}
	return err.Error()
}

// humanBytes formats a byte count in binary units (KiB, MiB, …).
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
