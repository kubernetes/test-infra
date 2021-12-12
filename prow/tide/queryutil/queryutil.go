/*
Copyright 2021 The Kubernetes Authors.

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

// Package queryutil provides a utility that helps to query api v4 with
// given number of goroutines running concurrently
package queryutil

import (
	"sync"
)

type QueryData struct {
	Org         string
	Query       string
	MetricIndex int
}

// QueriesInParallel runs process function concurrently respecting maximum number of goroutines
func QueriesInParallel(goroutines uint, queries []QueryData, process func(QueryData)) {
	toQuery := make(chan QueryData, len(queries))
	for _, q := range queries {
		toQuery <- q
	}
	close(toQuery)

	graphQLRoutines := int(goroutines)
	if goroutines == 0 {
		graphQLRoutines = len(queries)
	}
	var wg sync.WaitGroup
	for i := 0; i < graphQLRoutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for q := range toQuery {
				process(q)
			}
		}()
	}
	wg.Wait()
}
