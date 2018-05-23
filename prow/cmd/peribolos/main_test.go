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
)

func TestOptions(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		expected *options
	}{
		{
			name: "missing --config",
			args: []string{"--github-token-path=fake"},
		},
		{
			name: "missing --github-token-path",
			args: []string{"--config-path=fake"},
		},
		{
			name: "bad --github-endpoint",
			args: []string{"--config-path=foo", "--github-token-path=bar", "--github-endpoint=ht!tp://:dumb"},
		},
		{
			name: "minimal",
			args: []string{"--config-path=foo", "--github-token-path=bar"},
			expected: &options{
				config:   "foo",
				token:    "bar",
				endpoint: flagutil.NewStrings(defaultEndpoint),
			},
		},
		{
			name: "full",
			args: []string{"--config-path=foo", "--github-token-path=bar", "--github-endpoint=weird://url", "--confirm=true"},
			expected: &options{
				config:   "foo",
				token:    "bar",
				endpoint: flagutil.NewStrings("weird://url"),
				confirm:  true,
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
			t.Errorf("%s: actual %v != expected %v", tc.name, actual, tc.expected)
		}
	}

}
