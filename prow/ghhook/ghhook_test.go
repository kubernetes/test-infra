/*
Copyright 2020 The Kubernetes Authors.

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

package ghhook

import (
	"errors"
	"flag"
	"fmt"
	"reflect"
	"testing"

	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"
)

func TestGetOptions(t *testing.T) {
	defArgs := map[string][]string{
		"--hmac-path":         {"/fake/hmac-file"},
		"--hook-url":          {"https://not-a-url"},
		"--repo":              {"fake-org/fake-repo"},
		"--github-token-path": {"./testdata/token"},
	}
	cases := []struct {
		name     string
		args     map[string][]string
		expected func(*Options)
		err      bool
	}{
		{
			name: "reject empty --hmac-path",
			args: map[string][]string{
				"--hmac-path": nil,
			},
			err: true,
		},
		{
			name: "reject empty --hook-url",
			args: map[string][]string{
				"--hook-url": nil,
			},
			err: true,
		},
		{
			name: "empty --repo",
			args: map[string][]string{
				"--repo": nil,
			},
			err: true,
		},
		{
			name: "multi repo",
			args: map[string][]string{
				"--repo": {"org1", "org2/repo"},
			},
			expected: func(o *Options) {
				o.Repos = flagutil.NewStrings()
				o.Repos.Set("org1")
				o.Repos.Set("org2/repo")
			},
		},
		{
			name: "full flags",
			args: map[string][]string{
				"--event":   {"this", "that"},
				"--confirm": {"true"},
			},
			expected: func(o *Options) {
				o.Events.Set("this")
				o.Events.Set("that")
				o.Confirm = true
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var args []string
			for k, v := range defArgs {
				if _, ok := tc.args[k]; !ok {
					tc.args[k] = v
				}
			}

			for k, v := range tc.args {
				for _, arg := range v {
					args = append(args, k+"="+arg)
				}
			}

			expected := Options{
				HMACPath: "/fake/hmac-file",
				HookURL:  "https://not-a-url",
				Events:   flagutil.NewStrings(github.AllHookEvents...),
			}
			expected.Repos.Set("fake-org/fake-repo")

			if tc.expected != nil {
				tc.expected(&expected)
			}

			o, err := GetOptions(flag.NewFlagSet("fake-flags", flag.ExitOnError), args)
			if o != nil { // TODO(fejta): github.GitHubOptions not unit testable
				expected.GitHubOptions = o.GitHubOptions
				expected.GitHubHookClient = o.GitHubHookClient
			}
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error %s: %v", args, err)
				}
			case tc.err:
				t.Error("failed to receive an error")
			case !reflect.DeepEqual(*o, expected):
				t.Errorf("%#v != actual %#v", expected, o)
			}
		})
	}
}

func TestFindHook(t *testing.T) {
	const goal = "http://random-url"
	number := 7
	cases := []struct {
		name     string
		hooks    []github.Hook
		expected *int
	}{
		{
			name:  "nil on no match",
			hooks: []github.Hook{{}, {}},
		},
		{
			name: "return matched id",
			hooks: []github.Hook{{
				ID: number,
				Config: github.HookConfig{
					URL: goal,
				},
			}},
			expected: &number,
		},
	}

	for _, tc := range cases {
		actual := findHook(tc.hooks, goal)
		if !reflect.DeepEqual(actual, tc.expected) {
			t.Errorf("%s: expected %v != actual %v", tc.name, tc.expected, actual)
		}
	}
}

func TestReconcileHook(t *testing.T) {
	const goal = "http://goal-url"
	const targetId = 1000
	secret := "ingredient"
	j := "json"
	cases := []struct {
		name         string
		org          string
		hooks        []github.Hook
		expectCreate bool
		expectEdit   bool
		expectDelete bool
		err          bool
	}{
		{
			name: "fail on list error",
			org:  "list-error",
			err:  true,
		},
		{
			name: "fail on create error",
			org:  "create-error",
			err:  true,
		},
		{
			name: "fail on edit error",
			org:  "edit-error",
			hooks: []github.Hook{
				{
					Config: github.HookConfig{
						URL: goal,
					},
				},
			},
			err: true,
		},
		{
			name: "fail on delete error",
			org:  "delete-error",
			hooks: []github.Hook{
				{
					Config: github.HookConfig{
						URL: goal,
					},
				},
			},
			err: true,
		},
		{
			name:         "create when empty",
			expectCreate: true,
		},
		{
			name: "create when no match",
			hooks: []github.Hook{
				{
					ID: targetId + 6666,
					Config: github.HookConfig{
						URL: "http://random-url",
					},
				},
			},
			expectCreate: true,
		},
		{
			name: "edit exiting item",
			hooks: []github.Hook{
				{
					ID: targetId,
					Config: github.HookConfig{
						URL: goal,
					},
				},
			},
			expectEdit: true,
		},
		{
			name: "delete exiting item",
			hooks: []github.Hook{
				{
					ID: targetId,
					Config: github.HookConfig{
						URL: goal,
					},
				},
			},
			expectDelete: true,
		},
	}

	for _, tc := range cases {
		var created, edited, deleted *github.HookRequest
		ch := changer{
			lister: func(org string) ([]github.Hook, error) {
				if org == "list-error" {
					return nil, errors.New("inject list error")
				}
				return tc.hooks, nil
			},
			editor: func(org string, id int, req github.HookRequest) error {
				if org == "edit-error" {
					return errors.New("inject edit error")
				}
				if id != targetId {
					return fmt.Errorf("id %d != expected %d", id, targetId)
				}
				edited = &req
				return nil
			},
			creator: func(org string, req github.HookRequest) (int, error) {
				if org == "create-error" {
					return 0, errors.New("inject create error")
				}
				if created != nil {
					return 0, errors.New("already created")
				}
				created = &req
				return targetId, nil
			},
			deletor: func(org string, id int, req github.HookRequest) error {
				if org == "delete-error" {
					return errors.New("inject delete error")
				}
				if deleted != nil {
					return errors.New("already deleted")
				}
				deleted = &req
				return nil
			},
		}
		req := github.HookRequest{
			Name:   "web",
			Events: []string{"random"},
			Config: &github.HookConfig{
				URL:         goal,
				ContentType: &j,
				Secret:      &secret,
			},
		}

		err := reconcileHook(ch, tc.org, req, &Options{ShouldDelete: tc.expectDelete})
		switch {
		case err != nil:
			if !tc.err {
				t.Errorf("unexpected error: %v", err)
			}
		case tc.err:
			t.Error("failed to receive an error")
		case tc.expectCreate && created == nil:
			t.Error("failed to create")
		case tc.expectEdit && edited == nil:
			t.Error("failed to edit")
		case tc.expectDelete && deleted == nil:
			t.Error("failed to delete")
		case created != nil && !reflect.DeepEqual(req, *created):
			t.Errorf("created %#v != expected %#v", *created, req)
		case edited != nil && !reflect.DeepEqual(req, *edited):
			t.Errorf("edited %#v != expected %#v", *edited, req)
		case deleted != nil && !reflect.DeepEqual(req, *deleted):
			t.Errorf("deleted %#v != expected %#v", *deleted, req)
		}
	}
}
