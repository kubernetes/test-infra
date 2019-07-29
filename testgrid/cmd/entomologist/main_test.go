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
	"flag"
	"io/ioutil"
	"k8s.io/test-infra/prow/flagutil"
	"os"
	"reflect"
	"strconv"
	"testing"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/testgrid/issue_state"
)

func TestOptions(t *testing.T) {

	tmpdir, err := ioutil.TempDir("", "tmpdir")
	if err != nil {
		t.Errorf("Unexpected error while creating temprorary dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)
	fd, err := ioutil.TempFile("", "tmpfile")
	if err != nil {
		t.Errorf("Unexpected error while creating temprorary file: %v", err)
	}
	tmpfile := fd.Name()
	defer os.Remove(tmpfile)

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
			args: []string{"--github-org=testorg", "--github-repo=testrepo", "--output=gs://foo/bar"},
		},
		{
			name: "missing output, reject",
			args: []string{"--github-org=testorg", "--github-repo=testrepo"},
		},
		{
			name: "both oneshot and poll-interval, reject",
			args: []string{
				"--github-org=testorg",
				"--github-repo=testrepo",
				"--output=gs://foo/bar",
				"--gcs-credentials-file=/usr/foo/creds.json",
				"--oneshot",
				"--poll-interval=1h",
			},
		},
		{
			name: "required options with gcs bucket output",
			args: []string{
				"--github-org=testorg",
				"--github-repo=testrepo",
				"--output=gs://foo/bar",
				"--gcs-credentials-file=/usr/foo/creds.json",
			},
			expected: &options{
				organization:   "testorg",
				repository:     "testrepo",
				output:         "gs://foo/bar",
				gcsCredentials: "/usr/foo/creds.json",
			},
		},
		{
			name: "required options with directory output",
			args: []string{
				"--github-org=testorg",
				"--github-repo=testrepo",
				"--output=" + tmpdir,
			},
			expected: &options{
				organization:   "testorg",
				repository:     "testrepo",
				output:         tmpdir,
				gcsCredentials: "",
				pollInterval:   "",
			},
		},
		{
			name: "required options with file output",
			args: []string{
				"--github-org=testorg",
				"--github-repo=testrepo",
				"--output=" + tmpfile,
			},
			expected: &options{
				organization:   "testorg",
				repository:     "testrepo",
				output:         tmpfile,
				gcsCredentials: "",
				pollInterval:   "",
			},
		},
		{
			name: "oneshot with many options",
			args: []string{
				"--github-org=testorg",
				"--github-repo=testrepo",
				"--github-endpoint=https://127.0.0.1:8888",
				"--github-token-path=/usr/foo/tkpath",
				"--output=gs://foo/bar",
				"--gcs-credentials-file=/usr/foo/creds.json",
				"--oneshot",
			},
			expected: &options{
				organization:   "testorg",
				repository:     "testrepo",
				output:         "gs://foo/bar",
				gcsCredentials: "/usr/foo/creds.json",
				oneshot:        true,
				pollInterval:   "",
			},
		},
		{
			name: "poll-time with many options",
			args: []string{
				"--github-org=testorg",
				"--github-repo=testrepo",
				"--github-endpoint=https://127.0.0.1:8888",
				"--github-token-path=/usr/foo/tkpath",
				"--output=gs://foo/bar",
				"--gcs-credentials-file=/usr/foo/creds.json",
				"--poll-interval=1h",
			},
			expected: &options{
				organization:   "testorg",
				repository:     "testrepo",
				output:         "gs://foo/bar",
				gcsCredentials: "/usr/foo/creds.json",
				oneshot:        false,
				pollInterval:   "1h",
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

func TestPinIssues(t *testing.T) {
	testCases := []struct {
		issue         github.Issue
		issueComments []github.IssueComment
		expectedPins  []string
	}{
		{
			issue: github.Issue{
				Title:  "Body With No Target; ignore",
				Number: 2,
				Body:   "Don't capture this target: it's wrong",
			},
		},
		{
			issue: github.Issue{
				Title:  "Body with Single Target",
				Number: 3,
				Body:   "target:[Organization] Testing Name [sig-testing]",
			},
			expectedPins: []string{
				"[Organization] Testing Name [sig-testing]",
			},
		},
		{
			issue: github.Issue{
				Title:  "Body with Multiple Targets",
				Number: 4,
				Body:   "target://project:test\r\ntarget://project:example",
			},
			expectedPins: []string{
				"//project:test",
				"//project:example",
			},
		},
		{
			issue: github.Issue{
				Title:  "Tolerate Whitespace",
				Number: 5,
				Body:   "target:tolerateWindowsCR\r\ntarget:tolerateLinuxCR\ntarget:\t\ttrim-tabs\t",
			},
			issueComments: []github.IssueComment{
				{Body: "target: trim leading space"},
				{Body: "target:trim trailing space  "},
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
				{Body: "target://project:test"},
			},
			expectedPins: []string{
				"//project:test",
			},
		},
		{
			issue: github.Issue{
				Title:  "Multiple Comments",
				Number: 7,
			},
			issueComments: []github.IssueComment{
				{Body: "target://project:first\r\ntarget://project:second"},
				{Body: "target://project:third"},
				{Body: "This is a false target: it's not at the beginning of the line"},
			},
			expectedPins: []string{
				"//project:first",
				"//project:second",
				"//project:third",
			},
		},
		{
			issue: github.Issue{
				Title:       "Pull Request; ignore",
				Number:      8,
				PullRequest: &struct{}{},
				Body:        "target://project:no",
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
			var o options

			result, err := pinIssues(&g, o)
			if err != nil {
				t.Errorf("Error in Issue Pinning: %e", err)
			}

			if test.expectedPins == nil {
				if len(result.IssueInfo) != 0 {
					t.Errorf("Expected no issue, but got %v", result.IssueInfo)
				}
			} else {
				if len(result.IssueInfo) != 1 {
					t.Errorf("Returned %d issues from one issue: %v", len(result.IssueInfo), result.IssueInfo)
				}

				expected := issue_state.IssueInfo{
					Title:   test.issue.Title,
					IssueId: strconv.Itoa(test.issue.Number),
					RowIds:  test.expectedPins,
				}

				if !reflect.DeepEqual(*result.IssueInfo[0], expected) {
					t.Errorf("Targeter: Failed with %s, got %v, expected %v",
						expected.Title, *result.IssueInfo[0], expected)
				}
			}
		})
	}
}
