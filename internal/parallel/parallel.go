// Package parallel provides a small bounded worker-pool helper shared by the
// concurrent, progress-driven commands (apply's file deploy, repos clone/update,
// tools install/update). It exists so those call sites stop hand-rolling the
// same semaphore + WaitGroup + ordered-results loop.
package parallel

import "sync"

// Run executes fn for each item in items, running at most jobs invocations
// concurrently, and blocks until all have finished. The index is passed to fn
// so callers can write results[i] from each goroutine without a shared mutex —
// distinct indices never race. A jobs value below 1 is treated as 1.
func Run[T any](items []T, jobs int, fn func(i int, item T)) {
	if jobs < 1 {
		jobs = 1
	}

	sem := make(chan struct{}, jobs)
	var wg sync.WaitGroup

	for i, item := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, item T) {
			defer wg.Done()
			defer func() { <-sem }()
			fn(i, item)
		}(i, item)
	}
	wg.Wait()
}
