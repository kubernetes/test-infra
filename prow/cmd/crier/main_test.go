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

package main

import (
	"flag"
	"reflect"
	"testing"

	"k8s.io/test-infra/prow/flagutil"
	gerritclient "k8s.io/test-infra/prow/gerrit/client"
)

func TestOptions(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		expected *options
	}{
		{
			name: "empty, reject",
			args: []string{},
		},
		{
			name: "gerrit only support one worker",
			args: []string{"--gerrit-workers=99", "--gerrit-projects=foo=bar", "--cookiefile=foobar"},
			expected: &options{
				gerritWorkers:  1,
				cookiefilePath: "foobar",
				gerritProjects: map[string][]string{
					"foo": {"bar"},
				},
			},
		},
		{
			name: "gerrit missing --gerrit-projects, reject",
			args: []string{"--gerrit-workers=5", "--cookiefile=foobar"},
		},
		{
			name: "gerrit missing --cookiefile",
			args: []string{"--gerrit-workers=5", "--gerrit-projects=foo=bar"},
			expected: &options{
				gerritWorkers: 1,
				gerritProjects: map[string][]string{
					"foo": {"bar"},
				},
			},
		},
	}

	for _, tc := range cases {
		flags := flag.NewFlagSet(tc.name, flag.ContinueOnError)
		actual := options{
			gerritProjects: gerritclient.ProjectsFlag{},
		}
		err := actual.parseArgs(flags, tc.args)
		actual.github = flagutil.GitHubOptions{}
		switch {
		case err == nil && tc.expected == nil:
			t.Errorf("%s: failed to return an error", tc.name)
		case err != nil && tc.expected != nil:
			t.Errorf("%s: unexpected error: %v", tc.name, err)
		case tc.expected != nil && !reflect.DeepEqual(*tc.expected, actual):
			t.Errorf("%s: actual %v != expected %v", tc.name, actual, *tc.expected)
		}
	}
}
