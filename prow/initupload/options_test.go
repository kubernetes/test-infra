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

package initupload

import (
	"testing"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/gcsupload"
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
				Log: "testing",
				Options: &gcsupload.Options{
					DryRun: true,
					GCSConfiguration: &prowapi.GCSConfiguration{
						PathStrategy: prowapi.PathStrategyExplicit,
					},
				},
			},
			expectedErr: false,
		},
		{
			name: "missing clone log",
			input: Options{
				Options: &gcsupload.Options{
					DryRun: true,
					GCSConfiguration: &prowapi.GCSConfiguration{
						PathStrategy: prowapi.PathStrategyExplicit,
					},
				},
			},
			expectedErr: false,
		},
		{
			name: "missing path strategy",
			input: Options{
				Options: &gcsupload.Options{
					DryRun:           true,
					GCSConfiguration: &prowapi.GCSConfiguration{},
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
