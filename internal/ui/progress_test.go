package ui

import "testing"

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1048576, "1.0 MiB"},
		{6_710_886, "6.4 MiB"},
	}
	for _, c := range cases {
		if got := humanBytes(c.in); got != c.want {
			t.Errorf("humanBytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestDiscardProgress checks the no-op renderer satisfies the contract without
// panicking: a task can be driven through every method, and Write reports the
// full length so it works as an io.Writer sink.
func TestDiscardProgress(t *testing.T) {
	p := DiscardProgress()
	task := p.Task("x")
	task.Stage("resolving")
	task.SetTotal(100)
	n, err := task.Write([]byte("hello"))
	if err != nil || n != 5 {
		t.Fatalf("Write = (%d, %v), want (5, nil)", n, err)
	}
	task.Done("1.0.0")
	task.Fail(nil) // terminal calls are tolerated back-to-back in tests
	p.Wait()
}

// TestLineProgressSatisfiesInterface ensures the non-terminal renderer compiles
// to the Progress/Task contract and tolerates the full drive sequence.
func TestLineProgressSatisfiesInterface(t *testing.T) {
	var p Progress = &lineProgress{}
	task := p.Task("tool")
	task.Stage("downloading")
	if n, err := task.Write([]byte("abc")); err != nil || n != 3 {
		t.Fatalf("Write = (%d, %v), want (3, nil)", n, err)
	}
	task.Done("ok")
	p.Wait()
}
