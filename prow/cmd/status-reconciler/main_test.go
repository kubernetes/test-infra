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

	"k8s.io/test-infra/prow/flagutil"
)

func TestGatherOptions(t *testing.T) {
	cases := []struct {
		name     string
		args     map[string]string
		expected func(*options)
	}{
		{
			name: "minimal flags work",
		},
		{
			name: "gcs-credentials-file sets the GCS credentials on the storage client",
			args: map[string]string{
				"-gcs-credentials-file": "/creds",
			},
			expected: func(o *options) {
				o.storage = flagutil.StorageClientOptions{
					GCSCredentialsFile: "/creds",
				}
			},
		},
		{
			name: "s3-credentials-file sets the S3 credentials on the storage client",
			args: map[string]string{
				"-s3-credentials-file": "/creds",
			},
			expected: func(o *options) {
				o.storage = flagutil.StorageClientOptions{
					S3CredentialsFile: "/creds",
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expected := &options{
				dryRun:        true,
				configPath:    "yo",
				pluginConfig:  "/etc/plugins/plugins.yaml",
				kubernetes:    flagutil.KubernetesOptions{DeckURI: "http://whatever"},
				tokenBurst:    100,
				tokensPerHour: 300,
				instrumentationOptions: flagutil.InstrumentationOptions{
					MetricsPort: flagutil.DefaultMetricsPort,
					PProfPort:   flagutil.DefaultPProfPort,
				},
			}
			expectedfs := flag.NewFlagSet("fake-flags", flag.PanicOnError)
			expected.github.AddFlags(expectedfs)
			expected.github.TokenPath = flagutil.DefaultGitHubTokenPath
			if tc.expected != nil {
				tc.expected(expected)
			}

			argMap := map[string]string{
				"--config-path": "yo",
				"--deck-url":    "http://whatever",
			}
			for k, v := range tc.args {
				argMap[k] = v
			}
			var args []string
			for k, v := range argMap {
				args = append(args, k+"="+v)
			}
			fs := flag.NewFlagSet("fake-flags", flag.PanicOnError)
			actual := gatherOptions(fs, args...)
			switch err := actual.Validate(); {
			case err != nil:
				t.Errorf("unexpected error: %v", err)
			case !reflect.DeepEqual(*expected, actual):
				t.Errorf("%#v != expected %#v", actual, *expected)
			}
		})
	}
}
