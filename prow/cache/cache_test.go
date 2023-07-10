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

package cache

import (
	"fmt"
	"sync"
	"testing"
)

// TestGetOrAddSimple is a basic check that the underlying LRU cache
// implementation that powers the LRUCache itself is behaving in an expected
// way. We test things like cache eviction and also what to do when value
// construction fails.
func TestGetOrAddSimple(t *testing.T) {
	valConstructorCalls := 0
	goodValConstructor := func(val string) func() (interface{}, error) {
		return func() (interface{}, error) {
			valConstructorCalls++
			return "(val)" + val, nil
		}
	}
	badValConstructor := func(key string) func() (interface{}, error) {
		return func() (interface{}, error) {
			valConstructorCalls++
			return "", fmt.Errorf("could not construct val")
		}
	}

	goodValConstructorForInitialState := func(val string) func() (interface{}, error) {
		return func() (interface{}, error) {
			return val, nil
		}
	}

	simpleCache, err := NewLRUCache(2, Callbacks{})
	if err != nil {
		t.Error("could not initialize simpleCache")
	}

	type expected struct {
		val                 string
		err                 string
		valConstructorCalls int
		cachedValues        int
	}

	for _, tc := range []struct {
		name              string
		cache             *LRUCache
		cacheInitialState map[string]string
		key               string
		valConstructor    ValConstructor
		expected          expected
	}{
		{
			name:              "EmptyCache",
			cache:             simpleCache,
			cacheInitialState: nil,
			key:               "foo",
			valConstructor:    goodValConstructor("foo"),
			expected: expected{
				val:                 "(val)foo",
				err:                 "",
				valConstructorCalls: 1,
				cachedValues:        1,
			},
		},
		{
			name:  "CacheMissWithoutValueEviction",
			cache: simpleCache,
			cacheInitialState: map[string]string{
				"(key)foo": "(val)foo",
			},
			key:            "bar",
			valConstructor: goodValConstructor("bar"),
			expected: expected{
				val:                 "(val)bar",
				err:                 "",
				valConstructorCalls: 1,
				cachedValues:        2,
			},
		},
		{
			name:  "CacheMissWithValueEviction",
			cache: simpleCache,
			cacheInitialState: map[string]string{
				"(key)foo": "(val)foo",
				"(key)bar": "(val)bar",
			},
			key:            "cat",
			valConstructor: goodValConstructor("cat"),
			expected: expected{
				val:                 "(val)cat",
				err:                 "",
				valConstructorCalls: 1,
				// There are still only 2 values in the cache, even though we
				// tried to add a 3rd item ("cat").
				cachedValues: 2,
			},
		},
		{
			name:  "CacheHit",
			cache: simpleCache,
			cacheInitialState: map[string]string{
				"(key)foo": "(val)foo",
				"(key)bar": "(val)bar",
			},
			key:            "(key)bar",
			valConstructor: goodValConstructor("bar"),
			expected: expected{
				val: "(val)bar",
				err: "",
				// If the constructed value is already in the cache, we do not
				// need to construct it from scratch.
				valConstructorCalls: 0,
				cachedValues:        2,
			},
		},
		{
			// Constructing the value resulted in an error. We evict this entry
			// from the cache.
			name:              "BadValConstructor",
			cache:             simpleCache,
			cacheInitialState: nil,
			key:               "bar",
			valConstructor:    badValConstructor("bar"),
			expected: expected{
				val:                 "",
				err:                 "could not construct val",
				valConstructorCalls: 1,
				cachedValues:        0,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Reset test state.
			valConstructorCalls = 0
			simpleCache.Purge()

			for k, v := range tc.cacheInitialState {
				if tc.cache != nil {
					_, _, _ = tc.cache.GetOrAdd(k, goodValConstructorForInitialState(v))
				}
			}

			val, _, err := tc.cache.GetOrAdd(tc.key, tc.valConstructor)

			if tc.expected.err == "" {
				if err != nil {
					t.Errorf("Expected error 'nil' got '%v'", err.Error())
				}
			} else {
				if err == nil {
					t.Fatal("Expected non-nil error, got nil")
				}

				if tc.expected.err != err.Error() {
					t.Errorf("Expected error '%v', got '%v'", tc.expected.err, err.Error())
				}
			}

			if tc.expected.val == "<nil>" {
				if val != nil {
					t.Errorf("Expected val to be nil, got '%v'", val)
				}
			} else {
				if tc.expected.val != val {
					t.Errorf("Expected val '%v', got '%v'", tc.expected.val, val)
				}
			}

			if tc.expected.valConstructorCalls != valConstructorCalls {
				t.Errorf("Expected '%d' calls to valConstructor(), got '%d'", tc.expected.valConstructorCalls, valConstructorCalls)
			}

			if tc.cache != nil && tc.expected.cachedValues != tc.cache.Len() {
				t.Errorf("Expected cachedValues to be '%d', got '%d'", tc.expected.cachedValues, tc.cache.Len())
			}
		})
	}
}

