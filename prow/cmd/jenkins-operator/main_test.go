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

import "testing"

func TestOptions_Validate(t *testing.T) {
	var testCases = []struct {
		name        string
		input       options
		expectedErr bool
	}{
		{
			name: "minimal ok with jenkins token",
			input: options{
				jenkinsTokenFile: "secret",
			},
			expectedErr: false,
		},
		{
			name: "minimal ok with jenkins bearer token",
			input: options{
				jenkinsBearerTokenFile: "secret",
			},
			expectedErr: false,
		},
		{
			name: "both jenkins tokens failure",
			input: options{
				jenkinsTokenFile:       "secret",
				jenkinsBearerTokenFile: "other",
			},
			expectedErr: true,
		},
		{
			name: "all certificate files given",
			input: options{
				jenkinsTokenFile: "secret",
				certFile:         "cert",
				keyFile:          "key",
				caCertFile:       "cacert",
			},
			expectedErr: false,
		},
		{
			name: "missing cacert",
			input: options{
				jenkinsTokenFile: "secret",
				certFile:         "cert",
				keyFile:          "key",
			},
			expectedErr: true,
		},
		{
			name: "missing cert",
			input: options{
				jenkinsTokenFile: "secret",
				keyFile:          "key",
				caCertFile:       "cacert",
			},
			expectedErr: true,
		},
		{
			name: "missing key",
			input: options{
				jenkinsTokenFile: "secret",
				certFile:         "cert",
				caCertFile:       "cacert",
			},
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		err := testCase.input.Validate()
		if testCase.expectedErr && err == nil {
			t.Errorf("%s: expected an error but got none", testCase.name)
		}
		if !testCase.expectedErr && err != nil {
			t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
		}
	}
}
