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

package flagutil

import (
	"testing"

	"k8s.io/test-infra/pkg/flagutil"
)

func TestKubernetesOptions_Validate(t *testing.T) {
	var testCases = []struct {
		name        string
		dryRun      bool
		kubernetes  flagutil.OptionGroup
		expectedErr bool
	}{
		{
			name:        "all ok without dry-run",
			dryRun:      false,
			kubernetes:  &KubernetesOptions{},
			expectedErr: false,
		},
		{
			name:   "all ok with dry-run",
			dryRun: true,
			kubernetes: &KubernetesOptions{
				deckURI: "https://example.com",
			},
			expectedErr: false,
		},
		{
			name:        "missing deck endpoint with dry-run",
			dryRun:      true,
			kubernetes:  &KubernetesOptions{},
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.kubernetes.Validate(testCase.dryRun)
			if testCase.expectedErr && err == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && err != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
			}
		})
	}
}
