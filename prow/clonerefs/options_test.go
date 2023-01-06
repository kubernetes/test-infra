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

package clonerefs

import (
	"testing"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/github"
)

func TestOptions_Validate(t *testing.T) {
	var testCases = []struct {
		name        string
		input       Options
		expectedErr bool
	}{
		{
			name: "all ok",
			input: Options{
				SrcRoot: "test",
				Log:     "thing",
				GitRefs: []prowapi.Refs{
					{
						Repo: "repo1",
						Org:  "org1",
					},
				},
			},
			expectedErr: false,
		},
		{
			name: "missing src root",
			input: Options{
				Log: "thing",
				GitRefs: []prowapi.Refs{
					{
						Repo: "repo1",
						Org:  "org1",
					},
				},
			},
			expectedErr: true,
		},
		{
			name: "missing Log location",
			input: Options{
				SrcRoot: "test",
				GitRefs: []prowapi.Refs{
					{
						Repo: "repo1",
						Org:  "org1",
					},
				},
			},
			expectedErr: true,
		},
		{
			name: "missing refs",
			input: Options{
				SrcRoot: "test",
				Log:     "thing",
			},
			expectedErr: true,
		},
		{
			name: "separate repos",
			input: Options{
				SrcRoot: "test",
				Log:     "thing",
				GitRefs: []prowapi.Refs{
					{
						Repo: "repo1",
						Org:  "org1",
					},
					{
						Repo: "repo2",
						Org:  "org2",
					},
				},
			},
			expectedErr: false,
		},
		{
			name: "duplicate repos",
			input: Options{
				SrcRoot: "test",
				Log:     "thing",
				GitRefs: []prowapi.Refs{
					{
						Repo: "repo",
						Org:  "org",
					},
					{
						Repo: "repo",
						Org:  "org",
					},
				},
			},
			expectedErr: true,
		},
		{
			name: "specify access token file",
			input: Options{
				SrcRoot: "test",
				Log:     "thing",
				GitRefs: []prowapi.Refs{
					{
						Repo: "repo",
						Org:  "org",
					},
				},
				OauthTokenFile: "/tmp/token",
			},
			expectedErr: false,
		},
		{
			name: "specify GitHub App ID and private key",
			input: Options{
				SrcRoot: "test",
				Log:     "thing",
				GitRefs: []prowapi.Refs{
					{
						Repo: "repo",
						Org:  "org",
					},
				},
				GitHubAPIEndpoints:      []string{github.DefaultAPIEndpoint},
				GitHubAppID:             "123456",
				GitHubAppPrivateKeyFile: "/tmp/private-key.pem",
			},
			expectedErr: false,
		},
		{
			name: "specify aceess token file and GitHub App authentication",
			input: Options{
				SrcRoot: "test",
				Log:     "thing",
				GitRefs: []prowapi.Refs{
					{
						Repo: "repo",
						Org:  "org",
					},
				},
				OauthTokenFile:          "/tmp/token",
				GitHubAPIEndpoints:      []string{github.DefaultAPIEndpoint},
				GitHubAppID:             "123456",
				GitHubAppPrivateKeyFile: "/tmp/private-key.pem",
			},
			expectedErr: true,
		},
		{
			name: "specify GitHub App authentication but no API endpoints",
			input: Options{
				SrcRoot: "test",
				Log:     "thing",
				GitRefs: []prowapi.Refs{
					{
						Repo: "repo",
						Org:  "org",
					},
				},
				GitHubAppID:             "123456",
				GitHubAppPrivateKeyFile: "/tmp/private-key.pem",
			},
			expectedErr: true,
		},
		{
			name: "specify GitHub App ID but no private key",
			input: Options{
				SrcRoot: "test",
				Log:     "thing",
				GitRefs: []prowapi.Refs{
					{
						Repo: "repo",
						Org:  "org",
					},
				},
				GitHubAPIEndpoints: []string{github.DefaultAPIEndpoint},
				GitHubAppID:        "123456",
			},
			expectedErr: true,
		},
		{
			name: "specify GitHub App private key but no ID",
			input: Options{
				SrcRoot: "test",
				Log:     "thing",
				GitRefs: []prowapi.Refs{
					{
						Repo: "repo",
						Org:  "org",
					},
				},
				GitHubAPIEndpoints:      []string{github.DefaultAPIEndpoint},
				GitHubAppPrivateKeyFile: "/tmp/private-key.pem",
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
