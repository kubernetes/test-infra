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

func TestGithubOptions_getBaseURL(t *testing.T) {

	testCases := []struct {
		name        string
		endpoints   Strings
		gitURL      string
		tokenPath   string
		expectedErr bool
	}{
		{
			name:        "no token provided",
			endpoints:   NewStrings("https://github.mycorp.com/apis/v3"),
			gitURL:      "https://github.mycorp.com",
			tokenPath:   "",
			expectedErr: false,
		},
		{
			name:        "good github options",
			endpoints:   NewStrings("http://ghproxy", "https://api.github.com"),
			gitURL:      "https://github.com",
			tokenPath:   "token",
			expectedErr: false,
		},
		{
			name:        "invalid git url provided",
			endpoints:   NewStrings("http://github.mycorp.com/apis/v3", "http://apisgateway.mycorp.com/github"),
			gitURL:      "://github.com",
			tokenPath:   "token",
			expectedErr: true,
		},
		{
			name:        "invalid github endpoint provided",
			endpoints:   NewStrings("://github.mycorp.com/apis/v3", "http://apisgateway.mycorp.com/github"),
			gitURL:      "http://github.com",
			tokenPath:   "token",
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			o := GitHubOptions{
				endpoint:  testCase.endpoints,
				gitURL:    testCase.gitURL,
				TokenPath: testCase.tokenPath,
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
