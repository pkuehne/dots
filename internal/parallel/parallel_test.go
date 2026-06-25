package parallel

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestRunPreservesOrder(t *testing.T) {
	in := []int{0, 1, 2, 3, 4, 5, 6, 7}
	out := make([]int, len(in))
	Run(in, 3, func(i, item int) {
		out[i] = item * 2
	})
	for i, v := range out {
		if v != in[i]*2 {
			t.Fatalf("out[%d] = %d, want %d", i, v, in[i]*2)
		}
	}
}

func TestRunBoundsConcurrency(t *testing.T) {
	const jobs = 2
	var inflight, peak int32
	Run(make([]int, 12), jobs, func(int, int) {
		n := atomic.AddInt32(&inflight, 1)
		for {
			p := atomic.LoadInt32(&peak)
			if n <= p || atomic.CompareAndSwapInt32(&peak, p, n) {
				break
			}
		}
		time.Sleep(2 * time.Millisecond)
		atomic.AddInt32(&inflight, -1)
	})
	if peak > jobs {
		t.Fatalf("peak concurrency %d exceeded jobs %d", peak, jobs)
	}
}

func TestRunZeroJobsRunsSerially(t *testing.T) {
	var count int32
	Run([]int{1, 2, 3}, 0, func(int, int) {
		atomic.AddInt32(&count, 1)
	})
	if count != 3 {
		t.Fatalf("count = %d, want 3", count)
	}
}

func TestRunEmpty(t *testing.T) {
	called := false
	Run([]int{}, 4, func(int, int) { called = true })
	if called {
		t.Fatal("fn called for empty slice")
	}
}
