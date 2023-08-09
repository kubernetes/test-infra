/*
Copyright 2017 The Kubernetes Authors.

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

import (
	"flag"
	"reflect"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/flagutil"
	configflagutil "k8s.io/test-infra/prow/flagutil/config"
)

func Test_gatherOptions(t *testing.T) {
	cases := []struct {
		name     string
		args     map[string]string
		del      sets.Set[string]
		expected func(*options)
		err      bool
	}{
		{
			name: "minimal flags work",
			expected: func(o *options) {
				o.controllerManager.TimeoutListingProwJobs = 30 * time.Second
				o.controllerManager.TimeoutListingProwJobsDefault = 30 * time.Second
			},
		},
		{
			name: "explicitly set --config-path",
			args: map[string]string{
				"--config-path": "/random/value",
			},
			expected: func(o *options) {
				o.config.ConfigPath = "/random/value"
				o.controllerManager.TimeoutListingProwJobs = 30 * time.Second
				o.controllerManager.TimeoutListingProwJobsDefault = 30 * time.Second
			},
		},
		{
			name: "expicitly set --dry-run=false",
			args: map[string]string{
				"--dry-run": "false",
			},
			expected: func(o *options) {
				o.dryRun = false
				o.controllerManager.TimeoutListingProwJobs = 30 * time.Second
				o.controllerManager.TimeoutListingProwJobsDefault = 30 * time.Second
			},
		},
		{
			name: "gcs-credentials-file sets the credentials on the storage client",
			args: map[string]string{
				"-gcs-credentials-file": "/creds",
			},
			expected: func(o *options) {
				o.storage = flagutil.StorageClientOptions{
					GCSCredentialsFile: "/creds",
				}
				o.controllerManager.TimeoutListingProwJobs = 30 * time.Second
				o.controllerManager.TimeoutListingProwJobsDefault = 30 * time.Second
			},
		},
		{
			name: "s3-credentials-file sets the credentials on the storage client",
			args: map[string]string{
				"-s3-credentials-file": "/creds",
			},
			expected: func(o *options) {
				o.storage = flagutil.StorageClientOptions{
					S3CredentialsFile: "/creds",
				}
				o.controllerManager.TimeoutListingProwJobs = 30 * time.Second
				o.controllerManager.TimeoutListingProwJobsDefault = 30 * time.Second
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expected := &options{
				port: 8888,
				config: configflagutil.ConfigOptions{
					ConfigPathFlagName:                    "config-path",
					JobConfigPathFlagName:                 "job-config-path",
					ConfigPath:                            "yo",
					SupplementalProwConfigsFileNameSuffix: "_prowconfig.yaml",
					InRepoConfigCacheSize:                 200,
				},
				dryRun:                 true,
				syncThrottle:           800,
				statusThrottle:         400,
				maxRecordsPerPool:      1000,
				instrumentationOptions: flagutil.DefaultInstrumentationOptions(),
			}
			expectedfs := flag.NewFlagSet("fake-flags", flag.PanicOnError)
			expected.github.AddFlags(expectedfs)
			if tc.expected != nil {
				tc.expected(expected)
			}

			argMap := map[string]string{
				"--config-path": "yo",
			}
			for k, v := range tc.args {
				argMap[k] = v
			}
			for k := range tc.del {
				delete(argMap, k)
			}

			var args []string
			for k, v := range argMap {
				args = append(args, k+"="+v)
			}
			fs := flag.NewFlagSet("fake-flags", flag.PanicOnError)
			actual := gatherOptions(fs, args...)
			switch err := actual.Validate(); {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Errorf("failed to receive expected error")
			case !reflect.DeepEqual(*expected, actual):
				t.Errorf("%#v != expected %#v", actual, *expected)
			}
		})
	}
}

func TestProvider(t *testing.T) {
	tests := []struct {
		name       string
		provider   string
		tideConfig config.Tide
		expect     string
	}{
		{
			name:       "default",
			provider:   "",
			tideConfig: config.Tide{},
			expect:     "github",
		},
		{
			name:     "only-gerrit-config",
			provider: "",
			tideConfig: config.Tide{Gerrit: &config.TideGerritConfig{
				Queries: config.GerritOrgRepoConfigs{
					config.GerritOrgRepoConfig{},
				},
			}},
			expect: "gerrit",
		},
		{
			name:     "only-github-config",
			provider: "",
			tideConfig: config.Tide{TideGitHubConfig: config.TideGitHubConfig{
				Queries: config.TideQueries{
					{},
				},
			}},
			expect: "github",
		},
		{
			name:     "both-config",
			provider: "",
			tideConfig: config.Tide{
				TideGitHubConfig: config.TideGitHubConfig{
					Queries: config.TideQueries{
						{},
					},
				},
				Gerrit: &config.TideGerritConfig{
					Queries: config.GerritOrgRepoConfigs{
						config.GerritOrgRepoConfig{},
					},
				},
			},
			expect: "github",
		},
		{
			name:     "explicit-github",
			provider: "github",
			tideConfig: config.Tide{
				Gerrit: &config.TideGerritConfig{
					Queries: config.GerritOrgRepoConfigs{
						config.GerritOrgRepoConfig{},
					},
				},
			},
			expect: "github",
		},
		{
			name:     "explicit-gerrit",
			provider: "gerrit",
			tideConfig: config.Tide{
				TideGitHubConfig: config.TideGitHubConfig{
					Queries: config.TideQueries{
						{},
					},
				},
			},
			expect: "gerrit",
		},
		{
			name:     "explicit-unsupported-provider",
			provider: "foobar",
			tideConfig: config.Tide{
				TideGitHubConfig: config.TideGitHubConfig{
					Queries: config.TideQueries{
						{},
					},
				},
				Gerrit: &config.TideGerritConfig{
					Queries: config.GerritOrgRepoConfigs{
						config.GerritOrgRepoConfig{},
					},
				},
			},
			expect: "",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if want, got := tc.expect, provider(tc.provider, tc.tideConfig); want != got {
				t.Errorf("Wrong provider. Want: %s, got: %s", want, got)
			}
		})
	}
}
