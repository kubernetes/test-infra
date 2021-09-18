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
	"sync"
	"testing"

	"k8s.io/test-infra/prow/git/v2"
)

type fakeConfigAgent struct {
	sync.Mutex
	c *Config
}

func (f *fakeConfigAgent) Config() *Config {
	f.Lock()
	defer f.Unlock()
	return f.c
}

func TestNewProwYAMLCache(t *testing.T) {
	// Invalid size arguments result in a nil prowYAMLCache and non-nil error.
	invalids := []int{-1, 0}
	for _, invalid := range invalids {

		fca := &fakeConfigAgent{}
		cf := &testClientFactory{}
		prowYAMLCache, err := NewProwYAMLCache(invalid, fca, cf)

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

		fca := &fakeConfigAgent{}
		cf := &testClientFactory{}
		prowYAMLCache, err := NewProwYAMLCache(valid, fca, cf)

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

func TestGetProwYAMLCached(t *testing.T) {
	// fakeProwYAMLGetter mocks prowYAMLGetter(). Instead of using the
	// git.ClientFactory (and other operations), we just use a simple map to get
	// the *ProwYAML value we want. For simplicity we just reuse MakeCacheKey
	// even though we're not using a cache. The point of fakeProwYAMLGetter is to
	// act as a "source of truth" of authoritative *ProwYAML values for purposes
	// of the test cases in this unit test.
	fakeProwYAMLGetter := make(map[CacheKey]*ProwYAML)

	// goodValConstructor mocks config.getProwYAML.
	// This map pretends to be an expensive computation in order to generate a
	// *ProwYAML value.
	goodValConstructor := func(gc git.ClientFactory, identifier string, baseSHAGetter RefGetter, headSHAGetters ...RefGetter) (*ProwYAML, error) {

		baseSHA, headSHAs, err := GetAndCheckRefs(baseSHAGetter, headSHAGetters...)
		if err != nil {
			t.Fatal(err)
		}

		key, err := MakeCacheKey(identifier, baseSHA, headSHAs)
		if err != nil {
			t.Fatal(err)
		}

		val, ok := fakeProwYAMLGetter[key]
		if ok {
			return val, nil
		}

		return nil, fmt.Errorf("unable to construct *ProwYAML value")
	}

	fakeCacheKeyPartsSlice := []CacheKeyParts{
		{
			Identifier: "foo/bar",
			BaseSHA:    "ba5e",
			HeadSHAs:   []string{"abcd", "ef01"},
		},
	}
	// Populate fakeProwYAMLGetter.
	for _, fakeCacheKeyParts := range fakeCacheKeyPartsSlice {
		// To make it easier to compare Presubmit values, we only set the
		// Name field and only compare this field. We also only create a
		// single Presubmit (singleton slice), again for simplicity. Lastly
		// we also set the Name field to the same value as the "key", again
		// for simplicity.
		fakeCacheKey, err := fakeCacheKeyParts.CacheKey()
		if err != nil {
			t.Fatal(err)
		}
		fakeProwYAMLGetter[fakeCacheKey] = &ProwYAML{
			Presubmits: []Presubmit{
				{
					JobBase: JobBase{Name: string(fakeCacheKey)},
				},
			},
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

	type expected struct {
		prowYAML *ProwYAML
		cacheLen int
		err      string
	}

	for _, tc := range []struct {
		name           string
		valConstructor func(git.ClientFactory, string, RefGetter, ...RefGetter) (*ProwYAML, error)
		// We use a slice of CacheKeysParts for simplicity.
		cacheInitialState   []CacheKeyParts
		cacheCorrupted      bool
		inRepoConfigEnabled bool
		identifier          string
		baseSHAGetter       RefGetter
		headSHAGetters      []RefGetter
		expected            expected
	}{
		{
			name:                "CacheMiss",
			valConstructor:      goodValConstructor,
			cacheInitialState:   nil,
			cacheCorrupted:      false,
			inRepoConfigEnabled: true,
			identifier:          "foo/bar",
			baseSHAGetter:       goodSHAGetter("ba5e"),
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
				cacheLen: 1,
				err:      "",
			},
		},
		{
			// If the InRepoConfig is disabled for this repo, then the returned
			// value should be an empty &ProwYAML{}. Also, the cache miss should
			// not result in adding this entry into the cache (because the value
			// will be a meaninless empty &ProwYAML{}).
			name:                "CacheMiss/InRepoConfigDisabled",
			valConstructor:      goodValConstructor,
			cacheInitialState:   nil,
			cacheCorrupted:      false,
			inRepoConfigEnabled: false,
			identifier:          "foo/bar",
			baseSHAGetter:       goodSHAGetter("ba5e"),
			headSHAGetters: []RefGetter{
				goodSHAGetter("abcd"),
				goodSHAGetter("ef01")},
			expected: expected{
				prowYAML: &ProwYAML{},
				cacheLen: 0,
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
			cacheCorrupted:      false,
			inRepoConfigEnabled: true,
			identifier:          "foo/bar",
			baseSHAGetter:       goodSHAGetter("ba5e"),
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
				cacheLen: 1,
				err:      "",
			},
		},
		{
			name:                "BadValConstructorCacheMiss",
			valConstructor:      badValConstructor,
			cacheInitialState:   nil,
			cacheCorrupted:      false,
			inRepoConfigEnabled: true,
			identifier:          "foo/bar",
			baseSHAGetter:       goodSHAGetter("ba5e"),
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
			cacheCorrupted:      false,
			inRepoConfigEnabled: true,
			identifier:          "foo/bar",
			baseSHAGetter:       goodSHAGetter("ba5e"),
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
				cacheLen: 1,
				err:      "",
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
			cacheCorrupted:      true,
			inRepoConfigEnabled: true,
			identifier:          "foo/bar",
			baseSHAGetter:       goodSHAGetter("ba5e"),
			headSHAGetters: []RefGetter{
				goodSHAGetter("abcd"),
				goodSHAGetter("ef01")},
			expected: expected{
				prowYAML: nil,
				err:      "Programmer error: expected value type '*config.ProwYAML', got 'string'",
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
			cacheCorrupted:      true,
			inRepoConfigEnabled: true,
			identifier:          "foo/bar",
			baseSHAGetter:       goodSHAGetter("ba5e"),
			headSHAGetters: []RefGetter{
				goodSHAGetter("abcd"),
				goodSHAGetter("ef01")},
			expected: expected{
				prowYAML: nil,
				err:      "Programmer error: expected value type '*config.ProwYAML', got 'string'",
			},
		},
	} {
		t.Run(tc.name, func(t1 *testing.T) {
			// Reset test state.
			maybeEnabled := make(map[string]*bool)
			maybeEnabled[tc.identifier] = &tc.inRepoConfigEnabled

			fca := &fakeConfigAgent{
				c: &Config{
					ProwConfig: ProwConfig{
						InRepoConfig: InRepoConfig{
							Enabled: maybeEnabled,
						},
					},
				},
			}
			cf := &testClientFactory{}
			prowYAMLCache, err := NewProwYAMLCache(1, fca, cf)
			if err != nil {
				t.Fatal("could not initialize prowYAMLCache")
			}

			for _, kp := range tc.cacheInitialState {
				k, err := kp.CacheKey()
				if err != nil {
					t.Errorf("Expected error 'nil' got '%v'", err.Error())
				}
				_, _ = prowYAMLCache.GetOrAdd(k, goodValConstructorForInitialState(ProwYAML{
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
					k, err := kp.CacheKey()
					if err != nil {
						t.Errorf("Expected error 'nil' got '%v'", err.Error())
					}
					_, _ = prowYAMLCache.GetOrAdd(k, func() (interface{}, error) { return "<wrong-type>", nil })
				}
			}

			prowYAML, err := prowYAMLCache.GetProwYAML(tc.valConstructor, tc.identifier, tc.baseSHAGetter, tc.headSHAGetters...)

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

			if tc.expected.cacheLen != prowYAMLCache.Len() {
				t.Errorf("Expected '%d' cached elements, got '%d'", tc.expected.cacheLen, prowYAMLCache.Len())
			}
		})
	}
}
