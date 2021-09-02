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

package subscriber

import (
	"fmt"
	"testing"

	lru "github.com/hashicorp/golang-lru"
	"k8s.io/test-infra/prow/config"
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
	valids := []int{1, 5}
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
		cacheKey CacheKey
		err      string
	}

	for _, tc := range []struct {
		name           string
		identifier     string
		baseSHAGetter  config.RefGetter
		headSHAGetters []config.RefGetter
		expected       expected
	}{
		{
			name:          "Basic",
			identifier:    "foo/bar",
			baseSHAGetter: goodSHAGetter("ba5e"),
			headSHAGetters: []config.RefGetter{
				goodSHAGetter("abcd"),
				goodSHAGetter("ef01")},
			expected: expected{
				CacheKey{
					identifier: "foo/bar",
					baseSHA:    "ba5e",
					headSHAs:   []string{"abcd", "ef01"},
				},
				"",
			},
		},
		{
			name:           "NoHeadSHAGetters",
			identifier:     "foo/bar",
			baseSHAGetter:  goodSHAGetter("ba5e"),
			headSHAGetters: []config.RefGetter{},
			expected: expected{
				CacheKey{
					identifier: "foo/bar",
					baseSHA:    "ba5e",
					headSHAs:   []string{},
				},
				"",
			},
		},
		{
			name:          "EmptyIdentifierFailure",
			identifier:    "",
			baseSHAGetter: goodSHAGetter("ba5e"),
			headSHAGetters: []config.RefGetter{
				goodSHAGetter("abcd"),
				goodSHAGetter("ef01")},
			expected: expected{
				CacheKey{},
				"identifier cannot be empty",
			},
		},
		{
			name:          "BaseSHAGetterFailure",
			identifier:    "foo/bar",
			baseSHAGetter: badSHAGetter,
			headSHAGetters: []config.RefGetter{
				goodSHAGetter("abcd"),
				goodSHAGetter("ef01")},
			expected: expected{
				CacheKey{},
				"failed to get baseSHA: badSHAGetter",
			},
		},
		{
			name:          "HeadSHAGetterFailure",
			identifier:    "foo/bar",
			baseSHAGetter: goodSHAGetter("ba5e"),
			headSHAGetters: []config.RefGetter{
				goodSHAGetter("abcd"),
				badSHAGetter},
			expected: expected{
				CacheKey{},
				"failed to get headRef: badSHAGetter",
			},
		},
	} {
		t.Run(tc.name, func(t1 *testing.T) {
			cacheKey, err := MakeCacheKey(tc.identifier, tc.baseSHAGetter, tc.headSHAGetters...)

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

			if tc.expected.cacheKey.identifier != cacheKey.identifier {
				t.Errorf("Expected CacheKey identifier '%v', got '%v'", tc.expected.cacheKey.identifier, cacheKey.identifier)
			}

			if tc.expected.cacheKey.baseSHA != cacheKey.baseSHA {
				t.Errorf("Expected CacheKey baseSHA '%v', got '%v'", tc.expected.cacheKey.baseSHA, cacheKey.baseSHA)
			}

			if len(tc.expected.cacheKey.headSHAs) != len(cacheKey.headSHAs) {
				t.Errorf("Expected CacheKey length '%d', got '%d'", len(tc.expected.cacheKey.headSHAs), len(cacheKey.headSHAs))
			}

			if len(tc.expected.cacheKey.headSHAs) > 0 {
				for i := range tc.expected.cacheKey.headSHAs {
					if tc.expected.cacheKey.headSHAs[i] != cacheKey.headSHAs[i] {
						t.Errorf("Expected CacheKey headSHAs[%d] to be '%v', got '%v'", i, tc.expected.cacheKey.headSHAs[i], cacheKey.headSHAs[i])
					}
				}
			}

		})
	}
}

type simpleKey string

func (k simpleKey) String() string {
	return string(k)
}

