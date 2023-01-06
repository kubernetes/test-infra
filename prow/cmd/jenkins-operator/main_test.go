/*
Copyright 2018 The Kubernetes Authors.

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
	"testing"

	"k8s.io/test-infra/prow/flagutil"
)

func TestOptions_Validate(t *testing.T) {
	var testCases = []struct {
		name        string
		input       options
		expectedErr bool
	}{
		{
			name: "minimal ok with jenkins token",
			input: options{
				jenkinsURL:       "https://example.com",
				jenkinsTokenFile: "secret",
				github:           flagutil.GitHubOptions{TokenPath: "token"},
			},
			expectedErr: false,
		},
		{
			name: "minimal ok with jenkins bearer token",
			input: options{
				jenkinsURL:             "https://example.com",
				jenkinsBearerTokenFile: "secret",
				github:                 flagutil.GitHubOptions{TokenPath: "token"},
			},
			expectedErr: false,
		},
		{
			name: "both jenkins tokens failure",
			input: options{
				jenkinsURL:             "https://example.com",
				jenkinsTokenFile:       "secret",
				jenkinsBearerTokenFile: "other",
				github:                 flagutil.GitHubOptions{TokenPath: "token"},
			},
			expectedErr: true,
		},
		{
			name: "all certificate files given",
			input: options{
				jenkinsURL:       "https://example.com",
				jenkinsTokenFile: "secret",
				certFile:         "cert",
				keyFile:          "key",
				caCertFile:       "cacert",
				github:           flagutil.GitHubOptions{TokenPath: "token"},
			},
			expectedErr: false,
		},
		{
			name: "missing cacert",
			input: options{
				jenkinsURL:       "https://example.com",
				jenkinsTokenFile: "secret",
				certFile:         "cert",
				keyFile:          "key",
				github:           flagutil.GitHubOptions{TokenPath: "token"},
			},
			expectedErr: true,
		},
		{
			name: "missing cert",
			input: options{
				jenkinsURL:       "https://example.com",
				jenkinsTokenFile: "secret",
				keyFile:          "key",
				caCertFile:       "cacert",
				github:           flagutil.GitHubOptions{TokenPath: "token"},
			},
			expectedErr: true,
		},
		{
			name: "missing key",
			input: options{
				jenkinsURL:       "https://example.com",
				jenkinsTokenFile: "secret",
				certFile:         "cert",
				caCertFile:       "cacert",
				github:           flagutil.GitHubOptions{TokenPath: "token"},
			},
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		testCase.input.config.AddFlags(&flag.FlagSet{})
		if testCase.input.config.ConfigPath == "" {
			testCase.input.config.ConfigPath = "/etc/config/config.yaml"
		}
		err := testCase.input.Validate()
		if testCase.expectedErr && err == nil {
			t.Errorf("%s: expected an error but got none", testCase.name)
		}
		if !testCase.expectedErr && err != nil {
			t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
		}
	}
}
