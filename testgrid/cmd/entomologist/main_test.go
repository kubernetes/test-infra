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

package main

import (
	"context"
	"flag"
	"fmt"
	"reflect"
	"testing"

	"k8s.io/test-infra/pkg/io"
	"k8s.io/test-infra/testgrid/config"
	"k8s.io/test-infra/testgrid/issue_state"

	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

func Test_options(t *testing.T) {

	testCases := []struct {
		name     string
		args     []string
		expected *options
	}{
		{
			name: "no args, reject",
			args: []string{},
		},
		{
			name: "missing GCS credentials when GCS provided as output, reject",
			args: []string{"--repos=testorg/testrepo", "--output=gs://foo/bar", "--config=gs://some/config"},
		},
		{
			name: "missing output, reject",
			args: []string{"--repos=testorg/testrepo", "--config=gs://some/config"},
		},
		{
			name: "no config, reject",
			args: []string{"--repos=testorg/testrepo", "--output=foo/bar"},
		},
		{
			name: "both oneshot and poll-interval, reject",
			args: []string{
				"--repos=testorg/testrepo",
				"--output=gs://foo/bar",
				"--config=gs://some/config",
				"--gcs-credentials-file=/usr/foo/creds.json",
				"--oneshot",
				"--poll-interval=1h",
			},
		},
		{
			name: "required options with gcs bucket output",
			args: []string{
				"--repos=testorg/testrepo",
				"--output=gs://foo/bar",
				"--config=gs://some/config",
				"--gcs-credentials-file=/usr/foo/creds.json",
			},
			expected: &options{
				repositories:   []string{"testorg/testrepo"},
				output:         "gs://foo/bar",
				configPath:     "gs://some/config",
				gcsCredentials: "/usr/foo/creds.json",
			},
		},
		{
			name: "deprecated repo options",
			args: []string{
				"--github-org=testorg",
				"--github-repo=testrepo",
				"--output=gs://foo/bar",
				"--config=gs://some/config",
				"--gcs-credentials-file=/usr/foo/creds.json",
			},
			expected: &options{
				organization:   "testorg",
				repository:     "testrepo",
				repositories:   []string{"testorg/testrepo"},
				output:         "gs://foo/bar",
				configPath:     "gs://some/config",
				gcsCredentials: "/usr/foo/creds.json",
			},
		},
		{
			name: "several repos with directory output",
			args: []string{
				"--repos=org1/repo1,org2/repo2,org3/repo3",
				"--output=/file/path",
				"--config=gs://some/config",
			},
			expected: &options{
				repositories:   []string{"org1/repo1", "org2/repo2", "org3/repo3"},
				output:         "/file/path",
				configPath:     "gs://some/config",
				gcsCredentials: "",
				pollInterval:   "",
			},
		},
		{
			name: "oneshot with many options",
			args: []string{
				"--repos=testorg/testrepo",
				"--github-endpoint=https://127.0.0.1:8888",
				"--github-token-path=/usr/foo/tkpath",
				"--output=gs://foo/bar",
				"--config=gs://some/config",
				"--gcs-credentials-file=/usr/foo/creds.json",
				"--rate-limit=99",
				"--oneshot",
			},
			expected: &options{
				repositories:   []string{"testorg/testrepo"},
				output:         "gs://foo/bar",
				configPath:     "gs://some/config",
				gcsCredentials: "/usr/foo/creds.json",
				oneshot:        true,
				rateLimit:      99,
			},
		},
		{
			name: "poll-time with many options",
			args: []string{
				"--repos=testorg/testrepo",
				"--github-endpoint=https://127.0.0.1:8888",
				"--github-token-path=/usr/foo/tkpath",
				"--output=gs://foo/bar",
				"--config=gs://some/config",
				"--gcs-credentials-file=/usr/foo/creds.json",
				"--poll-interval=1h",
				"--rate-limit=450",
			},
			expected: &options{
				repositories:   []string{"testorg/testrepo"},
				output:         "gs://foo/bar",
				configPath:     "gs://some/config",
				gcsCredentials: "/usr/foo/creds.json",
				oneshot:        false,
				pollInterval:   "1h",
				rateLimit:      450,
			},
		},
	}

	for _, test := range testCases {
		flags := flag.NewFlagSet(test.name, flag.ContinueOnError)
		var actual options
		err := actual.parseArgs(flags, test.args)
		actual.github = flagutil.GitHubOptions{}
		switch {
		case err == nil && test.expected == nil:
			t.Errorf("%s: failed to return an error", test.name)
		case err != nil && test.expected != nil:
			t.Errorf("%s: unexpected error: %v", test.name, err)
		case test.expected != nil && !reflect.DeepEqual(*test.expected, actual):
			t.Errorf("%s: actual %#v != expected %#v", test.name, actual, *test.expected)
		}
	}
}

func Test_pinIssues(t *testing.T) {
	testCases := []struct {
		issue         github.Issue
		issueComments []github.IssueComment
		testGroups    []string
		expectedPins  []string
	}{
		{
			issue: github.Issue{
				Title:  "Body With No Target; ignore",
				Number: 2,
				Body:   "Don't capture this pin: it's wrong",
			},
		},
		{
			issue: github.Issue{
				Title:  "Body with Single Target",
				Number: 3,
				Body:   "Pin:[Organization] Testing Name [sig-testing]",
			},
			testGroups: []string{
				"[Organization] Testing Name [sig-testing]",
			},
			expectedPins: []string{
				"[Organization] Testing Name [sig-testing]",
			},
		},
		{
			issue: github.Issue{
				Title:  "Body with Multiple Targets",
				Number: 4,
				Body:   "Pin:project:test\r\nPin:project:example",
			},
			testGroups: []string{
				"project:test",
				"project:example",
			},
			expectedPins: []string{
				"project:test",
				"project:example",
			},
		},
		{
			issue: github.Issue{
				Title:  "Tolerate Whitespace and Capitalization",
				Number: 5,
				Body:   "Pin:tolerateWindowsCR\r\npin:tolerateLinuxCR\npin:\t\ttrim-tabs\t",
			},
			issueComments: []github.IssueComment{
				{Body: "Pin: trim leading space"},
				{Body: "pin:trim trailing space  "},
			},
			testGroups: []string{
				"tolerateWindowsCR",
				"tolerateLinuxCR",
				"trim-tabs",
				"trim leading space",
				"trim trailing space",
			},
			expectedPins: []string{
				"tolerateWindowsCR",
				"tolerateLinuxCR",
				"trim-tabs",
				"trim leading space",
				"trim trailing space",
			},
		},
		{
			issue: github.Issue{
				Title:  "Target in Comment",
				Number: 6,
			},
			issueComments: []github.IssueComment{
				{Body: "Pin:project:test"},
			},
			testGroups: []string{
				"project:test",
			},
			expectedPins: []string{
				"project:test",
			},
		},
		{
			issue: github.Issue{
				Title:  "Multiple Comments",
				Number: 7,
			},
			issueComments: []github.IssueComment{
				{Body: "Pin:project:first\r\nPin:project:second"},
				{Body: "Pin:project:third"},
				{Body: "This is a false Pin: it's not at the beginning of the line"},
			},
			testGroups: []string{
				"project:first",
				"project:second",
				"project:third",
			},
			expectedPins: []string{
				"project:first",
				"project:second",
				"project:third",
			},
		},
		{
			issue: github.Issue{
				Title:       "Pull Request; ignore",
				Number:      8,
				PullRequest: &struct{}{},
				Body:        "Pin:project:no",
			},
			testGroups: []string{
				"project:no",
			},
		},
		{
			issue: github.Issue{
				Title:  "Target doesn't match testgroup; ignore",
				Number: 9,
				Body:   "Pin:project:first",
			},
			testGroups: []string{
				"project:second",
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.issue.Title, func(t *testing.T) {

			g := fakegithub.FakeClient{
				Issues: map[int]*github.Issue{
					test.issue.Number: &test.issue,
				},
				IssueComments: map[int][]github.IssueComment{
					test.issue.Number: test.issueComments,
				},
			}

			exampleRepo := []string{"repo/org"}

			result := make(map[string]*issue_state.IssueState, 0)
			for _, group := range test.testGroups {
				result[group] = nil
			}

			err := pinIssues(&g, exampleRepo, result, context.Background())
			if err != nil {
				t.Errorf("Error in Issue Pinning: %e", err)
			}

			if test.expectedPins == nil {
				for _, issueState := range result {
					if issueState != nil {
						t.Errorf("Expecting no issue state, but got %v", issueState)
					}
				}
			}

			for _, expectedPin := range test.expectedPins {
				if result[expectedPin] == nil {
					t.Errorf("Expected 1 issue, but got nil test group %s", expectedPin)
				} else if len(result[expectedPin].IssueInfo) != 1 {
					t.Errorf("Expected 1 issue at %s, but got %v", expectedPin, result[expectedPin].IssueInfo)
				} else if result[expectedPin].IssueInfo[0].Title != test.issue.Title {
					t.Errorf("Wrong issue title; got %s, expected %s", result[expectedPin].IssueInfo[0].Title, test.issue.Title)
				}
			}
		})
	}
}

func Test_getTestGroups(t *testing.T) {
	testCases := []struct {
		name     string
		config   *config.Configuration
		expected map[string]*issue_state.IssueState
	}{
		{
			name: "Empty Config; empty map",
		},
		{
			name: "Config; returns groups",
			config: &config.Configuration{
				TestGroups: []*config.TestGroup{
					{Name: "Prime"},
					{Name: "Second"},
				},
			},
			expected: map[string]*issue_state.IssueState{
				"Prime":  nil,
				"Second": nil,
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			result := getTestGroups(test.config)
			if !reflect.DeepEqual(result, test.expected) {
				t.Errorf("Got %v, expected %v", result, test.expected)
			}
		})
	}
}

func Test_writeIssueStates(t *testing.T) {
	testCases := []struct {
		name                string
		newIssueStates      map[string]*issue_state.IssueState
		previousIssueStates []string
		expectWrite         []string
	}{
		{
			name: "No Issue States and no state; no operation",
		},
		{
			name: "Empty Issue States; delete file if it exists",
			newIssueStates: map[string]*issue_state.IssueState{
				"foo": nil,
				"bar": nil,
			},

			previousIssueStates: []string{"/bugs-foo"},
			expectWrite:         []string{"/bugs-foo"},
		},
		{
			name: "New Issue States; create if non-nil",
			newIssueStates: map[string]*issue_state.IssueState{
				"baz": {
					IssueInfo: []*issue_state.IssueInfo{
						{Title: "Issue Title"},
					},
				},
				"quux": nil,
			},
			expectWrite: []string{"/bugs-baz"},
		},
		{
			name: "Existing Issue States; overwrite",
			newIssueStates: map[string]*issue_state.IssueState{
				"baz": {
					IssueInfo: []*issue_state.IssueInfo{
						{Title: "Issue Title"},
					},
				},
			},
			previousIssueStates: []string{"/bugs-baz"},
			expectWrite:         []string{"/bugs-baz"},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			var client fakeClient
			client.readTargets = test.previousIssueStates

			err := writeIssueStates(test.newIssueStates, "", &client, context.Background())
			if err != nil {
				t.Errorf("Unexpected error: %e", err)
			}

			if !reflect.DeepEqual(client.writeTargets, test.expectWrite) {
				t.Errorf("Unexpected writes; got %v, expected %v", client.writeTargets, test.expectWrite)
			}
		})
	}
}

type fakeClient struct {
	readTargets  []string
	writeTargets []string
}

func (f *fakeClient) Reader(ctx context.Context, path string) (io.ReadCloser, error) {
	for _, target := range f.readTargets {
		if target == path {
			return fakeReader{}, nil
		}
	}
	return nil, fmt.Errorf("file not found")
}

func (f *fakeClient) Writer(ctx context.Context, path string) (io.WriteCloser, error) {
	f.writeTargets = append(f.writeTargets, path)
	return fakeWriter{}, nil
}

type fakeReader struct{}

func (fakeReader) Read(p []byte) (n int, err error) { return len(p), nil }
func (fakeReader) Close() error                     { return nil }

type fakeWriter struct{}

func (fakeWriter) Write(p []byte) (n int, err error) { return len(p), nil }
func (fakeWriter) Close() error                      { return nil }