func TestGetFromCache(t *testing.T) {
	keyConstructorCalls := 0
	goodKeyConstructor := func(key string) func() (fmt.Stringer, error) {
		return func() (fmt.Stringer, error) {
			keyConstructorCalls++
			return simpleKey("(key)" + key), nil
		}
	}
	badKeyConstructor := func(key string) func() (fmt.Stringer, error) {
		return func() (fmt.Stringer, error) {
			keyConstructorCalls++
			return simpleKey(""), fmt.Errorf("could not construct key")
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

	// simpleCache is a cache only used for testing. The difference between this
	// cache and the ones in ProwYAMLCache is that simpleCache only holds
	// strings, not []config.Presubmit or []config.Postsubmit as values.
	simpleCache, err := lru.New(2)
	if err != nil {
		t.Error("could not initialize simpleCache")
	}

	type expected struct {
		val                 string
		cacheHit            bool
		evicted             bool
		err                 string
		keyConstructorCalls int
		valConstructorCalls int
		cachedValues        int
	}

	for _, tc := range []struct {
		name              string
		cache             *lru.Cache
		cacheInitialState map[string]string
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
				cacheHit:            false,
				evicted:             false,
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
				cacheHit:            false,
				evicted:             false,
				err:                 "",
				keyConstructorCalls: 1,
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
			keyConstructor: goodKeyConstructor("bar"),
			valConstructor: goodValConstructor("bar"),
			expected: expected{
				val:                 "(val)bar",
				cacheHit:            false,
				evicted:             false,
				err:                 "",
				keyConstructorCalls: 1,
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
			keyConstructor: goodKeyConstructor("cat"),
			valConstructor: goodValConstructor("cat"),
			expected: expected{
				val:                 "(val)cat",
				cacheHit:            false,
				evicted:             true,
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
			cacheInitialState: map[string]string{
				"(key)foo": "(val)foo",
				"(key)bar": "(val)bar",
			},
			keyConstructor: goodKeyConstructor("bar"),
			valConstructor: goodValConstructor("bar"),
			expected: expected{
				val:                 "(val)bar",
				cacheHit:            true,
				evicted:             false,
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
				cacheHit:            false,
				evicted:             false,
				err:                 "could not construct key",
				keyConstructorCalls: 1,
				valConstructorCalls: 0,
				cachedValues:        0,
			},
		},
		{
			name:              "BadValConstructor",
			cache:             simpleCache,
			cacheInitialState: nil,
			keyConstructor:    goodKeyConstructor("bar"),
			valConstructor:    badValConstructor("bar"),
			expected: expected{
				val:                 "<nil>",
				cacheHit:            false,
				evicted:             false,
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
				cacheHit:            false,
				evicted:             false,
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
					_ = tc.cache.Add(k, v)
				}
			}

			val, cacheHit, evicted, err := GetFromCache(tc.cache, tc.keyConstructor, tc.valConstructor)

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

			if tc.expected.cacheHit != cacheHit {
				t.Errorf("Expected cache hit to be '%t', got '%t'", tc.expected.cacheHit, cacheHit)
			}

			if tc.expected.evicted != evicted {
				t.Errorf("Expected evicted to be '%t', got '%t'", tc.expected.evicted, evicted)
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

func TestGetPresubmitsFromCache(t *testing.T) {
	fakePresubmitsMap := make(map[string][]config.Presubmit)
	// This mocks config.GetPresubmits. Instead of using the git.ClientFactory
	// (and other operations), we just use a simple map to get the
	// []config.Presubmit value we want. For simplicity we just reuse
	// MakeCacheKey even though we're not using a cache. The point of
	// fakeGetPresubmits is to act as a "source of truth" of authoritative
	// []config.Presubmit values for purposes of the test cases in this unit
	// test.
	goodValConstructor := func(gc git.ClientFactory, identifier string, baseSHAGetter config.RefGetter, headSHAGetters ...config.RefGetter) ([]config.Presubmit, error) {

		key, err := MakeCacheKey(identifier, baseSHAGetter, headSHAGetters...)
		if err != nil {
			t.Fatal(err)
		}

		val, ok := fakePresubmitsMap[key.String()]
		if ok {
			return val, nil
		}

		return nil, fmt.Errorf("unable to construct []config.Presubmit value")
	}
	fakePresubmits := []CacheKey{
		{
			identifier: "foo/bar",
			baseSHA:    "ba5e",
			headSHAs:   []string{"abcd", "ef01"},
		},
	}
	// Populate config's Presubmits.
	fakePresubmitsPopulator := func(fakeItems []CacheKey, fakeItemsMap map[string][]config.Presubmit) {

		for _, fakeItem := range fakeItems {
			// To make it easier to compare Presubmit values, we only set the
			// Name field and only compare this field. We also only create a
			// single Presubmit (singleton slice), again for simplicity. Lastly
			// we also set the Name field to the same value as the "key", again
			// for simplicity.
			fakeItemsMap[fakeItem.String()] = []config.Presubmit{
				{
					JobBase: config.JobBase{Name: fakeItem.String()},
				},
			}
		}
	}

	fakePresubmitsPopulator(fakePresubmits, fakePresubmitsMap)

	badValConstructor := func(gc git.ClientFactory, identifier string, baseSHAGetter config.RefGetter, headSHAGetters ...config.RefGetter) ([]config.Presubmit, error) {
		return nil, fmt.Errorf("unable to construct []config.Presubmit value")
	}

	prowYAMLCache, err := NewProwYAMLCache(1)
	if err != nil {
		t.Fatal("could not initialize prowYAMLCache")
	}

	type expected struct {
		presubmits []config.Presubmit
		cacheHit   bool
		evicted    bool
		err        string
	}

	for _, tc := range []struct {
		name           string
		valConstructor func(git.ClientFactory, string, config.RefGetter, ...config.RefGetter) ([]config.Presubmit, error)
		// We use a slice of CacheKeys for simplicity.
		cacheInitialState []CacheKey
		cacheCorrupted    bool
		identifier        string
		baseSHAGetter     config.RefGetter
		headSHAGetters    []config.RefGetter
		expected          expected
	}{
		{
			name:              "CacheMiss",
			valConstructor:    goodValConstructor,
			cacheInitialState: nil,
			cacheCorrupted:    false,
			identifier:        "foo/bar",
			baseSHAGetter:     goodSHAGetter("ba5e"),
			headSHAGetters: []config.RefGetter{
				goodSHAGetter("abcd"),
				goodSHAGetter("ef01")},
			expected: expected{
				presubmits: []config.Presubmit{
					{
						JobBase: config.JobBase{Name: "identifier:foo/bar,baseSHA:ba5e,headSHA:abcd,headSHA:ef01"}},
				},
				cacheHit: false,
				evicted:  false,
				err:      "",
			},
		},
		{
			// If we get a cache hit, the value constructor function doesn't
			// matter because it will never be called.
			name:           "CacheHit",
			valConstructor: badValConstructor,
			cacheInitialState: []CacheKey{
				{
					identifier: "foo/bar",
					baseSHA:    "ba5e",
					headSHAs:   []string{"abcd", "ef01"},
				},
			},
			cacheCorrupted: false,
			identifier:     "foo/bar",
			baseSHAGetter:  goodSHAGetter("ba5e"),
			headSHAGetters: []config.RefGetter{
				goodSHAGetter("abcd"),
				goodSHAGetter("ef01")},
			expected: expected{
				presubmits: []config.Presubmit{
					{
						JobBase: config.JobBase{Name: "identifier:foo/bar,baseSHA:ba5e,headSHA:abcd,headSHA:ef01"},
					},
				},
				cacheHit: true,
				evicted:  false,
				err:      "",
			},
		},
		{
			name:              "BadValConstructorCacheMiss",
			valConstructor:    badValConstructor,
			cacheInitialState: nil,
			cacheCorrupted:    false,
			identifier:        "foo/bar",
			baseSHAGetter:     goodSHAGetter("ba5e"),
			headSHAGetters: []config.RefGetter{
				goodSHAGetter("abcd"),
				goodSHAGetter("ef01")},
			expected: expected{
				presubmits: nil,
				cacheHit:   false,
				evicted:    false,
				err:        "unable to construct []config.Presubmit value",
			},
		},
		{
			// If we get a cache hit, then it doesn't matter if the state of the
			// world was such that the value could not have been constructed from
			// scratch (because we're solely relying on the cache).
			name:           "BadValConstructorCacheHit",
			valConstructor: badValConstructor,
			cacheInitialState: []CacheKey{
				{
					identifier: "foo/bar",
					baseSHA:    "ba5e",
					headSHAs:   []string{"abcd", "ef01"},
				},
			},
			cacheCorrupted: false,
			identifier:     "foo/bar",
			baseSHAGetter:  goodSHAGetter("ba5e"),
			headSHAGetters: []config.RefGetter{
				goodSHAGetter("abcd"),
				goodSHAGetter("ef01")},
			expected: expected{
				presubmits: []config.Presubmit{
					{
						JobBase: config.JobBase{Name: "identifier:foo/bar,baseSHA:ba5e,headSHA:abcd,headSHA:ef01"}},
				},
				cacheHit: true,
				evicted:  false,
				err:      "",
			},
		},
		{
			// If the cache is corrupted (it holds values of a type that is not
			// []config.Presubmit), then we expect to reconstruct the value from
			// scratch.
			name:           "GoodValConstructorCorruptedCacheHit",
			valConstructor: goodValConstructor,
			cacheInitialState: []CacheKey{
				{
					identifier: "foo/bar",
					baseSHA:    "ba5e",
					headSHAs:   []string{"abcd", "ef01"},
				},
			},
			cacheCorrupted: true,
			identifier:     "foo/bar",
			baseSHAGetter:  goodSHAGetter("ba5e"),
			headSHAGetters: []config.RefGetter{
				goodSHAGetter("abcd"),
				goodSHAGetter("ef01")},
			expected: expected{
				presubmits: []config.Presubmit{
					{
						JobBase: config.JobBase{Name: "identifier:foo/bar,baseSHA:ba5e,headSHA:abcd,headSHA:ef01"}},
				},
				cacheHit: false,
				evicted:  false,
				err:      "",
			},
		},
		{
			// If the cache is corrupted (it holds values of a type that is not
			// []config.Presubmit), then we expect to reconstruct the value from
			// scratch. But a faulty value constructor will result in a "nil"
			// presubmits value.
			name:           "BadValConstructorCorruptedCacheHit",
			valConstructor: badValConstructor,
			cacheInitialState: []CacheKey{
				{
					identifier: "foo/bar",
					baseSHA:    "ba5e",
					headSHAs:   []string{"abcd", "ef01"},
				},
			},
			cacheCorrupted: true,
			identifier:     "foo/bar",
			baseSHAGetter:  goodSHAGetter("ba5e"),
			headSHAGetters: []config.RefGetter{
				goodSHAGetter("abcd"),
				goodSHAGetter("ef01")},
			expected: expected{
				presubmits: nil,
				cacheHit:   false,
				evicted:    false,
				err:        "unable to construct []config.Presubmit value",
			},
		},
	} {
		t.Run(tc.name, func(t1 *testing.T) {
			// Reset test state.
			prowYAMLCache.presubmits.Purge()

			for _, k := range tc.cacheInitialState {
				_ = prowYAMLCache.presubmits.Add(k.String(), []config.Presubmit{
					{
						JobBase: config.JobBase{Name: k.String()}},
				})
			}

			// Simulate storing a value of the wrong type in the cache (a string
			// instead of a []config.Presubmit).
			if tc.cacheCorrupted {
				prowYAMLCache.presubmits.Purge()

				for _, k := range tc.cacheInitialState {
					_ = prowYAMLCache.presubmits.Add(k.String(), "<wrong-type>")
				}
			}

			presubmits, cacheHit, evicted, err := prowYAMLCache.GetPresubmitsFromCache(tc.valConstructor, nil, tc.identifier, tc.baseSHAGetter, tc.headSHAGetters...)

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

			// The Presubmit type is not comparable. So instead of checking the
			// overall type for equality, we only check the Name field of it,
			// because it is a simple string type.
			if len(tc.expected.presubmits) != len(presubmits) {
				t.Fatalf("Expected presubmits length '%d', got '%d'", len(tc.expected.presubmits), len(presubmits))
			}
			for i := range tc.expected.presubmits {
				if tc.expected.presubmits[i].Name != presubmits[i].Name {
					t.Errorf("Expected presubmits[%d].Name to be '%v', got '%v'", i, tc.expected.presubmits[i].Name, presubmits[i].Name)
				}
			}

			if tc.expected.cacheHit != cacheHit {
				t.Errorf("Expected cache hit to be '%t', got '%t'", tc.expected.cacheHit, cacheHit)
			}

			if tc.expected.evicted != evicted {
				t.Errorf("Expected evicted to be '%t', got '%t'", tc.expected.evicted, evicted)
			}
		})
	}
}

// TestGetPostsubmitsFromCache is virtually identical to
// TestGetPresubmitsFromCache. The main difference is that the headSHAGetters
// are unused in the test cases.
func TestGetPostsubmitsFromCache(t *testing.T) {
	fakePostsubmitsMap := make(map[string][]config.Postsubmit)
	goodValConstructor := func(gc git.ClientFactory, identifier string, baseSHAGetter config.RefGetter, headSHAGetters ...config.RefGetter) ([]config.Postsubmit, error) {

		key, err := MakeCacheKey(identifier, baseSHAGetter, headSHAGetters...)
		if err != nil {
			t.Fatal(err)
		}

		val, ok := fakePostsubmitsMap[key.String()]
		if ok {
			return val, nil
		}

		return nil, fmt.Errorf("unable to construct []config.Postsubmit value")
	}
	fakePostsubmits := []CacheKey{
		{
			identifier: "foo/bar",
			baseSHA:    "ba5e",
		},
	}
	// Populate config's Postsubmits.
	fakePostsubmitsPopulator := func(fakeItems []CacheKey, fakeItemsMap map[string][]config.Postsubmit) {

		for _, fakeItem := range fakeItems {
			fakeItemsMap[fakeItem.String()] = []config.Postsubmit{
				{
					JobBase: config.JobBase{Name: fakeItem.String()},
				},
			}
		}
	}

	fakePostsubmitsPopulator(fakePostsubmits, fakePostsubmitsMap)

	badValConstructor := func(gc git.ClientFactory, identifier string, baseSHAGetter config.RefGetter, headSHAGetters ...config.RefGetter) ([]config.Postsubmit, error) {
		return nil, fmt.Errorf("unable to construct []config.Postsubmit value")
	}

	prowYAMLCache, err := NewProwYAMLCache(1)
	if err != nil {
		t.Fatal("could not initialize prowYAMLCache")
	}

	type expected struct {
		postsubmits []config.Postsubmit
		cacheHit    bool
		evicted     bool
		err         string
	}

	for _, tc := range []struct {
		name           string
		valConstructor func(git.ClientFactory, string, config.RefGetter, ...config.RefGetter) ([]config.Postsubmit, error)
		// We use a slice of CacheKeys for simplicity.
		cacheInitialState []CacheKey
		cacheCorrupted    bool
		identifier        string
		baseSHAGetter     config.RefGetter
		expected          expected
	}{
		{
			name:              "CacheMiss",
			valConstructor:    goodValConstructor,
			cacheInitialState: nil,
			cacheCorrupted:    false,
			identifier:        "foo/bar",
			baseSHAGetter:     goodSHAGetter("ba5e"),
			expected: expected{
				postsubmits: []config.Postsubmit{
					{
						JobBase: config.JobBase{Name: "identifier:foo/bar,baseSHA:ba5e"},
					},
				},
				cacheHit: false,
				evicted:  false,
				err:      "",
			},
		},
		{
			// If we get a cache hit, the value constructor function doesn't
			// matter because it will never be called.
			name:           "CacheHit",
			valConstructor: badValConstructor,
			cacheInitialState: []CacheKey{
				{
					identifier: "foo/bar",
					baseSHA:    "ba5e",
				},
			},
			cacheCorrupted: false,
			identifier:     "foo/bar",
			baseSHAGetter:  goodSHAGetter("ba5e"),
			expected: expected{
				postsubmits: []config.Postsubmit{
					{
						JobBase: config.JobBase{Name: "identifier:foo/bar,baseSHA:ba5e"},
					},
				},
				cacheHit: true,
				evicted:  false,
				err:      "",
			},
		},
		{
			name:              "BadValConstructorCacheMiss",
			valConstructor:    badValConstructor,
			cacheInitialState: nil,
			cacheCorrupted:    false,
			identifier:        "foo/bar",
			baseSHAGetter:     goodSHAGetter("ba5e"),
			expected: expected{
				postsubmits: nil,
				cacheHit:    false,
				evicted:     false,
				err:         "unable to construct []config.Postsubmit value",
			},
		},
		{
			// If we get a cache hit, then it doesn't matter if the state of the
			// world was such that the value could not have been constructed from
			// scratch (because we're solely relying on the cache).
			name:           "BadValConstructorCacheHit",
			valConstructor: badValConstructor,
			cacheInitialState: []CacheKey{
				{
					identifier: "foo/bar",
					baseSHA:    "ba5e",
				},
			},
			cacheCorrupted: false,
			identifier:     "foo/bar",
			baseSHAGetter:  goodSHAGetter("ba5e"),
			expected: expected{
				postsubmits: []config.Postsubmit{
					{
						JobBase: config.JobBase{Name: "identifier:foo/bar,baseSHA:ba5e"},
					},
				},
				cacheHit: true,
				evicted:  false,
				err:      "",
			},
		},
		{
			// If the cache is corrupted (it holds values of a type that is not
			// []config.Postsubmit), then we expect to reconstruct the value from
			// scratch.
			name:           "GoodValConstructorCorruptedCacheHit",
			valConstructor: goodValConstructor,
			cacheInitialState: []CacheKey{
				{
					identifier: "foo/bar",
					baseSHA:    "ba5e",
				},
			},
			cacheCorrupted: true,
			identifier:     "foo/bar",
			baseSHAGetter:  goodSHAGetter("ba5e"),
			expected: expected{
				postsubmits: []config.Postsubmit{
					{
						JobBase: config.JobBase{Name: "identifier:foo/bar,baseSHA:ba5e"},
					},
				},
				cacheHit: false,
				evicted:  false,
				err:      "",
			},
		},
		{
			// If the cache is corrupted (it holds values of a type that is not
			// []config.Postsubmit), then we expect to reconstruct the value from
			// scratch. But a faulty value constructor will result in a "nil"
			// postsubmits value.
			name:           "BadValConstructorCorruptedCacheHit",
			valConstructor: badValConstructor,
			cacheInitialState: []CacheKey{
				{
					identifier: "foo/bar",
					baseSHA:    "ba5e",
					headSHAs:   []string{"abcd", "ef01"},
				},
			},
			cacheCorrupted: true,
			identifier:     "foo/bar",
			baseSHAGetter:  goodSHAGetter("ba5e"),
			expected: expected{
				postsubmits: nil,
				cacheHit:    false,
				evicted:     false,
				err:         "unable to construct []config.Postsubmit value",
			},
		},
	} {
		t.Run(tc.name, func(t1 *testing.T) {
			// Reset test state.
			prowYAMLCache.postsubmits.Purge()

			for _, k := range tc.cacheInitialState {
				_ = prowYAMLCache.postsubmits.Add(k.String(), []config.Postsubmit{
					{
						JobBase: config.JobBase{Name: k.String()},
					}})
			}

			// Simulate storing a value of the wrong type in the cache (a string
			// instead of a []config.Postsubmit).
			if tc.cacheCorrupted {
				prowYAMLCache.postsubmits.Purge()

				for _, k := range tc.cacheInitialState {
					_ = prowYAMLCache.postsubmits.Add(k.String(), "<wrong-type>")
				}
			}

			postsubmits, cacheHit, evicted, err := prowYAMLCache.GetPostsubmitsFromCache(tc.valConstructor, nil, tc.identifier, tc.baseSHAGetter)

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

			// The Postsubmit type is not comparable. So instead of checking the
			// overall type for equality, we only check the Name field of it,
			// because it is a simple string type.
			if len(tc.expected.postsubmits) != len(postsubmits) {
				t.Fatalf("Expected postsubmits length '%d', got '%d'", len(tc.expected.postsubmits), len(postsubmits))
			}
			for i := range tc.expected.postsubmits {
				if tc.expected.postsubmits[i].Name != postsubmits[i].Name {
					t.Errorf("Expected postsubmits[%d].Name to be '%v', got '%v'", i, tc.expected.postsubmits[i].Name, postsubmits[i].Name)
				}
			}

			if tc.expected.cacheHit != cacheHit {
				t.Errorf("Expected cache hit to be '%t', got '%t'", tc.expected.cacheHit, cacheHit)
			}

			if tc.expected.evicted != evicted {
				t.Errorf("Expected evicted to be '%t', got '%t'", tc.expected.evicted, evicted)
			}
		})
	}
}
