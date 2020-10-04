/*
Copyright 2020 The Kubernetes Authors.

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

package flagutil

import (
	"testing"

	"k8s.io/test-infra/prow/github"
)

func TestGitHubOptions_Validate(t *testing.T) {
	t.Parallel()
	var testCases = []struct {
		name                    string
		in                      *GitHubOptions
		expectedTokenPath       string
		expectedGraphqlEndpoint string
		expectedErr             bool
	}{
		{
			name:                    "when no endpoints, sets github token path and graphql endpoint",
			in:                      &GitHubOptions{},
			expectedTokenPath:       DefaultGitHubTokenPath,
			expectedGraphqlEndpoint: github.DefaultGraphQLEndpoint,
			expectedErr:             false,
		},
		{
			name: "when empty endpoint, sets github token path and graphql endpoint",
			in: &GitHubOptions{
				endpoint: NewStrings(""),
			},
			expectedTokenPath:       DefaultGitHubTokenPath,
			expectedGraphqlEndpoint: github.DefaultGraphQLEndpoint,
			expectedErr:             false,
		},
		{
			name: "when invalid github endpoint, returns error",
			in: &GitHubOptions{
				endpoint: NewStrings("not a github url"),
			},
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(s *testing.T) {
			err := testCase.in.Validate(false)
			if testCase.expectedErr && err == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && err != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
			}
			if testCase.expectedTokenPath != testCase.in.TokenPath {
				t.Errorf("%s: unexpected token path", testCase.name)
			}
			if testCase.expectedGraphqlEndpoint != testCase.in.graphqlEndpoint {
				t.Errorf("%s: unexpected graphql endpoint", testCase.name)
			}
		})
	}
}
