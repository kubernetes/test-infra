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
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/diff"
)

func urlMustParse(t testing.TB, u string) *url.URL {
	ret, err := url.Parse(u)
	if err != nil {
		t.Fatal("Unable to parse URL: ", err)
	}
	return ret
}

type tmpdirMock struct {
	basedir   string
	readerIdx int
}

func newTmpdirMock(t testing.TB) (mock *tmpdirMock, cleanup func()) {
	basedir, err := ioutil.TempDir("", "mock")
	if err != nil {
		t.Fatal(err)
	}
	return &tmpdirMock{basedir: basedir}, func() { os.RemoveAll(basedir) }
}

func (t *tmpdirMock) recorded(idx int) string {
	return fmt.Sprintf("%s/%d", t.basedir, idx)
}

func (t *tmpdirMock) TempDir(dir, prefix string) (ret string, err error) {
	ret = t.recorded(t.readerIdx)
	t.readerIdx++
	return
}

func TestClone(t *testing.T) {
	gitbin, err := exec.LookPath("git")
	if err != nil {
		t.Errorf("Git is not found: %v", err)
	}

	tempdir, cleanup := newTmpdirMock(t)
	defer cleanup()
	ioutilTempDir = tempdir.TempDir
	defer func() { ioutilTempDir = ioutil.TempDir }()

	cases := []struct {
		name           string        //name of the test
		base           string        //Client parameters
		user           string        //if user is not empty, SetCredentials will be called
		tokenGenerator func() []byte //credentials token generator func

		repo                 string     // what repo do we clone?
		simulateExecFailures int        // how many exec failures?
		commands             [][]string // what commands does this run?
		expected             *Repo      // what repo does it return?
		err                  bool       // did it return any error?
	}{
		{
			name:                 "happy path with remote url and user",
			base:                 "https://github.com",
			user:                 "user",
			tokenGenerator:       func() []byte { return []byte("password") },
			repo:                 "org/repo",
			simulateExecFailures: 0,
			commands: [][]string{
				{"", gitbin, "clone", "--mirror", "https://user:password@github.com/org/repo", tempdir.recorded(0) + "/org/repo.git"},
				{"", gitbin, "clone", tempdir.recorded(0) + "/org/repo.git", tempdir.recorded(1)},
			},
			expected: &Repo{
				Dir:  tempdir.recorded(1),
				git:  gitbin,
				base: urlMustParse(t, "https://github.com"),
				repo: "org/repo",
				user: "user",
				pass: "password",
			},
		},
		{
			name:                 "happy path with remote url and user, one retry",
			base:                 "https://github.com",
			user:                 "user",
			tokenGenerator:       func() []byte { return []byte("password") },
			repo:                 "org/repo",
			simulateExecFailures: 1,
			commands: [][]string{
				{"", gitbin, "clone", "--mirror", "https://user:password@github.com/org/repo", tempdir.recorded(2) + "/org/repo.git"},
				{"", gitbin, "clone", "--mirror", "https://user:password@github.com/org/repo", tempdir.recorded(2) + "/org/repo.git"},
				{"", gitbin, "clone", tempdir.recorded(2) + "/org/repo.git", tempdir.recorded(3)},
			},
			expected: &Repo{
				Dir:  tempdir.recorded(3),
				git:  gitbin,
				base: urlMustParse(t, "https://github.com"),
				repo: "org/repo",
				user: "user",
				pass: "password",
			},
		},
		{
			name:                 "happy path with remote url and user, 2 retries",
			base:                 "https://github.com",
			user:                 "user",
			tokenGenerator:       func() []byte { return []byte("password") },
			repo:                 "org/repo",
			simulateExecFailures: 2,
			commands: [][]string{
				{"", gitbin, "clone", "--mirror", "https://user:password@github.com/org/repo", tempdir.recorded(4) + "/org/repo.git"},
				{"", gitbin, "clone", "--mirror", "https://user:password@github.com/org/repo", tempdir.recorded(4) + "/org/repo.git"},
				{"", gitbin, "clone", "--mirror", "https://user:password@github.com/org/repo", tempdir.recorded(4) + "/org/repo.git"},
				{"", gitbin, "clone", tempdir.recorded(4) + "/org/repo.git", tempdir.recorded(5)},
			},
			expected: &Repo{
				Dir:  tempdir.recorded(5),
				git:  gitbin,
				base: urlMustParse(t, "https://github.com"),
				repo: "org/repo",
				user: "user",
				pass: "password",
			},
		},
		{
			name:                 "error path with remote url and user, retries exceeded",
			base:                 "https://github.com",
			user:                 "user",
			tokenGenerator:       func() []byte { return []byte("password") },
			repo:                 "org/repo",
			simulateExecFailures: 3,
			commands: [][]string{
				{"", gitbin, "clone", "--mirror", "https://user:password@github.com/org/repo", tempdir.recorded(6) + "/org/repo.git"},
				{"", gitbin, "clone", "--mirror", "https://user:password@github.com/org/repo", tempdir.recorded(6) + "/org/repo.git"},
				{"", gitbin, "clone", "--mirror", "https://user:password@github.com/org/repo", tempdir.recorded(6) + "/org/repo.git"},
			},
			err: true,
		},
		{
			name:                 "happy path with GHE",
			base:                 "https://corp.github.initech.com",
			user:                 "user",
			tokenGenerator:       func() []byte { return []byte("password") },
			repo:                 "org/repo",
			simulateExecFailures: 0,
			commands: [][]string{
				{"", gitbin, "clone", "--mirror", "https://user:password@corp.github.initech.com/org/repo", tempdir.recorded(7) + "/org/repo.git"},
				{"", gitbin, "clone", tempdir.recorded(7) + "/org/repo.git", tempdir.recorded(8)},
			},
			expected: &Repo{
				Dir:  tempdir.recorded(8),
				git:  gitbin,
				base: urlMustParse(t, "https://corp.github.initech.com"),
				repo: "org/repo",
				user: "user",
				pass: "password",
			},
		},
	}
	defer func() { execWithDir = defaultExecWithDir }()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			simulatedFailures := 0
			var actualCommands [][]string
			execWithDir = func(dir, name string, args ...string) ([]byte, error) {
				actualCommands = append(actualCommands, append([]string{dir, name}, args...))
				if simulatedFailures < tc.simulateExecFailures {
					simulatedFailures++
					return []byte("git error stdout"), fmt.Errorf("Simulated failure %d", simulatedFailures)
				}
				return []byte("git stdout"), nil
			}
			baseURL, err := url.Parse(tc.base)
			if err != nil {
				t.Errorf("Unexpected error parsing url %s: %v", tc.base, err)
			}
			client, err := NewClient(baseURL)
			if err != nil {
				t.Errorf("Unexpected error creating Client: %v", err)
			}
			client.SetCredentials(tc.user, tc.tokenGenerator)

			actual, err := client.Clone(tc.repo)
			switch {
			case !tc.err && err != nil:
				t.Errorf("Unexpected error result: %v", err)
			case tc.err && err == nil:
				t.Errorf("Expected error, got nil")
			}
			if tc.err && actual != nil {
				t.Errorf("Unexpected non-nil result")
			}
			if tc.expected != nil {
				tc.expected.logger = client.logger
			}
			if !reflect.DeepEqual(tc.expected, actual) {
				t.Errorf("Unexpected value, diff: %s", diff.ObjectReflectDiff(actual, tc.expected))
			}
			if !reflect.DeepEqual(tc.commands, actualCommands) {
				t.Errorf("Unexpected call, diff: %s", diff.ObjectReflectDiff(tc.commands, actualCommands))
			}
		})
	}
}