// TestGetOrAddBurst tests getting 1000 sudden requests for the same cache key
// at the same time. Because our cache can handle this situation (called "cache
// stampede" or by its mitigation strategy known as "duplicate suppression"), we
// expect to only have created a __single__ cached entry, with the remaining 999
// "get" calls against the cache to reuse the cached entry. The HTTP analogue of
// duplicate suppression is known as request coalescing, which uses the same
// principle. For more discussion about duplicate suppression, see Alan Donovan
// and Brian Kernighan, "The Go Programming Language" (Addison-Wesley, 2016), p.
// 277.
func TestGetOrAddBurst(t *testing.T) {
	// testLock is used for guarding valConstructorCalls for purposes of
	// testing.
	testLock := sync.Mutex{}

	valConstructorCalls := 0
	// goodValConstructor simulates an "expensive" call by calculating the
	// Collatz Conjecture for a small input. The point is that the value
	// generated here will never be able to be optimized away by the compiler
	// (because its value cannot be precomputed by the compiler), guaranteeing
	// that some CPU cycles will be spent between the time we unlock the
	// testLock and the time we retrieve the computed value (all within the same
	// thread).
	goodValConstructor := func(input int) func() (interface{}, error) {
		return func() (interface{}, error) {
			testLock.Lock()
			valConstructorCalls++
			testLock.Unlock()
			steps := 0
			n := input
			max := input
			for n > 1 {
				if n > max {
					max = n
				}
				if n&1 == 0 {
					n >>= 1
				} else {
					n *= 3
					n++
				}
				steps++
			}
			return fmt.Sprintf("(val)input=%d,steps=%d,max=%d", input, steps, max), nil
		}
	}

	lruCache, err := NewLRUCache(1000, Callbacks{})
	if err != nil {
		t.Error("could not initialize lruCache")
	}

	valConstructorCalls = 0
	const maxConcurrentRequests = 500
	wg := sync.WaitGroup{}

	// Consider the case where all threads perform the same cache lookup.
	expectedVal := "(val)input=3,steps=7,max=16"
	wg.Add(maxConcurrentRequests)
	for i := 0; i < maxConcurrentRequests; i++ {
		go func() {
			// Input of 3 for goodValConstructor will take 7 steps and reach a
			// maximum value of 16. We check this below.
			constructedVal, _, err := lruCache.GetOrAdd(3, goodValConstructor(3))
			if err != nil {
				t.Error("could not fetch or construct value")
			}
			if constructedVal != expectedVal {
				t.Errorf("expected constructed value '%v', got '%v'", expectedVal, constructedVal)
			}
			wg.Done()
		}()
	}
	wg.Wait()

	// Expect that we only invoked the goodValConstructor once. Notice how the
	// user of lruCache does not need to worry about locking. The cache is smart
	// enough to perform duplicate suppression on its own, so that the value is
	// constructed and written into the cache only once, no matter how many
	// concurrent threads attempt to access it.
	if valConstructorCalls != 1 {
		t.Errorf("Expected valConstructorCalls '1', got '%v'", valConstructorCalls)
	}
	if lruCache.Len() != 1 {
		t.Errorf("Expected single cached element, got '%v'", lruCache.Len())
	}

	valConstructorCalls = 0
	lruCache.Purge()

	// Consider the case where all threads perform one of 5 different cache lookups.
	wg.Add(maxConcurrentRequests)
	for i := 0; i < maxConcurrentRequests; i++ {
		j := (i % 5) + 1
		expectedVal := ""
		go func() {
			constructedVal, _, err := lruCache.GetOrAdd(j, goodValConstructor(j))
			if err != nil {
				t.Error("could not fetch or construct value")
			}
			switch j {
			case 1:
				expectedVal = "(val)input=1,steps=0,max=1"
			case 2:
				expectedVal = "(val)input=2,steps=1,max=2"
			case 3:
				expectedVal = "(val)input=3,steps=7,max=16"
			case 4:
				expectedVal = "(val)input=4,steps=2,max=4"
			default:
				expectedVal = "(val)input=5,steps=5,max=16"
			}
			if constructedVal != expectedVal {
				t.Errorf("expected constructed value '%v', got '%v'", expectedVal, constructedVal)
			}
			wg.Done()
		}()
	}
	wg.Wait()

	// Only expect 5 valConstructor calls, because there are only 5 unique key lookups.
	if valConstructorCalls != 5 {
		t.Errorf("Expected valConstructorCalls '5', got '%v'", valConstructorCalls)
	}
	if lruCache.Len() != 5 {
		t.Errorf("Expected 5 cached entries, got '%v'", lruCache.Len())
	}
}

