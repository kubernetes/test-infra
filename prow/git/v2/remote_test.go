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
	"errors"
	"net/url"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/diff"
)

func TestHTTPResolver(t *testing.T) {
	var testCases = []struct {
		name        string
		remote      func() (*url.URL, error)
		username    LoginGetter
		token       TokenGetter
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
			actual, actualErr := HttpResolver(testCase.remote, testCase.username, testCase.token)()
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

type stringWithError struct {
	str string
	err error
}

// usernameVendor returns a func that vends logins and errors from the provided slices, in order
func usernameVendor(logins []stringWithError) func() (string, error) {
	index := 0
	return func() (string, error) {
		item := logins[index%len(logins)]
		index++
		return item.str, item.err
	}
}

// tokenVendor returns a func that vends tokens from the provided slice, in order
func tokenVendor(tokens [][]byte) func() []byte {
	index := 0
	return func() []byte {
		token := tokens[index%len(tokens)]
		index++
		return token
	}
}

func TestSSHRemoteResolverFactory(t *testing.T) {
	factory := sshRemoteResolverFactory{
		host: "ssh.host.com",
		username: usernameVendor([]stringWithError{
			{str: "first", err: nil},
			{str: "second", err: errors.New("oops")},
			{str: "third", err: nil},
		}),
	}

	central := factory.CentralRemote("org", "repo")
	for i, expected := range []stringWithError{
		{str: "git@ssh.host.com:org/repo.git", err: nil},
		{str: "git@ssh.host.com:org/repo.git", err: nil},
		{str: "git@ssh.host.com:org/repo.git", err: nil},
	} {
		actualRemote, actualErr := central()
		if actualRemote != expected.str {
			t.Errorf("central remote test %d returned incorrect remote, expected %v got %v", i, expected.str, actualRemote)
		}
		if !reflect.DeepEqual(actualErr, expected.err) {
			t.Errorf("central remote test %d returned incorrect error, expected %v got %v", i, expected.err, actualErr)
		}
	}

	publish := factory.PublishRemote("org", "repo")
	for i, expected := range []stringWithError{
		{str: "git@ssh.host.com:first/repo.git", err: nil},
		{str: "", err: errors.New("oops")},
		{str: "git@ssh.host.com:third/repo.git", err: nil},
	} {
		actualRemote, actualErr := publish()
		if actualRemote != expected.str {
			t.Errorf("publish remote test %d returned incorrect remote, expected %v got %v", i, expected.str, actualRemote)
		}
		if !reflect.DeepEqual(actualErr, expected.err) {
			t.Errorf("publish remote test %d returned incorrect error, expected %v got %v", i, expected.err, actualErr)
		}
	}
}

func TestHTTPResolverFactory_NoAuth(t *testing.T) {
	t.Run("CentralRemote", func(t *testing.T) {
		expected := "https://some-host.com/org/repo"
		res, err := (&httpResolverFactory{host: "some-host.com"}).CentralRemote("org", "repo")()
		if err != nil {
			t.Fatalf("CentralRemote: %v", err)
		}
		if res != expected {
			t.Errorf("Expected result to be %s, was %s", expected, res)
		}
	})

	t.Run("PublishRemote", func(t *testing.T) {
		expectedErr := "could not resolve remote: username not configured, no publish repo available"
		_, err := (&httpResolverFactory{host: "some-host.com"}).PublishRemote("org", "repo")()
		if err == nil || err.Error() != expectedErr {
			t.Errorf("expectedErr to be %s, was %v", expectedErr, err)
		}
	})
}

func TestHTTPResolverFactory(t *testing.T) {
	factory := httpResolverFactory{
		host: "host.com",
		username: usernameVendor([]stringWithError{
			{str: "first", err: nil},
			{str: "second", err: errors.New("oops")},
			{str: "third", err: nil},
			// this is called twice for publish remote resolution
			{str: "first", err: nil},
			{str: "first", err: nil},
			{str: "second", err: errors.New("oops")},
			{str: "third", err: nil},
			{str: "third", err: nil},
			{str: "fourth", err: nil},
			{str: "fourth", err: errors.New("oops")},
		}),
		token: tokenVendor([][]byte{[]byte("one"), []byte("three")}), // only called when username succeeds
	}

	central := factory.CentralRemote("org", "repo")
	for i, expected := range []stringWithError{
		{str: "https://first:one@host.com/org/repo", err: nil},
		{str: "", err: errors.New("could not resolve username: oops")},
		{str: "https://third:three@host.com/org/repo", err: nil},
	} {
		actualRemote, actualErr := central()
		if actualRemote != expected.str {
			t.Errorf("central remote test %d returned incorrect remote, expected %v got %v", i, expected.str, actualRemote)
		}
		if !reflect.DeepEqual(actualErr, expected.err) {
			t.Errorf("central remote test %d returned incorrect error, expected %v got %v", i, expected.err, actualErr)
		}
	}

	publish := factory.PublishRemote("org", "repo")
	for i, expected := range []stringWithError{
		{str: "https://first:one@host.com/first/repo", err: nil},
		{str: "", err: errors.New("could not resolve remote: oops")},
		{str: "https://third:three@host.com/third/repo", err: nil},
		{str: "", err: errors.New("could not resolve username: oops")},
	} {
		actualRemote, actualErr := publish()
		if actualRemote != expected.str {
			t.Errorf("publish remote test %d returned incorrect remote, expected %v got %v", i, expected.str, actualRemote)
		}
		if !reflect.DeepEqual(actualErr, expected.err) {
			t.Errorf("publish remote test %d returned incorrect error, expected %v got %v", i, expected.err, actualErr)
		}
	}
}
