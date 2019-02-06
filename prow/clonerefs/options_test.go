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

	"k8s.io/test-infra/prow/kube"
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
				GitRefs: []kube.Refs{
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
				GitRefs: []kube.Refs{
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
				GitRefs: []kube.Refs{
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
				GitRefs: []kube.Refs{
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
				GitRefs: []kube.Refs{
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
