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

package config

import (
	"fmt"
	"reflect"
	"sync"
	"testing"

	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/test-infra/prow/git/v2"
)

func TestNewProwYAMLCache(t *testing.T) {
	// Invalid size arguments result in a nil prowYAMLCache and non-nil err.
	invalids := []int{-1, 0}
	for _, invalid := range invalids {

		prowYAMLCache, err := NewProwYAMLCache(invalid)

		if err == nil {
			t.Fatal("Expected non-nil error, got nil")
		}

		if err.Error() != "Must provide a positive size" {
			t.Errorf("Expected error 'Must provide a positive size', got '%v'", err.Error())
		}

		if prowYAMLCache != nil {
			t.Errorf("Expected nil prowYAMLCache, got %v", prowYAMLCache)
		}
	}

	// Valid size arguments.
	valids := []int{1, 5, 1000}
	for _, valid := range valids {

		prowYAMLCache, err := NewProwYAMLCache(valid)

		if err != nil {
			t.Errorf("Expected error 'nil' got '%v'", err.Error())
		}

		if prowYAMLCache == nil {
			t.Errorf("Expected non-nil prowYAMLCache, got nil")
		}
	}
}

func goodSHAGetter(sha string) func() (string, error) {
	return func() (string, error) {
		return sha, nil
	}
}

func badSHAGetter() (string, error) {
	return "", fmt.Errorf("badSHAGetter")
}

func TestMakeCacheKey(t *testing.T) {
	type expected struct {
		cacheKeyParts CacheKeyParts
		err           string
	}

	for _, tc := range []struct {
		name           string
		identifier     string
		baseSHAGetter  RefGetter
		headSHAGetters []RefGetter
		expected       expected
	}{
		{
			name:          "Basic",
			identifier:    "foo/bar",
			baseSHAGetter: goodSHAGetter("ba5e"),
			headSHAGetters: []RefGetter{
				goodSHAGetter("abcd"),
				goodSHAGetter("ef01")},
			expected: expected{
				CacheKeyParts{
					Identifier: "foo/bar",
					BaseSHA:    "ba5e",
					HeadSHAs:   []string{"abcd", "ef01"},
				},
				"",
			},
		},
		{
			name:           "NoHeadSHAGetters",
			identifier:     "foo/bar",
			baseSHAGetter:  goodSHAGetter("ba5e"),
			headSHAGetters: []RefGetter{},
			expected: expected{
				CacheKeyParts{
					Identifier: "foo/bar",
					BaseSHA:    "ba5e",
					HeadSHAs:   nil,
				},
				"",
			},
		},
		{
			name:          "EmptyIdentifierFailure",
			identifier:    "",
			baseSHAGetter: goodSHAGetter("ba5e"),
			headSHAGetters: []RefGetter{
				goodSHAGetter("abcd"),
				goodSHAGetter("ef01")},
			expected: expected{
				CacheKeyParts{},
				"identifier cannot be empty",
			},
		},
		{
			name:          "BaseSHAGetterFailure",
			identifier:    "foo/bar",
			baseSHAGetter: badSHAGetter,
			headSHAGetters: []RefGetter{
				goodSHAGetter("abcd"),
				goodSHAGetter("ef01")},
			expected: expected{
				CacheKeyParts{},
				"failed to get baseSHA: badSHAGetter",
			},
		},
		{
			name:          "HeadSHAGetterFailure",
			identifier:    "foo/bar",
			baseSHAGetter: goodSHAGetter("ba5e"),
			headSHAGetters: []RefGetter{
				goodSHAGetter("abcd"),
				badSHAGetter},
			expected: expected{
				CacheKeyParts{},
				"failed to get headRef: badSHAGetter",
			},
		},
	} {
		t.Run(tc.name, func(t1 *testing.T) {
			cacheKeyParts, err := MakeCacheKeyParts(tc.identifier, tc.baseSHAGetter, tc.headSHAGetters...)

			if tc.expected.err == "" {
				if err != nil {
					t.Errorf("Expected error 'nil' got '%v'", err.Error())
				}
				if !reflect.DeepEqual(tc.expected.cacheKeyParts, cacheKeyParts) {
					t.Errorf("CacheKeyParts do not match:\n%s", diff.ObjectReflectDiff(tc.expected.cacheKeyParts, cacheKeyParts))
				}
			} else {
				if err == nil {
					t.Fatal("Expected non-nil error, got nil")
				}

				if tc.expected.err != err.Error() {
					t.Errorf("Expected error '%v', got '%v'", tc.expected.err, err.Error())
				}
			}
		})
	}
}

