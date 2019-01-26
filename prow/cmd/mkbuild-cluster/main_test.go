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
)

func TestOptions(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		expected *options
	}{
		{
			args: []string{"--cluster=foo", "--alias=bar", "--zone=z", "--project=p"},
			expected: &options{
				cluster: "foo",
				alias:   "bar",
				zone:    "z",
				project: "p",
			},
		},
		{
			name: "missing --cluster",
			args: []string{"--alias=bar", "--zone=z", "--project=p"},
		},
		{
			name: "missing --alias",
			args: []string{"--cluster=foo", "--zone=z", "--project=p"},
		},
		{
			name: "missing --zone",
			args: []string{"--cluster=foo", "--alias=bar", "--project=p"},
		},
		{
			name: "--missing --project",
			args: []string{"--cluster=foo", "--alias=bar", "--zone=z"},
		},
		{
			args: []string{
				"--cluster=foo",
				"--alias=bar",
				"--zone=z",
				"--project=p",
				"--account=a",
				"--print-file",
				"--print-entry",
				"--get-client-cert",
				"--change-context",
				"--skip-check",
			},
			expected: &options{
				cluster:       "foo",
				alias:         "bar",
				zone:          "z",
				project:       "p",
				account:       "a",
				skipCheck:     true,
				getClientCert: true,
				changeContext: true,
				printData:     true,
				printEntry:    true,
			},
		},
	}

	for _, tc := range cases {
		flags := flag.NewFlagSet(tc.name, flag.ContinueOnError)
		var actual options
		err := actual.parseArgs(flags, tc.args)
		switch {
		case err == nil && tc.expected == nil:
			t.Errorf("%s: failed to return an error", tc.name)
		case err != nil && tc.expected != nil:
			t.Errorf("%s: unexpected error: %v", tc.name, err)
		case tc.expected != nil && !reflect.DeepEqual(*tc.expected, actual):
			t.Errorf("%s: actual %#v != expected %#v", tc.name, actual, *tc.expected)
		}
	}
}
