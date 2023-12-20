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
	"fmt"
	"reflect"
	"testing"
)

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

// fakeTokenGetter returns a func that vends tokens from the provided slice, in order
func fakeTokenGetter(tokens []string) func(org string) (string, error) {
	index := 0
	return func(_ string) (string, error) {
		token := tokens[index%len(tokens)]
		index++
		return token, nil
	}
}

func TestSSHRemoteResolverFactory(t *testing.T) {
	factory := sshRemoteResolverFactory{
		host: "ssh.host.com",
		username: usernameVendor([]stringWithError{
			{str: "zero", err: nil},
			{str: "first", err: nil},
			{str: "second", err: errors.New("oops")},
			{str: "third", err: nil},
			// For publish remote test with unique fork name
			{str: "fourth", err: nil},
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

	// Test with unique fork name before
	expected := "git@ssh.host.com:zero/fork-repo.git"
	actualRemote, actualErr := publish("fork-repo")
	if actualRemote != expected {
		t.Errorf("publish remote test with different fork name returned incorrect remote, expected %v got %v", expected, actualRemote)
	}
	if actualErr != nil {
		t.Errorf("publish remote test with different fork name returned an unexpected error: %v", actualErr)
	}
	// Test with default fork name
	for i, expected := range []stringWithError{
		{str: "git@ssh.host.com:first/repo.git", err: nil},
		{str: "", err: errors.New("oops")},
		{str: "git@ssh.host.com:third/repo.git", err: nil},
	} {
		actualRemote, actualErr := publish("")
		if actualRemote != expected.str {
			t.Errorf("publish remote test %d returned incorrect remote, expected %v got %v", i, expected.str, actualRemote)
		}
		if !reflect.DeepEqual(actualErr, expected.err) {
			t.Errorf("publish remote test %d returned incorrect error, expected %v got %v", i, expected.err, actualErr)
		}
	}
	// Test with unique fork name after
	expected = "git@ssh.host.com:fourth/fork-repo.git"
	actualRemote, actualErr = publish("fork-repo")
	if actualRemote != expected {
		t.Errorf("publish remote test with different fork name returned incorrect remote, expected %v got %v", expected, actualRemote)
	}
	if actualErr != nil {
		t.Errorf("publish remote test with different fork name returned an unexpected error: %v", actualErr)
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
		expectedErr := "username not configured, no publish repo available"
		_, err := (&httpResolverFactory{host: "some-host.com"}).PublishRemote("org", "repo")("")
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
			// For testing publish remote with a unique forkName
			{str: "zero", err: nil},
			{str: "zero", err: nil},
			// testing publish remote with default forkName
			{str: "first", err: nil},
			{str: "first", err: nil},
			{str: "second", err: errors.New("oops")},
			{str: "third", err: nil},
			{str: "third", err: nil},
			{str: "fourth", err: nil},
			{str: "fourth", err: errors.New("oops")},
			// For testing publish remote with a unique forkName
			{str: "fifth", err: nil},
			{str: "fifth", err: nil},
		}),
		token: fakeTokenGetter([]string{"one", "three"}), // only called when username succeeds
	}

	central := factory.CentralRemote("org", "repo")
	for i, expected := range []stringWithError{
		{str: "https://first:one@host.com/org/repo", err: nil},
		{str: "", err: fmt.Errorf("could not resolve username: %w", errors.New("oops"))},
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
	// test publish with a different fork name before
	expected := "https://zero:one@host.com/zero/fork-repo"
	actualRemote, actualErr := publish("fork-repo")
	if actualRemote != expected {
		t.Errorf("publish remote with a different fork name returned incorrect remote, expected %v got %v", expected, actualRemote)
	}
	if actualErr != nil {
		t.Errorf("publish remote with a different fork name returned an unexpected error: %v", actualErr)
	}

	// Test with default fork name
	for i, expected := range []stringWithError{
		{str: "https://first:three@host.com/first/repo", err: nil},
		{str: "", err: fmt.Errorf("could not resolve username: %w", errors.New("oops"))},
		{str: "https://third:one@host.com/third/repo", err: nil},
		{str: "", err: fmt.Errorf("could not resolve remote: %w", fmt.Errorf("could not resolve username: %w", errors.New("oops")))},
	} {
		actualRemote, actualErr := publish("")
		if actualRemote != expected.str {
			t.Errorf("publish remote test %d returned incorrect remote, expected %v got %v", i, expected.str, actualRemote)
		}
		if !reflect.DeepEqual(actualErr, expected.err) {
			t.Errorf("publish remote test %d returned incorrect error, expected %v got %v", i, expected.err, actualErr)
		}
	}
	// test publish with a different fork name after
	expected = "https://fifth:three@host.com/fifth/fork-repo"
	actualRemote, actualErr = publish("fork-repo")
	if actualRemote != expected {
		t.Errorf("publish remote with a different fork name returned incorrect remote, expected %v got %v", expected, actualRemote)
	}
	if actualErr != nil {
		t.Errorf("publish remote with a different fork name returned an unexpected error: %v", actualErr)
	}
}