func TestGetOrAddSimple(t *testing.T) {
	keyConstructorCalls := 0
	goodKeyConstructor := func(key string) func() (CacheKey, error) {
		return func() (CacheKey, error) {
			keyConstructorCalls++
			return CacheKey("(key)" + key), nil
		}
	}
	badKeyConstructor := func(key string) func() (CacheKey, error) {
		return func() (CacheKey, error) {
			keyConstructorCalls++
			return CacheKey(""), fmt.Errorf("could not construct key")
		}
	}

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

	goodKeyConstructorForInitialState := func(key string) func() (CacheKey, error) {
		return func() (CacheKey, error) {
			return CacheKey(key), nil
		}
	}
	goodValConstructorForInitialState := func(val string) func() (interface{}, error) {
		return func() (interface{}, error) {
			return val, nil
		}
	}
	// simpleCache is a cache only used for testing. The difference between this
	// cache and the ones in ProwYAMLCache is that simpleCache only holds
	// strings, not ProwYAMLs as values.
	simpleCache, err := NewLRUCache(2)
	if err != nil {
		t.Error("could not initialize simpleCache")
	}

	type expected struct {
		val                 string
		err                 string
		keyConstructorCalls int
		valConstructorCalls int
		cachedValues        int
	}

	for _, tc := range []struct {
		name              string
		cache             *LRUCache
		cacheInitialState map[CacheKey]string
		keyConstructor    keyConstructor
		valConstructor    valConstructor
		expected          expected
	}{
		{
			name:              "NilCache",
			cache:             nil,
			cacheInitialState: nil,
			keyConstructor:    goodKeyConstructor("foo"),
			valConstructor:    goodValConstructor("foo"),
			expected: expected{
				val:                 "(val)foo",
				err:                 "",
				keyConstructorCalls: 0,
				valConstructorCalls: 1,
				// Since there is no cache, its size does not change even after
				// calling GetFromCache.
				cachedValues: 0,
			},
		},
		{
			name:              "EmptyCache",
			cache:             simpleCache,
			cacheInitialState: nil,
			keyConstructor:    goodKeyConstructor("foo"),
			valConstructor:    goodValConstructor("foo"),
			expected: expected{
				val:                 "(val)foo",
				err:                 "",
				keyConstructorCalls: 1,
				valConstructorCalls: 1,
				cachedValues:        1,
			},
		},
		{
			name:  "CacheMissWithoutValueEviction",
			cache: simpleCache,
			cacheInitialState: map[CacheKey]string{
				"(key)foo": "(val)foo",
			},
			keyConstructor: goodKeyConstructor("bar"),
			valConstructor: goodValConstructor("bar"),
			expected: expected{
				val:                 "(val)bar",
				err:                 "",
				keyConstructorCalls: 1,
				valConstructorCalls: 1,
				cachedValues:        2,
			},
		},
		{
			name:  "CacheMissWithValueEviction",
			cache: simpleCache,
			cacheInitialState: map[CacheKey]string{
				"(key)foo": "(val)foo",
				"(key)bar": "(val)bar",
			},
			keyConstructor: goodKeyConstructor("cat"),
			valConstructor: goodValConstructor("cat"),
			expected: expected{
				val:                 "(val)cat",
				err:                 "",
				keyConstructorCalls: 1,
				valConstructorCalls: 1,
				// There are still only 2 values in the cache, even though we
				// tried to add a 3rd item ("cat").
				cachedValues: 2,
			},
		},
		{
			name:  "CacheHit",
			cache: simpleCache,
			cacheInitialState: map[CacheKey]string{
				"(key)foo": "(val)foo",
				"(key)bar": "(val)bar",
			},
			keyConstructor: goodKeyConstructor("bar"),
			valConstructor: goodValConstructor("bar"),
			expected: expected{
				val:                 "(val)bar",
				err:                 "",
				keyConstructorCalls: 1,
				// If the constructed value is already in the cache, we do not
				// need to construct it from scratch.
				valConstructorCalls: 0,
				cachedValues:        2,
			},
		},
		{
			name:              "BadKeyConstructor",
			cache:             simpleCache,
			cacheInitialState: nil,
			keyConstructor:    badKeyConstructor("bar"),
			valConstructor:    goodValConstructor("bar"),
			expected: expected{
				val:                 "<nil>",
				err:                 "could not construct key",
				keyConstructorCalls: 1,
				valConstructorCalls: 0,
				cachedValues:        0,
			},
		},
		{
			// Constructing the value resulted in an error. We evict this entry
			// from the cache.
			name:              "BadValConstructor",
			cache:             simpleCache,
			cacheInitialState: nil,
			keyConstructor:    goodKeyConstructor("bar"),
			valConstructor:    badValConstructor("bar"),
			expected: expected{
				val:                 "",
				err:                 "could not construct val",
				keyConstructorCalls: 1,
				valConstructorCalls: 1,
				cachedValues:        0,
			},
		},
		{
			name:              "BadValConstructorNilCache",
			cache:             nil,
			cacheInitialState: nil,
			keyConstructor:    goodKeyConstructor("bar"),
			valConstructor:    badValConstructor("bar"),
			expected: expected{
				val:                 "<nil>",
				err:                 "could not construct val",
				keyConstructorCalls: 0,
				valConstructorCalls: 1,
				cachedValues:        0,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Reset test state.
			keyConstructorCalls = 0
			valConstructorCalls = 0
			simpleCache.Purge()

			for k, v := range tc.cacheInitialState {
				if tc.cache != nil {
					_, _ = tc.cache.GetOrAdd(goodKeyConstructorForInitialState(string(k)), goodValConstructorForInitialState(v))
				}
			}

			val, err := tc.cache.GetOrAdd(tc.keyConstructor, tc.valConstructor)

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

			if tc.expected.keyConstructorCalls != keyConstructorCalls {
				t.Errorf("Expected '%d' calls to keyConstructor(), got '%d'", tc.expected.keyConstructorCalls, keyConstructorCalls)
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
// against the cache at the same time. Because our cache can handle this
// situation (called "cache stampede" or by its mitigation strategy known as
// "duplicate suppression"), we expect to only have created a **single** cached
// entry, with the remaining 999 "get" calls against the cache to reuse the
// cached entry. The HTTP analogue of duplicate suppression is known as request
// coalescing, which uses the same principle. For more discussion about
// duplicate suppression, see Alan Donovan and Brian Kernighan, "The Go
// Programming Language" (Addison-Wesley, 2016), p. 276.
func TestGetOrAddBurst(t *testing.T) {
	// testLock is used for guarding keyConstructorCalls and valConstructorCalls
	// for purposes of testing.
	testLock := sync.Mutex{}

	keyConstructorCalls := 0
	// Always return the same key. This way we force all GetOrAdd() calls to
	// target the same cache entry.
	goodKeyConstructor := func(input int) func() (CacheKey, error) {
		return func() (CacheKey, error) {
			testLock.Lock()
			keyConstructorCalls++
			testLock.Unlock()
			return CacheKey(fmt.Sprintf("(key)%d", input)), nil
		}
	}

	valConstructorCalls := 0
	// This value is expensive to calculate. We simulate an "expensive" call by
	// calculating the Collatz Conjecture for a small input. The point is that
	// the value generated here will never be able to be optimized away by the
	// compiler (because its value cannot be precomputed by the compiler),
	// guaranteeing that some CPU cycles will be spent between the time we
	// unlock the testLock and the time we send a true value to the
	// valConstructionFinished channel.
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

	keyConstructorCalls = 0
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
			constructedVal, err := lruCache.GetOrAdd(goodKeyConstructor(3), goodValConstructor(3))
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
	if keyConstructorCalls != maxConcurrentRequests {
		t.Errorf("Expected keyConstructorCalls '%v', got '%v'", maxConcurrentRequests, keyConstructorCalls)
	}

	keyConstructorCalls = 0
	valConstructorCalls = 0
	lruCache.Purge()
	// Consider the case where all threads perform one of 5 different cache lookups.
	wg.Add(maxConcurrentRequests)
	for i := 0; i < maxConcurrentRequests; i++ {
		j := (i % 5) + 1
		expectedVal := ""
		go func() {
			constructedVal, err := lruCache.GetOrAdd(goodKeyConstructor(j), goodValConstructor(j))
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
	if keyConstructorCalls != maxConcurrentRequests {
		t.Errorf("Expected keyConstructorCalls '%v', got '%v'", maxConcurrentRequests, keyConstructorCalls)
	}
}

func TestGetProwYAMLFromCache(t *testing.T) {
	// fakeProwYAMLMap mocks prowYAMLGetter. Instead of using the
	// git.ClientFactory (and other operations), we just use a simple map to get
	// the *ProwYAML value we want. For simplicity we just reuse MakeCacheKey
	// even though we're not using a cache. The point of fakeProwYAMLMap is to
	// act as a "source of truth" of authoritative *ProwYAML values for purposes
	// of the test cases in this unit test.
	fakeProwYAMLMap := make(map[CacheKey]*ProwYAML)

	// goodValConstructor mocks config.getProwYAML.
	// This map pretends to be an expensive computation in order to generate a
	// *ProwYAML value.
	goodValConstructor := func(gc git.ClientFactory, identifier string, baseSHAGetter RefGetter, headSHAGetters ...RefGetter) (*ProwYAML, error) {

		keyParts, err := MakeCacheKeyParts(identifier, baseSHAGetter, headSHAGetters...)
		if err != nil {
			t.Fatal(err)
		}

		key, err := MakeCacheKey(keyParts)
		if err != nil {
			t.Fatal(err)
		}

		val, ok := fakeProwYAMLMap[key]
		if ok {
			return val, nil
		}

		return nil, fmt.Errorf("unable to construct *ProwYAML value")
	}

	fakeProwYAMLs := []CacheKeyParts{
		{
			Identifier: "foo/bar",
			BaseSHA:    "ba5e",
			HeadSHAs:   []string{"abcd", "ef01"},
		},
	}
	// Populate fakeProwYAMLMap.
	for _, fakeProwYAML := range fakeProwYAMLs {
		// To make it easier to compare Presubmit values, we only set the
		// Name field and only compare this field. We also only create a
		// single Presubmit (singleton slice), again for simplicity. Lastly
		// we also set the Name field to the same value as the "key", again
		// for simplicity.
		fakeProwYAMLKey, err := MakeCacheKey(fakeProwYAML)
		if err != nil {
			t.Fatal(err)
		}
		fakeProwYAMLMap[fakeProwYAMLKey] = &ProwYAML{
			Presubmits: []Presubmit{
				{
					JobBase: JobBase{Name: string(fakeProwYAMLKey)},
				},
			},
		}
	}

	// goodKeyConstructorForInitialState is used for warming up the cache for
	// tests that need it.
	goodKeyConstructorForInitialState := func(key CacheKey) func() (CacheKey, error) {
		return func() (CacheKey, error) {
			return key, nil
		}
	}
	// goodValConstructorForInitialState is used for warming up the cache for
	// tests that need it.
	goodValConstructorForInitialState := func(val ProwYAML) func() (interface{}, error) {
		return func() (interface{}, error) {
			return &val, nil
		}
	}

	badValConstructor := func(gc git.ClientFactory, identifier string, baseSHAGetter RefGetter, headSHAGetters ...RefGetter) (*ProwYAML, error) {
		return nil, fmt.Errorf("unable to construct *ProwYAML value")
	}

	prowYAMLCache, err := NewProwYAMLCache(1)
	if err != nil {
		t.Fatal("could not initialize prowYAMLCache")
	}

	type expected struct {
		prowYAML *ProwYAML
		err      string
	}

	for _, tc := range []struct {
		name           string
		valConstructor func(git.ClientFactory, string, RefGetter, ...RefGetter) (*ProwYAML, error)
		// We use a slice of CacheKeysParts for simplicity.
		cacheInitialState []CacheKeyParts
		cacheCorrupted    bool
		identifier        string
		baseSHAGetter     RefGetter
		headSHAGetters    []RefGetter
		expected          expected
	}{
		{
			name:              "CacheMiss",
			valConstructor:    goodValConstructor,
			cacheInitialState: nil,
			cacheCorrupted:    false,
			identifier:        "foo/bar",
			baseSHAGetter:     goodSHAGetter("ba5e"),
			headSHAGetters: []RefGetter{
				goodSHAGetter("abcd"),
				goodSHAGetter("ef01")},
			expected: expected{
				prowYAML: &ProwYAML{
					Presubmits: []Presubmit{
						{
							JobBase: JobBase{Name: `{"identifier":"foo/bar","baseSHA":"ba5e","headSHAs":["abcd","ef01"]}`}},
					},
				},
				err: "",
			},
		},
		{
			// If we get a cache hit, the value constructor function doesn't
			// matter because it will never be called.
			name:           "CacheHit",
			valConstructor: badValConstructor,
			cacheInitialState: []CacheKeyParts{
				{
					Identifier: "foo/bar",
					BaseSHA:    "ba5e",
					HeadSHAs:   []string{"abcd", "ef01"},
				},
			},
			cacheCorrupted: false,
			identifier:     "foo/bar",
			baseSHAGetter:  goodSHAGetter("ba5e"),
			headSHAGetters: []RefGetter{
				goodSHAGetter("abcd"),
				goodSHAGetter("ef01")},
			expected: expected{
				prowYAML: &ProwYAML{
					Presubmits: []Presubmit{
						{
							JobBase: JobBase{Name: `{"identifier":"foo/bar","baseSHA":"ba5e","headSHAs":["abcd","ef01"]}`},
						},
					},
				},
				err: "",
			},
		},
		{
			name:              "BadValConstructorCacheMiss",
			valConstructor:    badValConstructor,
			cacheInitialState: nil,
			cacheCorrupted:    false,
			identifier:        "foo/bar",
			baseSHAGetter:     goodSHAGetter("ba5e"),
			headSHAGetters: []RefGetter{
				goodSHAGetter("abcd"),
				goodSHAGetter("ef01")},
			expected: expected{
				prowYAML: nil,
				err:      "unable to construct *ProwYAML value",
			},
		},
		{
			// If we get a cache hit, then it doesn't matter if the state of the
			// world was such that the value could not have been constructed from
			// scratch (because we're solely relying on the cache).
			name:           "BadValConstructorCacheHit",
			valConstructor: badValConstructor,
			cacheInitialState: []CacheKeyParts{
				{
					Identifier: "foo/bar",
					BaseSHA:    "ba5e",
					HeadSHAs:   []string{"abcd", "ef01"},
				},
			},
			cacheCorrupted: false,
			identifier:     "foo/bar",
			baseSHAGetter:  goodSHAGetter("ba5e"),
			headSHAGetters: []RefGetter{
				goodSHAGetter("abcd"),
				goodSHAGetter("ef01")},
			expected: expected{
				prowYAML: &ProwYAML{
					Presubmits: []Presubmit{
						{
							JobBase: JobBase{Name: `{"identifier":"foo/bar","baseSHA":"ba5e","headSHAs":["abcd","ef01"]}`}},
					},
				},
				err: "",
			},
		},
		{
			// If the cache is corrupted (it holds values of a type that is not
			// *ProwYAML), then we expect an error.
			name:           "GoodValConstructorCorruptedCacheHit",
			valConstructor: goodValConstructor,
			cacheInitialState: []CacheKeyParts{
				{
					Identifier: "foo/bar",
					BaseSHA:    "ba5e",
					HeadSHAs:   []string{"abcd", "ef01"},
				},
			},
			cacheCorrupted: true,
			identifier:     "foo/bar",
			baseSHAGetter:  goodSHAGetter("ba5e"),
			headSHAGetters: []RefGetter{
				goodSHAGetter("abcd"),
				goodSHAGetter("ef01")},
			expected: expected{
				prowYAML: nil,
				err:      "cache value type error: expected value type '*config.ProwYAML', got 'string'",
			},
		},
		{
			// If the cache is corrupted (it holds values of a type that is not
			// *ProwYAML), then we expect an error.
			name:           "BadValConstructorCorruptedCacheHit",
			valConstructor: badValConstructor,
			cacheInitialState: []CacheKeyParts{
				{
					Identifier: "foo/bar",
					BaseSHA:    "ba5e",
					HeadSHAs:   []string{"abcd", "ef01"},
				},
			},
			cacheCorrupted: true,
			identifier:     "foo/bar",
			baseSHAGetter:  goodSHAGetter("ba5e"),
			headSHAGetters: []RefGetter{
				goodSHAGetter("abcd"),
				goodSHAGetter("ef01")},
			expected: expected{
				prowYAML: nil,
				err:      "cache value type error: expected value type '*config.ProwYAML', got 'string'",
			},
		},
	} {
		t.Run(tc.name, func(t1 *testing.T) {
			// Reset test state.
			prowYAMLCache.Purge()

			for _, kp := range tc.cacheInitialState {
				k, err := MakeCacheKey(kp)
				if err != nil {
					t.Errorf("Expected error 'nil' got '%v'", err.Error())
				}
				_, _ = prowYAMLCache.GetOrAdd(goodKeyConstructorForInitialState(k), goodValConstructorForInitialState(ProwYAML{
					Presubmits: []Presubmit{
						{
							JobBase: JobBase{Name: string(k)}},
					},
				}))
			}

			// Simulate storing a value of the wrong type in the cache (a string
			// instead of a *ProwYAML).
			if tc.cacheCorrupted {
				prowYAMLCache.Purge()

				for _, kp := range tc.cacheInitialState {
					k, err := MakeCacheKey(kp)
					if err != nil {
						t.Errorf("Expected error 'nil' got '%v'", err.Error())
					}
					_, _ = prowYAMLCache.GetOrAdd(goodKeyConstructorForInitialState(k), func() (interface{}, error) { return "<wrong-type>", nil })
				}
			}

			prowYAML, err := GetProwYAMLFromCache(prowYAMLCache, tc.valConstructor, nil, tc.identifier, tc.baseSHAGetter, tc.headSHAGetters...)

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

			if tc.expected.prowYAML == nil && prowYAML != nil {
				t.Fatalf("Expected nil for *ProwYAML, got '%v'", *prowYAML)
			}

			if tc.expected.prowYAML != nil && prowYAML == nil {
				t.Fatal("Expected non-nil for *ProwYAML, got nil")
			}

			// If we got what we expected, there's no need to compare these two.
			if tc.expected.prowYAML == nil && prowYAML == nil {
				return
			}

			// The Presubmit type is not comparable. So instead of checking the
			// overall type for equality, we only check the Name field of it,
			// because it is a simple string type.
			if len(tc.expected.prowYAML.Presubmits) != len(prowYAML.Presubmits) {
				t.Fatalf("Expected prowYAML length '%d', got '%d'", len(tc.expected.prowYAML.Presubmits), len(prowYAML.Presubmits))
			}
			for i := range tc.expected.prowYAML.Presubmits {
				if tc.expected.prowYAML.Presubmits[i].Name != prowYAML.Presubmits[i].Name {
					t.Errorf("Expected presubmits[%d].Name to be '%v', got '%v'", i, tc.expected.prowYAML.Presubmits[i].Name, prowYAML.Presubmits[i].Name)
				}
			}
		})
	}
}
