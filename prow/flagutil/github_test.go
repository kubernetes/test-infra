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
)

func TestGithubOptions(t *testing.T) {

	testCases := []struct {
		name        string
		endpoints   Strings
		gitEndpoint string
		tokenPath   string
		expectedErr bool
	}{
		{
			name:        "No token provided, we expect no errors",
			endpoints:   NewStrings("https://github.mycorp.com/apis/v3"),
			gitEndpoint: "https://github.mycorp.com",
			tokenPath:   "",
			expectedErr: false,
		},
		{
			name:        "Good github options provided, we expect no errors",
			endpoints:   NewStrings("http://ghproxy", "https://api.github.com"),
			gitEndpoint: "https://github.com",
			tokenPath:   "token",
			expectedErr: false,
		},
		{
			name:        "Invalid git url provided, we expect an error",
			endpoints:   NewStrings("http://github.mycorp.com/apis/v3", "http://apisgateway.mycorp.com/github"),
			gitEndpoint: "://github.com",
			tokenPath:   "token",
			expectedErr: true,
		},
		{
			name:        "Invalid github endpoint provided, we expect an error",
			endpoints:   NewStrings("://github.mycorp.com/apis/v3", "http://apisgateway.mycorp.com/github"),
			gitEndpoint: "http://github.com",
			tokenPath:   "token",
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			o := GitHubOptions{
				endpoint:    testCase.endpoints,
				GitEndpoint: testCase.gitEndpoint,
				TokenPath:   testCase.tokenPath,
			}
			err := o.Validate(false)
			if testCase.expectedErr && err == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && err != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
			}
		})
	}
}
