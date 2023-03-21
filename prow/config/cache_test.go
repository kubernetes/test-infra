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

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/sync/errgroup"
	v1 "k8s.io/api/core/v1"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/git/v2"
)

type fakeConfigAgent struct {
	sync.Mutex
	c *Config
}

func (fca *fakeConfigAgent) Config() *Config {
	fca.Lock()
	defer fca.Unlock()
	return fca.c
}

func TestNewInRepoConfigCache(t *testing.T) {
	// Invalid size arguments result in a nil cache and non-nil error.
	invalids := []int{-1, 0}
	for _, invalid := range invalids {

		fca := &fakeConfigAgent{}
		cf := &testClientFactory{}
		cache, err := NewInRepoConfigCache(invalid, fca, cf)

		if err == nil {
			t.Fatal("Expected non-nil error, got nil")
		}

		if err.Error() != "Must provide a positive size" {
			t.Errorf("Expected error 'Must provide a positive size', got '%v'", err.Error())
		}

		if cache != nil {
			t.Errorf("Expected nil cache, got %v", cache)
		}
	}

	// Valid size arguments.
	valids := []int{1, 5, 1000}
	for _, valid := range valids {

		fca := &fakeConfigAgent{}
		cf := &testClientFactory{}
		cache, err := NewInRepoConfigCache(valid, fca, cf)

		if err != nil {
			t.Errorf("Expected error 'nil' got '%v'", err.Error())
		}

		if cache == nil {
			t.Errorf("Expected non-nil cache, got nil")
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

		kp := CacheKeyParts{
			Identifier: identifier,
			BaseSHA:    baseSHA,
			HeadSHAs:   headSHAs,
		}
		key, err := kp.CacheKey()
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
			cache, err := NewInRepoConfigCache(1, fca, cf)
			if err != nil {
				t.Fatal("could not initialize cache")
			}

			for _, kp := range tc.cacheInitialState {
				k, err := kp.CacheKey()
				if err != nil {
					t.Errorf("Expected error 'nil' got '%v'", err.Error())
				}
				_, _, _ = cache.GetOrAdd(k, goodValConstructorForInitialState(ProwYAML{
					Presubmits: []Presubmit{
						{
							JobBase: JobBase{Name: string(k)}},
					},
				}))
			}

			// Simulate storing a value of the wrong type in the cache (a string
			// instead of a *ProwYAML).
			if tc.cacheCorrupted {
				cache.Purge()

				for _, kp := range tc.cacheInitialState {
					k, err := kp.CacheKey()
					if err != nil {
						t.Errorf("Expected error 'nil' got '%v'", err.Error())
					}
					_, _, _ = cache.GetOrAdd(k, func() (interface{}, error) { return "<wrong-type>", nil })
				}
			}

			prowYAML, err := cache.getProwYAML(tc.valConstructor, tc.identifier, tc.baseSHAGetter, tc.headSHAGetters...)

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

			if tc.expected.cacheLen != cache.Len() {
				t.Errorf("Expected '%d' cached elements, got '%d'", tc.expected.cacheLen, cache.Len())
			}
		})
	}
}

