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
	"testing"

	lru "github.com/hashicorp/golang-lru"
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

func TestGetFromCache(t *testing.T) {
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

	// simpleCache is a cache only used for testing. The difference between this
	// cache and the ones in ProwYAMLCache is that simpleCache only holds
	// strings, not []Presubmit or []Postsubmit as values.
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
			cacheInitialState: map[CacheKey]string{
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
			cacheInitialState: map[CacheKey]string{
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
			cacheInitialState: map[CacheKey]string{
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
	fakePresubmitsMap := make(map[CacheKey][]Presubmit)
	// This mocks GetPresubmits. Instead of using the git.ClientFactory
	// (and other operations), we just use a simple map to get the
	// []Presubmit value we want. For simplicity we just reuse
	// MakeCacheKey even though we're not using a cache. The point of
	// fakeGetPresubmits is to act as a "source of truth" of authoritative
	// []Presubmit values for purposes of the test cases in this unit
	// test.
	goodValConstructor := func(gc git.ClientFactory, identifier string, baseSHAGetter RefGetter, headSHAGetters ...RefGetter) ([]Presubmit, error) {

		keyParts, err := MakeCacheKeyParts(identifier, baseSHAGetter, headSHAGetters...)
		if err != nil {
			t.Fatal(err)
		}

		key, err := MakeCacheKey(keyParts)
		if err != nil {
			t.Fatal(err)
		}

		val, ok := fakePresubmitsMap[key]
		if ok {
			return val, nil
		}

		return nil, fmt.Errorf("unable to construct []Presubmit value")
	}
	fakePresubmits := []CacheKeyParts{
		{
			Identifier: "foo/bar",
			BaseSHA:    "ba5e",
			HeadSHAs:   []string{"abcd", "ef01"},
		},
	}
	// Populate config's Presubmits.
	for _, fakePresubmit := range fakePresubmits {
		// To make it easier to compare Presubmit values, we only set the
		// Name field and only compare this field. We also only create a
		// single Presubmit (singleton slice), again for simplicity. Lastly
		// we also set the Name field to the same value as the "key", again
		// for simplicity.
		fakePresubmitKey, err := MakeCacheKey(fakePresubmit)
		if err != nil {
			t.Fatal(err)
		}
		fakePresubmitsMap[fakePresubmitKey] = []Presubmit{
			{
				JobBase: JobBase{Name: string(fakePresubmitKey)},
			},
		}
	}

	badValConstructor := func(gc git.ClientFactory, identifier string, baseSHAGetter RefGetter, headSHAGetters ...RefGetter) ([]Presubmit, error) {
		return nil, fmt.Errorf("unable to construct []Presubmit value")
	}

	prowYAMLCache, err := NewProwYAMLCache(1)
	if err != nil {
		t.Fatal("could not initialize prowYAMLCache")
	}

	type expected struct {
		presubmits []Presubmit
		cacheHit   bool
		evicted    bool
		err        string
	}

	for _, tc := range []struct {
		name           string
		valConstructor func(git.ClientFactory, string, RefGetter, ...RefGetter) ([]Presubmit, error)
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
				presubmits: []Presubmit{
					{
						JobBase: JobBase{Name: `{"identifier":"foo/bar","baseSHA":"ba5e","headSHAs":["abcd","ef01"]}`}},
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
				presubmits: []Presubmit{
					{
						JobBase: JobBase{Name: `{"identifier":"foo/bar","baseSHA":"ba5e","headSHAs":["abcd","ef01"]}`},
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
			headSHAGetters: []RefGetter{
				goodSHAGetter("abcd"),
				goodSHAGetter("ef01")},
			expected: expected{
				presubmits: nil,
				cacheHit:   false,
				evicted:    false,
				err:        "unable to construct []Presubmit value",
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
				presubmits: []Presubmit{
					{
						JobBase: JobBase{Name: `{"identifier":"foo/bar","baseSHA":"ba5e","headSHAs":["abcd","ef01"]}`}},
				},
				cacheHit: true,
				evicted:  false,
				err:      "",
			},
		},
		{
			// If the cache is corrupted (it holds values of a type that is not
			// []Presubmit), then we expect to reconstruct the value from
			// scratch.
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
				presubmits: []Presubmit{
					{
						JobBase: JobBase{Name: `{"identifier":"foo/bar","baseSHA":"ba5e","headSHAs":["abcd","ef01"]}`}},
				},
				cacheHit: false,
				evicted:  false,
				err:      "",
			},
		},
		{
			// If the cache is corrupted (it holds values of a type that is not
			// []Presubmit), then we expect to reconstruct the value from
			// scratch. But a faulty value constructor will result in a "nil"
			// presubmits value.
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
				presubmits: nil,
				cacheHit:   false,
				evicted:    false,
				err:        "unable to construct []Presubmit value",
			},
		},
	} {
		t.Run(tc.name, func(t1 *testing.T) {
			// Reset test state.
			prowYAMLCache.presubmits.Purge()

			for _, kp := range tc.cacheInitialState {
				k, err := MakeCacheKey(kp)
				if err != nil {
					t.Errorf("Expected error 'nil' got '%v'", err.Error())
				}
				_ = prowYAMLCache.presubmits.Add(k, []Presubmit{
					{
						JobBase: JobBase{Name: string(k)}},
				})
			}

			// Simulate storing a value of the wrong type in the cache (a string
			// instead of a []Presubmit).
			if tc.cacheCorrupted {
				prowYAMLCache.presubmits.Purge()

				for _, kp := range tc.cacheInitialState {
					k, err := MakeCacheKey(kp)
					if err != nil {
						t.Errorf("Expected error 'nil' got '%v'", err.Error())
					}
					_ = prowYAMLCache.presubmits.Add(k, "<wrong-type>")
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
