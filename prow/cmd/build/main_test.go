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
	}{{
		name: "forgot cert",
		args: []string{"--tls-private-key-file=k"},
	},
		{
			name: "forgot private key",
			args: []string{"--tls-cert-file=c"},
		},
		{
			name: "works with both private/pub",
			args: []string{"--tls-cert-file=c", "--tls-private-key-file=k"},
			expected: &options{
				cert:       "c",
				privateKey: "k",
			},
		},
		{
			name:     "defaults work",
			expected: &options{},
		}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			flags := flag.NewFlagSet(tc.name, flag.ContinueOnError)
			var actual options
			switch err := actual.parse(flags, tc.args); {
			case tc.expected == nil:
				if err == nil {
					t.Error("failed to receive an error")
				}
			case err != nil:
				t.Errorf("unexpected error: %v", err)
			case !reflect.DeepEqual(&actual, tc.expected):
				t.Errorf("actual %#v != expected %#v", actual, *tc.expected)
			}
		})
	}
}
