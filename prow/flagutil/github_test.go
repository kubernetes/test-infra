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
	"flag"
	"testing"
)

func TestGithubOptions(t *testing.T) {

	testCases := []struct {
		name        string
		flags       []string
		expectedErr bool
	}{
		{
			name: "No token provided, we expect no errors",
			flags: []string{
				"--github-endpoint=https://github.mycorp.com/apis/v3",
				"--git-endpoint=https://github.mycorp.com",
			},
			expectedErr: false,
		},
		{
			name: "multiple GitHub API endpoints provided, we expect no errors",
			flags: []string{
				"--github-endpoint=http://ghproxy",
				"--github-endpoint=https://api.github.com",
				"--git-endpoint=https://github.com",
				"--github-token-path=token",
			},
			expectedErr: false,
		},
		{
			name: "Invalid git url provided, we expect an error",
			flags: []string{
				"--github-endpoint=http://github.mycorp.com/apis/v3",
				"--github-endpoint=http://apisgateway.mycorp.com/github",
				"--git-endpoint=://github.com",
				"--github-token-path=token",
			},
			expectedErr: true,
		},
		{
			name: "Invalid github endpoint provided, we expect an error",
			flags: []string{
				"--github-endpoint=://github.mycorp.com/apis/v3",
				"--github-endpoint=http://apisgateway.mycorp.com/github",
				"--git-endpoint=://github.com",
				"--github-token-path=token",
			},
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			o := GitHubOptions{}
			fs := flag.NewFlagSet(t.Name(), flag.ContinueOnError)
			o.AddFlagsWithoutDefaultGithubTokenPath(fs)
			err := fs.Parse(testCase.flags)
			errNoDryRun := err
			errWithDryRun := err
			if err == nil {
				errNoDryRun = o.Validate(false)
				errWithDryRun = o.Validate(true)
			}
			if testCase.expectedErr && errNoDryRun == nil {
				t.Errorf("expected an error but got none")
			}
			if !testCase.expectedErr && errNoDryRun != nil {
				t.Errorf("expected no error but got one: %v", err)
			}
			if testCase.expectedErr && errWithDryRun == nil {
				t.Errorf("expected an error but got none")
			}
			if !testCase.expectedErr && errWithDryRun != nil {
				t.Errorf("expected no error but got one: %v", err)
			}
		})
	}
}