// TestGetProwYAMLCachedAndDefaulted checks that calls to
// cache.GetPresubmits() and cache.GetPostsubmits() return
// defaulted values from the Config, and that changing (reloading) this Config
// and calling it again with the same key (same cached ProwYAML, which has both
// []Presubmit and []Postsubmit jobs) results in returning a __differently__
// defaulted ProwYAML object.
func TestGetProwYAMLCachedAndDefaulted(t *testing.T) {
	identifier := "org/repo"
	baseSHAGetter := goodSHAGetter("ba5e")
	headSHAGetters := []RefGetter{
		goodSHAGetter("abcd"),
		goodSHAGetter("ef01"),
	}

	envBefore := []v1.EnvVar{
		{
			Name:  "ENV_VAR_FOO",
			Value: "VALUE",
		},
	}
	decorationConfigBefore := &prowapi.DecorationConfig{
		GCSConfiguration: &prowapi.GCSConfiguration{
			PathStrategy: prowapi.PathStrategyExplicit,
			DefaultOrg:   "org",
			DefaultRepo:  "repo",
		},
		GCSCredentialsSecret: pStr("service-account-secret"),
		UtilityImages: &prowapi.UtilityImages{
			CloneRefs:  "clonerefs:default-BEFORE",
			InitUpload: "initupload:default-BEFORE",
			Entrypoint: "entrypoint:default-BEFORE",
			Sidecar:    "sidecar:default-BEFORE",
		},
	}

	envAfter := ([]v1.EnvVar)(nil)
	decorationConfigAfter := &prowapi.DecorationConfig{
		GCSConfiguration: &prowapi.GCSConfiguration{
			PathStrategy: prowapi.PathStrategyExplicit,
			DefaultOrg:   "org",
			DefaultRepo:  "repo",
		},
		GCSCredentialsSecret: pStr("service-account-secret"),
		UtilityImages: &prowapi.UtilityImages{
			CloneRefs:  "clonerefs:default-AFTER",
			InitUpload: "initupload:default-AFTER",
			Entrypoint: "entrypoint:default-AFTER",
			Sidecar:    "sidecar:default-AFTER",
		},
	}

	type expected struct {
		presubmits  []Presubmit
		postsubmits []Postsubmit
	}

	true_ := true

	defaultedPresubmit := func(env []v1.EnvVar, dc *prowapi.DecorationConfig) Presubmit {
		return Presubmit{
			JobBase: JobBase{
				Name:           "presubmitFoo",
				Agent:          "kubernetes",
				Cluster:        "clusterFoo",
				Namespace:      pStr("default"),
				ProwJobDefault: &prowapi.ProwJobDefault{TenantID: "GlobalDefaultID"},
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:    "hello",
							Image:   "there",
							Command: []string{"earthlings"},
							Env:     env,
						},
					},
				},
				UtilityConfig: UtilityConfig{
					Decorate:         &true_,
					DecorationConfig: dc,
				},
			},
			Trigger:      `(?m)^/test( | .* )presubmitFoo,?($|\s.*)`,
			RerunCommand: "/test presubmitFoo",
			Reporter: Reporter{
				Context:    "presubmitFoo",
				SkipReport: false,
			},
		}
	}

	defaultedPostsubmit := func(env []v1.EnvVar, dc *prowapi.DecorationConfig) Postsubmit {
		return Postsubmit{
			JobBase: JobBase{
				Name:           "postsubmitFoo",
				Agent:          "kubernetes",
				Cluster:        "clusterFoo",
				Namespace:      pStr("default"),
				ProwJobDefault: &prowapi.ProwJobDefault{TenantID: "GlobalDefaultID"},
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:    "hello",
							Image:   "there",
							Command: []string{"earthlings"},
							Env:     env,
						},
					},
				},
				UtilityConfig: UtilityConfig{
					Decorate:         &true_,
					DecorationConfig: dc,
				},
			},
			Reporter: Reporter{
				Context:    "postsubmitFoo",
				SkipReport: false,
			},
		}
	}
	inRepoConfigEnabled := make(map[string]*bool)
	inRepoConfigEnabled[identifier] = &true_

	// fakeProwYAMLGetterFunc mocks prowYAMLGetter(). Instead of using the
	// git.ClientFactory (and other operations), we just use a simple map to get
	// the *ProwYAML value we want. The point of fakeProwYAMLGetterFunc is to
	// act as a "source of truth" of authoritative *ProwYAML values for purposes
	// of the test cases in this unit test.
	fakeProwYAMLGetterFunc := func() ProwYAMLGetter {
		presubmitUndecorated := Presubmit{
			JobBase: JobBase{
				Name:      "presubmitFoo",
				Cluster:   "clusterFoo",
				Namespace: pStr("default"),
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:    "hello",
							Image:   "there",
							Command: []string{"earthlings"},
						},
					},
				},
			},
		}
		postsubmitUndecorated := Postsubmit{
			JobBase: JobBase{
				Name:      "postsubmitFoo",
				Cluster:   "clusterFoo",
				Namespace: pStr("default"),
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:    "hello",
							Image:   "there",
							Command: []string{"earthlings"},
						},
					},
				},
			},
		}
		return fakeProwYAMLGetterFactory(
			[]Presubmit{presubmitUndecorated},
			[]Postsubmit{postsubmitUndecorated})
	}

	makeConfig := func(env []v1.EnvVar, ddc []*DefaultDecorationConfigEntry) *Config {
		return &Config{
			ProwConfig: ProwConfig{
				InRepoConfig: InRepoConfig{
					AllowedClusters: map[string][]string{
						"org/repo": {"clusterFoo"},
					},
					Enabled: inRepoConfigEnabled,
				},
				Plank: Plank{
					DefaultDecorationConfigs: ddc,
				},
				PodNamespace: "default",
			},
			JobConfig: JobConfig{
				DecorateAllJobs: true,
				ProwYAMLGetter:  fakeProwYAMLGetterFunc(),
				Presets: []Preset{
					{
						Env: env,
					},
				},
			},
		}
	}

	presubmitBefore := defaultedPresubmit(envBefore, decorationConfigBefore)
	postsubmitBefore := defaultedPostsubmit(envBefore, decorationConfigBefore)

	for _, tc := range []struct {
		name string
		// Initial state of Config with a particular DefaultDecorationConfigEntry.
		configBefore   *Config
		expectedBefore expected
		// Changed state of Config with a possibly __different__ DefaultDecorationConfigEntry.
		configAfter   *Config
		expectedAfter expected
	}{
		{
			// Config has not changed between multiple
			// cache.GetPresubmits() calls.
			name: "ConfigNotChanged",
			configBefore: makeConfig(envBefore, []*DefaultDecorationConfigEntry{
				{
					OrgRepo: "*",
					Cluster: "*",
					Config:  decorationConfigBefore,
				},
			}),
			// These are the expected []Presubmit and []Postsubmit values when
			// defaulted with the "decorationConfigBefore" value. Among other
			// things, the UtilityConfig.DecorationConfig value should reflect
			// the same settings as "decorationConfigBefore".
			expectedBefore: expected{
				presubmits:  []Presubmit{presubmitBefore},
				postsubmits: []Postsubmit{postsubmitBefore},
			},
			// For this test case, we do not change the
			// DefualtDecorationConfigEntry at all, so we don't expect any
			// changes.
			configAfter: makeConfig(envBefore, []*DefaultDecorationConfigEntry{
				{
					OrgRepo: "*",
					Cluster: "*",
					Config:  decorationConfigBefore,
				},
			}),
			expectedAfter: expected{
				presubmits:  []Presubmit{presubmitBefore},
				postsubmits: []Postsubmit{postsubmitBefore},
			},
		},
		{
			// Config has changed between multiple requests to cache.
			name: "ConfigChanged",
			configBefore: makeConfig(envBefore, []*DefaultDecorationConfigEntry{
				{
					OrgRepo: "*",
					Cluster: "*",
					Config:  decorationConfigBefore,
				},
			}),
			// These are the expected []Presubmit and []Postsubmit values when
			// defaulted with the "decorationConfigBefore" value. Among other
			// things, the UtilityConfig.DecorationConfig value should reflect
			// the same settings as "decorationConfigBefore".
			expectedBefore: expected{
				presubmits:  []Presubmit{presubmitBefore},
				postsubmits: []Postsubmit{postsubmitBefore},
			},
			// Change the config to decorationConfigAfter.
			configAfter: makeConfig(envAfter, []*DefaultDecorationConfigEntry{
				{
					OrgRepo: "*",
					Cluster: "*",
					Config:  decorationConfigAfter,
				},
			}),
			// Expect "Env" field to be a nil pointer.
			expectedAfter: expected{
				presubmits: []Presubmit{
					{
						JobBase: JobBase{
							Name:           "presubmitFoo",
							Agent:          "kubernetes",
							Cluster:        "clusterFoo",
							Namespace:      pStr("default"),
							ProwJobDefault: &prowapi.ProwJobDefault{TenantID: "GlobalDefaultID"},
							Spec: &v1.PodSpec{
								Containers: []v1.Container{
									{
										Name:    "hello",
										Image:   "there",
										Command: []string{"earthlings"},
										// Env field is a nil pointer!
										Env: nil,
									},
								},
							},
							UtilityConfig: UtilityConfig{
								Decorate:         &true_,
								DecorationConfig: decorationConfigAfter,
							},
						},
						Trigger:      `(?m)^/test( | .* )presubmitFoo,?($|\s.*)`,
						RerunCommand: "/test presubmitFoo",
						Reporter: Reporter{
							Context:    "presubmitFoo",
							SkipReport: false,
						},
					},
				},
				postsubmits: []Postsubmit{
					{
						JobBase: JobBase{
							Name:           "postsubmitFoo",
							Agent:          "kubernetes",
							Cluster:        "clusterFoo",
							Namespace:      pStr("default"),
							ProwJobDefault: &prowapi.ProwJobDefault{TenantID: "GlobalDefaultID"},
							Spec: &v1.PodSpec{
								Containers: []v1.Container{
									{
										Name:    "hello",
										Image:   "there",
										Command: []string{"earthlings"},
										// Env field is a nil pointer!
										Env: nil,
									},
								},
							},
							UtilityConfig: UtilityConfig{
								Decorate:         &true_,
								DecorationConfig: decorationConfigAfter,
							},
						},
						Reporter: Reporter{
							Context:    "postsubmitFoo",
							SkipReport: false,
						},
					},
				},
			},
		},
	} {
		t.Run(tc.name, func(t1 *testing.T) {
			// Set initial Config.
			fca := &fakeConfigAgent{
				c: tc.configBefore,
			}
			cf := &testClientFactory{}

			// Initialize cache. Notice that it relies on a snapshot of the Config with configBefore.
			cache, err := NewInRepoConfigCache(10, fca, cf)
			if err != nil {
				t1.Fatal("could not initialize cache")
			}

			// Get cached values. These cached values should be defaulted by the
			// initial Config.
			// Make sure that this runs concurrently without problem.
			var errGroup errgroup.Group
			for i := 0; i < 1000; i++ {
				errGroup.Go(func() error {
					presubmits, err := cache.GetPresubmits(identifier, baseSHAGetter, headSHAGetters...)
					if err != nil {
						return fmt.Errorf("Expected error 'nil' got '%v'", err.Error())
					}
					if diff := cmp.Diff(tc.expectedBefore.presubmits, presubmits, cmpopts.IgnoreUnexported(Presubmit{}, Brancher{}, RegexpChangeMatcher{})); diff != "" {
						return fmt.Errorf("(before Config reload) presubmits mismatch (-want +got):\n%s", diff)
					}
					return nil
				})

				errGroup.Go(func() error {
					postsubmits, err := cache.GetPostsubmits(identifier, baseSHAGetter, headSHAGetters...)
					if err != nil {
						return fmt.Errorf("Expected error 'nil' got '%v'", err.Error())
					}

					if diff := cmp.Diff(tc.expectedBefore.postsubmits, postsubmits, cmpopts.IgnoreUnexported(Postsubmit{}, Brancher{}, RegexpChangeMatcher{})); diff != "" {
						return fmt.Errorf("(before Config reload) postsubmits mismatch (-want +got):\n%s", diff)
					}
					return nil
				})
			}

			if err := errGroup.Wait(); err != nil {
				t.Fatalf("Failed processing concurrently: %v", err)
			}

			// Reload Config.
			fca.c = tc.configAfter

			presubmits, err := cache.GetPresubmits(identifier, baseSHAGetter, headSHAGetters...)
			if err != nil {
				t1.Fatalf("Expected error 'nil' got '%v'", err.Error())
			}
			postsubmits, err := cache.GetPostsubmits(identifier, baseSHAGetter, headSHAGetters...)
			if err != nil {
				t1.Fatalf("Expected error 'nil' got '%v'", err.Error())
			}

			if diff := cmp.Diff(tc.expectedAfter.presubmits, presubmits, cmpopts.IgnoreUnexported(Presubmit{}, Brancher{}, RegexpChangeMatcher{})); diff != "" {
				t1.Errorf("(after Config reload) presubmits mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.expectedAfter.postsubmits, postsubmits, cmpopts.IgnoreUnexported(Postsubmit{}, Brancher{}, RegexpChangeMatcher{})); diff != "" {
				t1.Errorf("(after Config reload) postsubmits mismatch (-want +got):\n%s", diff)
			}

		})
	}

}