func TestCloneCache(t *testing.T) {
	gitbin, err := exec.LookPath("git")
	if err != nil {
		t.Errorf("Git is not found: %v", err)
	}
	tempdir, cleanup := newTmpdirMock(t)
	defer cleanup()
	ioutilTempDir = tempdir.TempDir
	var actualCommands [][]string
	execWithDir = func(dir, name string, args ...string) ([]byte, error) {
		actualCommands = append(actualCommands, append([]string{dir, name}, args...))
		return nil, nil
	}
	defer func() { ioutilTempDir = ioutil.TempDir; execWithDir = defaultExecWithDir }()
	expectedCommands := [][]string{
		{tempdir.recorded(0) + "/org/repo.git", gitbin, "fetch"},
		{"", gitbin, "clone", tempdir.recorded(0) + "/org/repo.git", tempdir.recorded(1)},
	}
	baseURL := urlMustParse(t, "https://github.com")
	client, err := NewClient(baseURL)
	if err != nil {
		t.Fatalf("Unexpected error creating Client: %v", err)
	}
	client.SetCredentials("user", func() []byte { return []byte("password") })

	err = os.MkdirAll(tempdir.recorded(0)+"/org/repo.git", os.ModePerm)
	_, err = client.Clone("org/repo")
	if err != nil {
		t.Errorf("Unexpected error returned: %v", err)
	}
	if !reflect.DeepEqual(expectedCommands, actualCommands) {
		t.Errorf("Unexpected calls, diff: %s", diff.ObjectReflectDiff(expectedCommands, actualCommands))
	}
}

func TestNewClient(t *testing.T) {
	gitURL := urlMustParse(t, "https://github.mycorp.com")
	t.Logf("Verifying client is created with correct base endpoint.")
	a, err := NewClient(gitURL)
	if err != nil {
		t.Fatalf("Error creating new client: %+v", err)
	}
	if *a.base != *gitURL {
		t.Errorf("NewClient base was different than expected: %v", err)
	}
}

func TestRemote(t *testing.T) {
	tests := []struct {
		name      string
		base      string
		user      string
		pass      string
		pathItems string
		expected  string
		err       bool
	}{
		{
			name:      "A valid remote url, with user, and password, no path",
			base:      "https://github.com",
			user:      "user",
			pass:      "pass",
			pathItems: "",
			expected:  "https://user:pass@github.com",
		},
		{
			name:      "A valid remote url, with user, password, organization and repository",
			base:      "https://github.com",
			user:      "user",
			pass:      "pass",
			pathItems: "user/repo",
			expected:  "https://user:pass@github.com/user/repo",
		},
		{
			name:      "local path, no user, no password, organization and repository",
			base:      "/var/temp/dir",
			user:      "",
			pass:      "",
			pathItems: "user/repo",
			expected:  "/var/temp/dir/user/repo",
		},
		{
			name:      "A valid remote url ended with slash, with user, and password, no path",
			base:      "https://github.com/",
			user:      "user",
			pass:      "pass",
			pathItems: "user/repo",
			expected:  "https://user:pass@github.com/user/repo",
		},
	}

	for _, test := range tests {
		base, _ := url.Parse(test.base)
		savedBase := *base
		testURL := remote(base, test.user, test.pass, test.pathItems)
		if test.expected != testURL.String() {
			t.Errorf(`git remote did not match expected remote: expected: "%v" actual: "%v"`, test.expected, testURL)
		}
		if *base != savedBase {
			t.Error("base was modified in git.Remote")
		}
	}
}
