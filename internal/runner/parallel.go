package runner

import "sync"

// runParallel runs work for each item with at most parallelism concurrent
// goroutines, writing each result into the returned slice at the item's
// original index (so output order matches input order). A parallelism of zero
// or less is treated as one. It returns the first non-nil error any worker
// produced; the results slice is always returned so partially-completed work
// remains inspectable.
//
// This is the shared fan-out used by Run (and, later, the apply runner).
// Callers that need cancellation/fail-fast thread a context through the work
// closure.
func runParallel[Item any](items []Item, parallelism int, work func(idx int, item Item) (SuiteResult, error)) ([]SuiteResult, error) {
	if parallelism <= 0 {
		parallelism = 1
	}

	results := make([]SuiteResult, len(items))
	errs := make([]error, len(items))

	sem := make(chan struct{}, parallelism)
	var wg sync.WaitGroup
	for idx, item := range items {
		wg.Add(1)
		go func(idx int, item Item) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx], errs[idx] = work(idx, item)
		}(idx, item)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return results, err
		}
	}
	return results, nil
}
