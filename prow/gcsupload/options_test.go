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

package gcsupload

import (
	"testing"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/flagutil"
)

func TestOptions_Validate(t *testing.T) {
	var testCases = []struct {
		name        string
		input       Options
		expectedErr bool
	}{
		{
			name: "minimal set ok",
			input: Options{
				DryRun: true,
				GCSConfiguration: &prowapi.GCSConfiguration{
					PathStrategy: prowapi.PathStrategyExplicit,
				},
			},
			expectedErr: false,
		},
		{
			name: "push to GCS, ok",
			input: Options{
				DryRun: false,
				StorageClientOptions: flagutil.StorageClientOptions{
					GCSCredentialsFile: "secrets",
				},
				GCSConfiguration: &prowapi.GCSConfiguration{
					Bucket:       "seal",
					PathStrategy: prowapi.PathStrategyExplicit,
				},
			},
			expectedErr: false,
		},
		{
			name: "push to GCS, missing bucket",
			input: Options{
				DryRun: false,
				StorageClientOptions: flagutil.StorageClientOptions{
					GCSCredentialsFile: "secrets",
				},
				GCSConfiguration: &prowapi.GCSConfiguration{
					PathStrategy: prowapi.PathStrategyExplicit,
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

func TestValidatePathOptions(t *testing.T) {
	var testCases = []struct {
		name        string
		strategy    string
		org         string
		repo        string
		expectedErr bool
	}{
		{
			name:        "invalid strategy",
			strategy:    "whatever",
			expectedErr: true,
		},
		{
			name:        "explicit strategy, no defaults",
			strategy:    prowapi.PathStrategyExplicit,
			expectedErr: false,
		},
		{
			name:        "legacy strategy, no defaults",
			strategy:    "legacy",
			expectedErr: true,
		},
		{
			name:        "legacy strategy, no default repo",
			strategy:    "legacy",
			org:         "org",
			expectedErr: true,
		},
		{
			name:        "legacy strategy, no default org",
			strategy:    "legacy",
			repo:        "repo",
			expectedErr: true,
		},
		{
			name:        "legacy strategy, with defaults",
			strategy:    "legacy",
			org:         "org",
			repo:        "repo",
			expectedErr: false,
		},
		{
			name:        "single strategy, no defaults",
			strategy:    "single",
			expectedErr: true,
		},
		{
			name:        "single strategy, no default repo",
			strategy:    "single",
			org:         "org",
			expectedErr: true,
		},
		{
			name:        "single strategy, no default org",
			strategy:    "single",
			repo:        "repo",
			expectedErr: true,
		},
		{
			name:        "single strategy, with defaults",
			strategy:    "single",
			org:         "org",
			repo:        "repo",
			expectedErr: false,
		},
	}

	for _, testCase := range testCases {
		o := Options{
			DryRun: true,
			GCSConfiguration: &prowapi.GCSConfiguration{
				PathStrategy: testCase.strategy,
				DefaultOrg:   testCase.org,
				DefaultRepo:  testCase.repo,
			},
		}
		err := o.Validate()
		if err != nil && !testCase.expectedErr {
			t.Errorf("%s: expected no err but got %v", testCase.name, err)
		}
		if err == nil && testCase.expectedErr {
			t.Errorf("%s: expected err but got none", testCase.name)
		}
	}
}
