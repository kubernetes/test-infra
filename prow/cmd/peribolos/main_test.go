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
	"fmt"
	"reflect"
	"sort"
	"testing"

	"k8s.io/test-infra/prow/config/org"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"

	"k8s.io/apimachinery/pkg/util/sets"
)

func TestOptions(t *testing.T) {
	weirdFlags := flagutil.NewStrings(defaultEndpoint)
	weirdFlags.Set("weird://url") // no error possible
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
			name: "--minAdmins too low",
			args: []string{"--config-path=foo", "--github-token-path=bar", "--min-admins=1"},
		},
		{
			name: "--maximum-removal-delta too high",
			args: []string{"--config-path=foo", "--github-token-path=bar", "--maximum-removal-delta=1.1"},
		},
		{
			name: "--maximum-removal-delta too low",
			args: []string{"--config-path=foo", "--github-token-path=bar", "--maximum-removal-delta=-0.1"},
		},
		{
			name: "maximal delta",
			args: []string{"--config-path=foo", "--github-token-path=bar", "--maximum-removal-delta=1"},
			expected: &options{
				config:       "foo",
				token:        "bar",
				endpoint:     flagutil.NewStrings(defaultEndpoint),
				minAdmins:    defaultMinAdmins,
				requireSelf:  true,
				maximumDelta: 1,
			},
		},
		{
			name: "minimal delta",
			args: []string{"--config-path=foo", "--github-token-path=bar", "--maximum-removal-delta=0"},
			expected: &options{
				config:       "foo",
				token:        "bar",
				endpoint:     flagutil.NewStrings(defaultEndpoint),
				minAdmins:    defaultMinAdmins,
				requireSelf:  true,
				maximumDelta: 0,
			},
		},
		{
			name: "minimal admins",
			args: []string{"--config-path=foo", "--github-token-path=bar", "--min-admins=2"},
			expected: &options{
				config:       "foo",
				token:        "bar",
				endpoint:     flagutil.NewStrings(defaultEndpoint),
				minAdmins:    2,
				requireSelf:  true,
				maximumDelta: defaultDelta,
			},
		},
		{
			name: "minimal",
			args: []string{"--config-path=foo", "--github-token-path=bar"},
			expected: &options{
				config:       "foo",
				token:        "bar",
				endpoint:     flagutil.NewStrings(defaultEndpoint),
				minAdmins:    defaultMinAdmins,
				requireSelf:  true,
				maximumDelta: defaultDelta,
			},
		},
		{
			name: "full",
			args: []string{"--config-path=foo", "--github-token-path=bar", "--github-endpoint=weird://url", "--confirm=true", "--require-self=false"},
			expected: &options{
				config:       "foo",
				token:        "bar",
				endpoint:     weirdFlags,
				confirm:      true,
				requireSelf:  false,
				minAdmins:    defaultMinAdmins,
				maximumDelta: defaultDelta,
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
			t.Errorf("%s: actual %v != expected %v", tc.name, actual, *tc.expected)
		}
	}
}

type fakeClient struct {
	admins     sets.String
	members    sets.String
	removed    sets.String
	newAdmins  sets.String
	newMembers sets.String
}

func (c *fakeClient) BotName() (string, error) {
	return "me", nil
}

func (c *fakeClient) ListOrgMembers(org, role string) ([]github.TeamMember, error) {
	var ret []github.TeamMember
	switch role {
	case github.RoleMember:
		for m := range c.members {
			ret = append(ret, github.TeamMember{Login: m})
		}
		return ret, nil
	case github.RoleAdmin:
		for m := range c.admins {
			ret = append(ret, github.TeamMember{Login: m})
		}
		return ret, nil
	default:
		// RoleAll: implmenent when/if necessary
		return nil, fmt.Errorf("bad role: %s", role)
	}
}

func (c *fakeClient) RemoveOrgMembership(org, user string) error {
	if user == "fail" {
		return fmt.Errorf("injected failure for %s", user)
	}
	c.removed.Insert(user)
	c.admins.Delete(user)
	c.members.Delete(user)
	return nil
}

