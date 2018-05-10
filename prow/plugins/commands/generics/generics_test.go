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

package generics

import (
	"reflect"
	"testing"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/commands"
)

func TestAddRemoveCommand(t *testing.T) {
	argsFromMatches := func(matches [][]string) []string {
		args := make([]string, 0, len(matches))
		for _, match := range matches {
			args = append(args, match[2])
		}
		return args
	}

	tcs := []struct {
		name           string
		text           string
		expectedAdd    []string
		expectedRemove []string
	}{
		{
			name: "No command",
			text: "blah blah\n/lgtm\n",
		},
		{
			name:        "Empty add",
			text:        "blah blah\n/do-the-thing\nblah blah",
			expectedAdd: []string{""},
		},
		{
			name:           "Empty remove",
			text:           "blah blah\n/remove-do-the-thing\nblah blah",
			expectedRemove: []string{""},
		},
		{
			name:        "Arged add",
			text:        "blah blah\n/do-the-thing don't mess up\nblah blah",
			expectedAdd: []string{"don't mess up"},
		},
		{
			name:           "Arged remove",
			text:           "blah blah\n/remove-do-the-thing   don't mess up\nblah blah",
			expectedRemove: []string{"don't mess up"},
		},
		{
			name:        "Multiple adds",
			text:        "blah blah\n/do-the-thing don't mess up\nblah blah\n/do-the-thing again",
			expectedAdd: []string{"don't mess up", "again"},
		},
		{
			name:           "Multiple removes",
			text:           "blah blah\n/remove-do-the-thing don't mess up\nblah blah\n/remove-do-the-thing\n",
			expectedRemove: []string{"don't mess up", ""},
		},
		{
			name:           "Add and remove",
			text:           "/lgtm\n/do-the-thing add \n/remove-do-the-thing remove\n/do-the-thing add again\n/remove-do-the-thing remove again",
			expectedAdd:    []string{"add", "add again"},
			expectedRemove: []string{"remove", "remove again"},
		},
	}

	for _, tc := range tcs {
		t.Logf("Running test case: [%s].", tc.name)
		var adder, remover commands.MatchHandler
		adder = func(ctx *commands.Context) error {
			if len(tc.expectedAdd) == 0 {
				t.Error("Did not expect add handler to be called.")
			}
			args := argsFromMatches(ctx.Matches)
			if !reflect.DeepEqual(args, tc.expectedAdd) {
				t.Errorf("Expected add handler to receive args: %q, but got %q.", tc.expectedAdd, args)
			}
			return nil
		}
		remover = func(ctx *commands.Context) error {
			if len(tc.expectedRemove) == 0 {
				t.Error("Did not expect remove handler to be called.")
			}
			args := argsFromMatches(ctx.Matches)
			if !reflect.DeepEqual(args, tc.expectedRemove) {
				t.Errorf("Expected remove handler to receive args: %q, but got %q.", tc.expectedRemove, args)
			}
			return nil
		}
		e := github.GenericCommentEvent{
			Action: github.GenericCommentActionCreated,
			IsPR:   true,
			Body:   tc.text,
		}
		cmd := AddRemoveCommand("do-the-thing", commands.IssueTypePR, adder, remover)
		if err := cmd.GenericCommentHandler(plugins.PluginClient{}, e); err != nil {
			t.Errorf("Unexpected error: %v.", err)
		}
	}
}
