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

	simpleCache, err := NewLRUCache(2)
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

	lruCache, err := NewLRUCache(1000)
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