func (c *fakeClient) UpdateOrgMembership(org, user string, admin bool) (*github.OrgMembership, error) {
	if user == "fail" {
		return nil, fmt.Errorf("injected failure for %s", user)
	}
	var state string
	if c.members.Has(user) || c.admins.Has(user) {
		state = github.StateActive
	} else {
		state = github.StatePending
	}
	var role string
	if admin {
		c.newAdmins.Insert(user)
		c.admins.Insert(user)
		role = github.RoleAdmin
	} else {
		c.newMembers.Insert(user)
		c.members.Insert(user)
		role = github.RoleMember
	}
	return &github.OrgMembership{
		Role:  role,
		State: state,
	}, nil
}

func TestConfigureOrgMembers(t *testing.T) {
	cases := []struct {
		name       string
		opt        options
		config     org.Config
		admins     []string
		members    []string
		err        bool
		remove     []string
		addAdmins  []string
		addMembers []string
	}{
		{
			name: "too few admins",
			opt: options{
				minAdmins: 5,
			},
			config: org.Config{
				Admins: []string{"joe"},
			},
			err: true,
		},
		{
			name: "remove too many admins",
			opt: options{
				maximumDelta: 0.3,
			},
			config: org.Config{
				Admins: []string{"keep", "me"},
			},
			admins: []string{"a", "b", "c", "keep"},
			err:    true,
		},
		{
			name: "forgot to add self",
			opt: options{
				requireSelf: true,
			},
			config: org.Config{
				Admins: []string{"other"},
			},
			err: true,
		},
		{
			name: "forgot to add required admins",
			opt: options{
				requiredAdmins: flagutil.NewStrings("francis"),
			},
			err: true,
		},
		{
			name:   "can remove self with flag",
			config: org.Config{},
			opt: options{
				maximumDelta: 1,
			},
			admins: []string{"me"},
			remove: []string{"me"},
		},
		{
			name: "forgot to remove duplicate entry",
			config: org.Config{
				Admins:  []string{"me"},
				Members: []string{"me"},
			},
			err: true,
		},
		{
			name:   "removal fails",
			admins: []string{"fail"},
			err:    true,
		},
		{
			name: "adding admin fails",
			config: org.Config{
				Admins: []string{"fail"},
			},
			err: true,
		},
		{
			name: "adding member fails",
			config: org.Config{
				Members: []string{"fail"},
			},
			err: true,
		},
		{
			name: "promote to admin",
			config: org.Config{
				Admins: []string{"promote"},
			},
			members:   []string{"promote"},
			addAdmins: []string{"promote"},
		},
		{
			name: "downgrade to member",
			config: org.Config{
				Members: []string{"downgrade"},
			},
			admins:     []string{"downgrade"},
			addMembers: []string{"downgrade"},
		},
		{
			name: "some of everything",
			config: org.Config{
				Admins:  []string{"keep-admin", "new-admin"},
				Members: []string{"keep-member", "new-member"},
			},
			opt: options{
				maximumDelta: 0.5,
			},
			admins:     []string{"keep-admin", "drop-admin"},
			members:    []string{"keep-member", "drop-member"},
			remove:     []string{"drop-admin", "drop-member"},
			addMembers: []string{"new-member"},
			addAdmins:  []string{"new-admin"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fc := &fakeClient{
				admins:     sets.NewString(tc.admins...),
				members:    sets.NewString(tc.members...),
				removed:    sets.String{},
				newAdmins:  sets.String{},
				newMembers: sets.String{},
			}
			err := configureOrgMembers(tc.opt, fc, "random-org", tc.config)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("Unexpected error: %v", err)
				}
			case tc.err:
				t.Errorf("Failed to receive error")
			default:
				if err := cmpLists(tc.remove, fc.removed.List()); err != nil {
					t.Errorf("Wrong users removed: %v", err)
				} else if err := cmpLists(tc.addMembers, fc.newMembers.List()); err != nil {
					t.Errorf("Wrong members added: %v", err)
				} else if err := cmpLists(tc.addAdmins, fc.newAdmins.List()); err != nil {
					t.Errorf("Wrong admins added: %v", err)
				}
			}
		})
	}
}

func cmpLists(a, b []string) error {
	if a == nil {
		a = []string{}
	}
	if b == nil {
		b = []string{}
	}
	sort.Strings(a)
	sort.Strings(b)
	if !reflect.DeepEqual(a, b) {
		return fmt.Errorf("%v != %v", a, b)
	}
	return nil
}
