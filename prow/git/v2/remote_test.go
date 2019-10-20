/*
Copyright 2019 The Kubernetes Authors.

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

package git

import (
	"net/url"
	"testing"
)

func TestSimpleAuthResolver_Resolve(t *testing.T) {
	var testCases = []struct {
		name        string
		remote      func() (*url.URL, error)
		username    func() (string, error)
		token       func() []byte
		expected    string
		expectedErr bool
	}{
		{
			name: "happy case works",
			remote: func() (*url.URL, error) {
				return &url.URL{
					Scheme: "https",
					Host:   "github.com",
					Path:   "org/repo",
				}, nil
			},
			username: func() (string, error) {
				return "gitUser", nil
			},
			token: func() []byte {
				return []byte("pass")
			},
			expected:    "https://gitUser:pass@github.com/org/repo",
			expectedErr: false,
		},
		{
			name: "failure to resolve remote URL creates error",
			remote: func() (*url.URL, error) {
				return nil, errors.New("oops")
			},
			expectedErr: true,
		},
		{
			name: "failure to get username creates error",
			remote: func() (*url.URL, error) {
				return nil, nil
			},
			username: func() (string, error) {
				return "", errors.New("oops")
			},
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actual, actualErr := NewSimpleAuthResolver(testCase.remote, testCase.username, testCase.token).Resolve()
			if testCase.expectedErr && actualErr == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && actualErr != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, actualErr)
			}
			if actual != testCase.expected {
				t.Errorf("%s: got incorrect remote URL: %v", testCase.name, diff.StringDiff(actual, testCase.expected))
			}
		})
	}
}

func TestSSHResolver_Resolve(t *testing.T) {
	var testCases = []struct {
		name        string
		base, repo  string
		org         func() (string, error)
		expected    string
		expectedErr bool
	}{
		{
			name: "happy case works",
			base: "github.com",
			org: func() (string, error) {
				return "my-gitUser", nil
			},
			repo:     "something",
			expected: "git@github.com:my-gitUser/something.git",
		},
		{
			name: "org resolution fails",
			base: "github.com",
			org: func() (string, error) {
				return "", errors.New("oops")
			},
			repo:        "something",
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actual, actualErr := NewSSHResolver(testCase.base, testCase.repo, testCase.org).Resolve()
			if testCase.expectedErr && actualErr == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && actualErr != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, actualErr)
			}
			if actual != testCase.expected {
				t.Errorf("%s: got incorrect remote URL: %v", testCase.name, diff.StringDiff(actual, testCase.expected))
			}
		})
	}
}
