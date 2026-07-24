/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import "sync"

// forEachParallel calls fn once for every item in items, using up to
// workers goroutines at a time. It waits for all calls to complete before
// returning. fn must be safe to call concurrently from multiple
// goroutines. If workers is less than 1, a single worker is used.
func forEachParallel[T any](items []T, workers int, fn func(T)) {
	if workers < 1 {
		workers = 1
	}
	if workers > len(items) {
		workers = len(items)
	}
	if workers <= 1 {
		for _, item := range items {
			fn(item)
		}
		return
	}

	work := make(chan T)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for item := range work {
				fn(item)
			}
		}()
	}
	for _, item := range items {
		work <- item
	}
	close(work)
	wg.Wait()
}