func TestCallbacks(t *testing.T) {
	goodValConstructor := func(val string) func() (interface{}, error) {
		return func() (interface{}, error) {
			return val, nil
		}
	}
	badValConstructor := func(val string) func() (interface{}, error) {
		return func() (interface{}, error) {
			return "", fmt.Errorf("could not construct val")
		}
	}

	lookupsCounter := 0
	hitsCounter := 0
	missesCounter := 0
	forcedEvictionsCounter := 0
	manualEvictionsCounter := 0

	counterLock := &sync.Mutex{}
	mkCallback := func(counter *int) EventCallback {
		callback := func(key interface{}) {
			counterLock.Lock()
			(*counter)++
			counterLock.Unlock()
		}
		return callback
	}

	lookupsCallback := mkCallback(&lookupsCounter)
	hitsCallback := mkCallback(&hitsCounter)
	missesCallback := mkCallback(&missesCounter)
	forcedEvictionsCallback := func(key interface{}, _ interface{}) {
		forcedEvictionsCounter++
	}
	manualEvictionsCallback := mkCallback(&manualEvictionsCounter)

	defaultCallbacks := Callbacks{
		LookupsCallback:         lookupsCallback,
		HitsCallback:            hitsCallback,
		MissesCallback:          missesCallback,
		ForcedEvictionsCallback: forcedEvictionsCallback,
		ManualEvictionsCallback: manualEvictionsCallback,
	}

	type expected struct {
		lookups         int
		hits            int
		misses          int
		forcedEvictions int
		manualEvictions int
		// If the value constructor is flaky, then it can result in a (mostly
		// harmless) race in which events occur. For example, the key may be
		// evicted by either the underlying LRU cache (if it gets to it first),
		// or by us when we manually try to evict it. This can result in an
		// unpredictable number of forced versus manual evictions.
		//
		// This flakiness can have cascading effects to the other metrics like
		// lookups/hits/misses. So, if our test case has bad constructors in it,
		// we need to be less strict about how we compare these expected results
		// versus what we get.
		racyEvictions bool
	}

	type lookup struct {
		key            string
		valConstructor func(val string) func() (interface{}, error)
	}

	for _, tc := range []struct {
		name              string
		cacheSize         int
		cacheInitialState map[string]string
		cacheCallbacks    Callbacks
		// Perform lookups for each key here. It could result in a hit or miss.
		lookups  []lookup
		expected expected
	}{
		{
			name:      "NoDefinedCallbacksResultsInNOP",
			cacheSize: 2,
			cacheInitialState: map[string]string{
				"(key)foo": "(val)bar",
			},
			cacheCallbacks: Callbacks{},
			lookups: []lookup{
				{"(key)foo", goodValConstructor},
			},
			expected: expected{
				lookups:         0,
				hits:            0,
				misses:          0,
				forcedEvictions: 0,
				manualEvictions: 0,
			},
		},
		{
			name:              "OneHitOneMiss",
			cacheSize:         2,
			cacheInitialState: map[string]string{},
			cacheCallbacks:    defaultCallbacks,
			lookups: []lookup{
				{"(key)foo", goodValConstructor},
				{"(key)foo", goodValConstructor},
			},
			expected: expected{
				lookups: 2,
				// One hit for a subsequent successful lookup.
				hits: 1,
				// One miss for the initial cache construction (initial state).
				misses:          1,
				forcedEvictions: 0,
				manualEvictions: 0,
			},
		},
		{
			name:              "ManyMissesAndSomeForcedEvictions",
			cacheSize:         2,
			cacheInitialState: map[string]string{},
			cacheCallbacks:    defaultCallbacks,
			lookups: []lookup{
				{"(key)1", goodValConstructor},
				{"(key)2", goodValConstructor},
				{"(key)3", goodValConstructor},
				{"(key)4", goodValConstructor},
				{"(key)5", goodValConstructor},
			},
			expected: expected{
				lookups: 5,
				hits:    0,
				misses:  5,
				// 3 Forced evictions because the cache size is 2.
				forcedEvictions: 3,
				manualEvictions: 0,
			},
		},
		{
			name:              "ManualEvictions",
			cacheSize:         2,
			cacheInitialState: map[string]string{},
			cacheCallbacks:    defaultCallbacks,
			lookups: []lookup{
				lookup{"(key)1", goodValConstructor},
				lookup{"(key)2", goodValConstructor},
				lookup{"(key)3", goodValConstructor},
				lookup{"(key)1", badValConstructor},
				lookup{"(key)2", badValConstructor},
				lookup{"(key)3", badValConstructor},
			},
			expected: expected{
				lookups:         6,
				hits:            0,
				misses:          0,
				forcedEvictions: 0,
				manualEvictions: 0,
				// If racyEvictions is true, then we expect some positive number of evictions to occur.
				racyEvictions: true,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache, err := NewLRUCache(tc.cacheSize, tc.cacheCallbacks)
			if err != nil {
				t.Error("could not initialize simpleCache")
			}
			// Reset test state.
			lookupsCounter = 0
			hitsCounter = 0
			missesCounter = 0
			forcedEvictionsCounter = 0
			manualEvictionsCounter = 0

			var wg sync.WaitGroup

			// For the sake of realism, perform all lookups concurrently. The
			// concurrency should have no effect on the operation of the
			// callbacks.
			for k, v := range tc.cacheInitialState {
				k := k
				v := v
				wg.Add(1)
				go func() {
					cache.GetOrAdd(k, goodValConstructor(v))
					wg.Done()
				}()
			}

			for _, lookup := range tc.lookups {
				lookup := lookup
				wg.Add(1)
				go func() {
					cache.GetOrAdd(lookup.key, lookup.valConstructor("(val)"+lookup.key))
					wg.Done()
				}()
			}
			wg.Wait()

			if tc.expected.lookups != lookupsCounter {
				t.Errorf("Expected lookupsCounter to be '%d', got '%d'", tc.expected.lookups, lookupsCounter)
			}

			// If we expect racy evictions, then we expect *some* evictions to occur.
			if tc.expected.racyEvictions {
				totalEvictions := forcedEvictionsCounter + manualEvictionsCounter
				if totalEvictions == 0 {
					t.Errorf("Expected total evictions to be greater than 0, got '%d'", totalEvictions)
				}
			} else {
				if tc.expected.hits != hitsCounter {
					t.Errorf("Expected hitsCounter to be '%d', got '%d'", tc.expected.hits, hitsCounter)
				}
				if tc.expected.misses != missesCounter {
					t.Errorf("Expected missesCounter to be '%d', got '%d'", tc.expected.misses, missesCounter)
				}
				if tc.expected.forcedEvictions != forcedEvictionsCounter {
					t.Errorf("Expected forcedEvictionsCounter to be '%d', got '%d'", tc.expected.forcedEvictions, forcedEvictionsCounter)
				}
				if tc.expected.manualEvictions != manualEvictionsCounter {
					t.Errorf("Expected manualEvictionsCounter to be '%d', got '%d'", tc.expected.manualEvictions, manualEvictionsCounter)
				}
			}
		})
	}
}
