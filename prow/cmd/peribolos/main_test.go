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
	"errors"
	"flag"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/test-infra/prow/config/org"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"

	"k8s.io/apimachinery/pkg/util/sets"
)

func TestOptions(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		expected *options
	}{
		{
			name: "missing --config",
			args: []string{},
		},
		{
			name: "bad --github-endpoint",
			args: []string{"--config-path=foo", "--github-endpoint=ht!tp://:dumb"},
		},
		{
			name: "--minAdmins too low",
			args: []string{"--config-path=foo", "--min-admins=1"},
		},
		{
			name: "--maximum-removal-delta too high",
			args: []string{"--config-path=foo", "--maximum-removal-delta=1.1"},
		},
		{
			name: "--maximum-removal-delta too low",
			args: []string{"--config-path=foo", "--maximum-removal-delta=-0.1"},
		},
		{
			name: "reject --dump-full-config without --dump",
			args: []string{"--config-path=foo", "--dump-full-config"},
		},
		{
			name: "maximal delta",
			args: []string{"--config-path=foo", "--maximum-removal-delta=1"},
			expected: &options{
				config:       "foo",
				minAdmins:    defaultMinAdmins,
				requireSelf:  true,
				maximumDelta: 1,
				logLevel:     "info",
			},
		},
		{
			name: "minimal delta",
			args: []string{"--config-path=foo", "--maximum-removal-delta=0"},
			expected: &options{
				config:       "foo",
				minAdmins:    defaultMinAdmins,
				requireSelf:  true,
				maximumDelta: 0,
				logLevel:     "info",
			},
		},
		{
			name: "minimal admins",
			args: []string{"--config-path=foo", "--min-admins=2"},
			expected: &options{
				config:       "foo",
				minAdmins:    2,
				requireSelf:  true,
				maximumDelta: defaultDelta,
				logLevel:     "info",
			},
		},
		{
			name: "reject dump and confirm",
			args: []string{"--confirm", "--dump=frogger"},
		},
		{
			name: "reject dump and config-path",
			args: []string{"--config-path=foo", "--dump=frogger"},
		},
		{
			name: "reject --fix-team-members without --fix-teams",
			args: []string{"--config-path=foo", "--fix-team-members"},
		},
		{
			name: "allow dump without config",
			args: []string{"--dump=frogger"},
			expected: &options{
				minAdmins:    defaultMinAdmins,
				requireSelf:  true,
				maximumDelta: defaultDelta,
				dump:         "frogger",
				logLevel:     "info",
			},
		},
		{
			name: "minimal",
			args: []string{"--config-path=foo"},
			expected: &options{
				config:       "foo",
				minAdmins:    defaultMinAdmins,
				requireSelf:  true,
				maximumDelta: defaultDelta,
				logLevel:     "info",
			},
		},
		{
			name: "full",
			args: []string{"--config-path=foo", "--github-token-path=bar", "--github-endpoint=weird://url", "--confirm=true", "--require-self=false", "--dump=", "--fix-org", "--fix-org-members", "--fix-teams", "--fix-team-members", "--log-level=debug"},
			expected: &options{
				config:         "foo",
				confirm:        true,
				requireSelf:    false,
				minAdmins:      defaultMinAdmins,
				maximumDelta:   defaultDelta,
				fixOrg:         true,
				fixOrgMembers:  true,
				fixTeams:       true,
				fixTeamMembers: true,
				logLevel:       "debug",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			flags := flag.NewFlagSet(tc.name, flag.ContinueOnError)
			var actual options
			err := actual.parseArgs(flags, tc.args)
			actual.github = flagutil.GitHubOptions{}
			switch {
			case err == nil && tc.expected == nil:
				t.Errorf("%s: failed to return an error", tc.name)
			case err != nil && tc.expected != nil:
				t.Errorf("%s: unexpected error: %v", tc.name, err)
			case tc.expected != nil && !reflect.DeepEqual(*tc.expected, actual):
				t.Errorf("%s: got incorrect options: %v", tc.name, cmp.Diff(actual, *tc.expected, cmp.AllowUnexported(options{}, flagutil.Strings{}, flagutil.GitHubOptions{})))
			}
		})
	}
}

type fakeClient struct {
	orgMembers sets.Set[string]
	admins     sets.Set[string]
	invitees   sets.Set[string]
	members    sets.Set[string]
	removed    sets.Set[string]
	newAdmins  sets.Set[string]
	newMembers sets.Set[string]
}

func (c *fakeClient) BotUser() (*github.UserData, error) {
	return &github.UserData{Login: "me"}, nil
}

func (c fakeClient) makeMembers(people sets.Set[string]) []github.TeamMember {
	var ret []github.TeamMember
	for p := range people {
		ret = append(ret, github.TeamMember{Login: p})
	}
	return ret
}

func (c *fakeClient) ListOrgMembers(org, role string) ([]github.TeamMember, error) {
	switch role {
	case github.RoleMember:
		return c.makeMembers(c.members), nil
	case github.RoleAdmin:
		return c.makeMembers(c.admins), nil
	default:
		// RoleAll: implement when/if necessary
		return nil, fmt.Errorf("bad role: %s", role)
	}
}

func (c *fakeClient) ListOrgInvitations(org string) ([]github.OrgInvitation, error) {
	var ret []github.OrgInvitation
	for p := range c.invitees {
		if p == "fail" {
			return nil, errors.New("injected list org invitations failure")
		}
		ret = append(ret, github.OrgInvitation{
			TeamMember: github.TeamMember{
				Login: p,
			},
		})
	}
	return ret, nil
}

func (c *fakeClient) RemoveOrgMembership(org, user string) error {
	if user == "fail" {
		return errors.New("injected remove org membership failure")
	}
	c.removed.Insert(user)
	c.admins.Delete(user)
	c.members.Delete(user)
	return nil
}

func (c *fakeClient) UpdateOrgMembership(org, user string, admin bool) (*github.OrgMembership, error) {
	if user == "fail" {
		return nil, errors.New("injected update org failure")
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
		Membership: github.Membership{
			Role:  role,
			State: state,
		},
	}, nil
}

func (c *fakeClient) ListTeamMembersBySlug(org, teamSlug, role string) ([]github.TeamMember, error) {
	if teamSlug != configuredTeamSlug {
		return nil, fmt.Errorf("only team: %s supported, not %s", configuredTeamSlug, teamSlug)
	}
	switch role {
	case github.RoleMember:
		return c.makeMembers(c.members), nil
	case github.RoleMaintainer:
		return c.makeMembers(c.admins), nil
	default:
		return nil, fmt.Errorf("fake does not support: %s", role)
	}
}

func (c *fakeClient) ListTeamInvitationsBySlug(org, teamSlug string) ([]github.OrgInvitation, error) {
	if teamSlug != configuredTeamSlug {
		return nil, fmt.Errorf("only team: %s supported, not %s", configuredTeamSlug, teamSlug)
	}
	var ret []github.OrgInvitation
	for p := range c.invitees {
		if p == "fail" {
			return nil, errors.New("injected list org invitations failure")
		}
		ret = append(ret, github.OrgInvitation{
			TeamMember: github.TeamMember{
				Login: p,
			},
		})
	}
	return ret, nil
}

const configuredTeamSlug = "team-slug"

func (c *fakeClient) UpdateTeamMembershipBySlug(org, teamSlug, user string, maintainer bool) (*github.TeamMembership, error) {
	if teamSlug != configuredTeamSlug {
		return nil, fmt.Errorf("only team: %s supported, not %s", configuredTeamSlug, teamSlug)
	}
	if user == "fail" {
		return nil, fmt.Errorf("injected failure for %s", user)
	}
	var state string
	if c.orgMembers.Has(user) || len(c.orgMembers) == 0 {
		state = github.StateActive
	} else {
		state = github.StatePending
	}
	var role string
	if maintainer {
		c.newAdmins.Insert(user)
		c.admins.Insert(user)
		role = github.RoleMaintainer
	} else {
		c.newMembers.Insert(user)
		c.members.Insert(user)
		role = github.RoleMember
	}
	return &github.TeamMembership{
		Membership: github.Membership{
			Role:  role,
			State: state,
		},
	}, nil
}

func (c *fakeClient) RemoveTeamMembershipBySlug(org, teamSlug, user string) error {
	if teamSlug != configuredTeamSlug {
		return fmt.Errorf("only team: %s supported, not %s", configuredTeamSlug, teamSlug)
	}
	if user == "fail" {
		return fmt.Errorf("injected failure for %s", user)
	}
	c.removed.Insert(user)
	c.admins.Delete(user)
	c.members.Delete(user)
	return nil
}

func TestConfigureMembers(t *testing.T) {
	cases := []struct {
		name     string
		want     memberships
		have     memberships
		remove   sets.Set[string]
		members  sets.Set[string]
		supers   sets.Set[string]
		invitees sets.Set[string]
		err      bool
	}{
		{
			name: "forgot to remove duplicate entry",
			want: memberships{
				members: sets.New[string]("me"),
				super:   sets.New[string]("me"),
			},
			err: true,
		},
		{
			name: "removal fails",
			have: memberships{
				members: sets.New[string]("fail"),
			},
			err: true,
		},
		{
			name: "adding admin fails",
			want: memberships{
				super: sets.New[string]("fail"),
			},
			err: true,
		},
		{
			name: "adding member fails",
			want: memberships{
				members: sets.New[string]("fail"),
			},
			err: true,
		},
		{
			name: "promote to admin",
			have: memberships{
				members: sets.New[string]("promote"),
			},
			want: memberships{
				super: sets.New[string]("promote"),
			},
			supers: sets.New[string]("promote"),
		},
		{
			name: "downgrade to member",
			have: memberships{
				super: sets.New[string]("downgrade"),
			},
			want: memberships{
				members: sets.New[string]("downgrade"),
			},
			members: sets.New[string]("downgrade"),
		},
		{
			name: "some of everything",
			have: memberships{
				super:   sets.New[string]("keep-admin", "drop-admin"),
				members: sets.New[string]("keep-member", "drop-member"),
			},
			want: memberships{
				members: sets.New[string]("keep-member", "new-member"),
				super:   sets.New[string]("keep-admin", "new-admin"),
			},
			remove:  sets.New[string]("drop-admin", "drop-member"),
			members: sets.New[string]("new-member"),
			supers:  sets.New[string]("new-admin"),
		},
		{
			name: "ensure case insensitivity",
			have: memberships{
				super:   sets.New[string]("lower"),
				members: sets.New[string]("UPPER"),
			},
			want: memberships{
				super:   sets.New[string]("Lower"),
				members: sets.New[string]("UpPeR"),
			},
		},
		{
			name: "remove invites for those not in org config",
			have: memberships{
				members: sets.New[string]("member-one", "member-two"),
			},
			want: memberships{
				members: sets.New[string]("member-one", "member-two"),
			},
			remove:   sets.New[string]("member-three"),
			invitees: sets.New[string]("member-three"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			removed := sets.Set[string]{}
			members := sets.Set[string]{}
			supers := sets.Set[string]{}
			adder := func(user string, super bool) error {
				if user == "fail" {
					return fmt.Errorf("injected adder failure for %s", user)
				}
				if super {
					supers.Insert(user)
				} else {
					members.Insert(user)
				}
				return nil
			}

			remover := func(user string) error {
				if user == "fail" {
					return fmt.Errorf("injected remover failure for %s", user)
				}
				removed.Insert(user)
				return nil
			}

			err := configureMembers(tc.have, tc.want, tc.invitees, adder, remover)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("Unexpected error: %v", err)
				}
			case tc.err:
				t.Errorf("Failed to receive error")
			default:
				if err := cmpLists(sets.List(tc.remove), sets.List(removed)); err != nil {
					t.Errorf("Wrong users removed: %v", err)
				} else if err := cmpLists(sets.List(tc.members), sets.List(members)); err != nil {
					t.Errorf("Wrong members added: %v", err)
				} else if err := cmpLists(sets.List(tc.supers), sets.List(supers)); err != nil {
					t.Errorf("Wrong supers added: %v", err)
				}
			}
		})
	}
}

func TestConfigureOrgMembers(t *testing.T) {
	cases := []struct {
		name        string
		opt         options
		config      org.Config
		admins      []string
		members     []string
		invitations []string
		err         bool
		remove      []string
		addAdmins   []string
		addMembers  []string
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
				requireSelf:  false,
			},
			admins: []string{"me"},
			remove: []string{"me"},
		},
		{
			name: "reject same person with both roles",
			config: org.Config{
				Admins:  []string{"me"},
				Members: []string{"me"},
			},
			err: true,
		},
		{
			name:   "github remove rpc fails",
			admins: []string{"fail"},
			err:    true,
		},
		{
			name: "github add rpc fails",
			config: org.Config{
				Admins: []string{"fail"},
			},
			err: true,
		},
		{
			name: "require team member to be org member",
			config: org.Config{
				Teams: map[string]org.Team{
					"group": {
						Members: []string{"non-member"},
					},
				},
			},
			err: true,
		},
		{
			name: "require team maintainer to be org member",
			config: org.Config{
				Teams: map[string]org.Team{
					"group": {
						Maintainers: []string{"non-member"},
					},
				},
			},
			err: true,
		},
		{
			name: "require team members with upper name to be org member",
			config: org.Config{
				Teams: map[string]org.Team{
					"foo": {
						Members: []string{"Me"},
					},
				},
				Members: []string{"Me"},
			},
			members: []string{"Me"},
		},
		{
			name: "require team maintainer with upper name to be org member",
			config: org.Config{
				Teams: map[string]org.Team{
					"foo": {
						Maintainers: []string{"Me"},
					},
				},
				Admins: []string{"Me"},
			},
			admins: []string{"Me"},
		},
		{
			name: "disallow duplicate names",
			config: org.Config{
				Teams: map[string]org.Team{
					"duplicate": {},
					"other": {
						Previously: []string{"duplicate"},
					},
				},
			},
			err: true,
		},
		{
			name: "disallow duplicate names (single team)",
			config: org.Config{
				Teams: map[string]org.Team{
					"foo": {
						Previously: []string{"foo"},
					},
				},
			},
			err: true,
		},
		{
			name: "trivial case works",
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
		{
			name: "do not reinvite",
			config: org.Config{
				Admins:  []string{"invited-admin"},
				Members: []string{"invited-member"},
			},
			invitations: []string{"invited-admin", "invited-member"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fc := &fakeClient{
				admins:     sets.New[string](tc.admins...),
				members:    sets.New[string](tc.members...),
				removed:    sets.Set[string]{},
				newAdmins:  sets.Set[string]{},
				newMembers: sets.Set[string]{},
			}

			err := configureOrgMembers(tc.opt, fc, fakeOrg, tc.config, sets.New[string](tc.invitations...))
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("Unexpected error: %v", err)
				}
			case tc.err:
				t.Errorf("Failed to receive error")
			default:
				if err := cmpLists(tc.remove, sets.List(fc.removed)); err != nil {
					t.Errorf("Wrong users removed: %v", err)
				} else if err := cmpLists(tc.addMembers, sets.List(fc.newMembers)); err != nil {
					t.Errorf("Wrong members added: %v", err)
				} else if err := cmpLists(tc.addAdmins, sets.List(fc.newAdmins)); err != nil {
					t.Errorf("Wrong admins added: %v", err)
				}
			}
		})
	}
}

type fakeTeamClient struct {
	teams map[string]github.Team
	max   int
}

func makeFakeTeamClient(teams ...github.Team) *fakeTeamClient {
	fc := fakeTeamClient{
		teams: map[string]github.Team{},
	}
	for _, t := range teams {
		fc.teams[t.Slug] = t
		if t.ID >= fc.max {
			fc.max = t.ID + 1
		}
	}
	return &fc
}

const fakeOrg = "random-org"

func (c *fakeTeamClient) CreateTeam(org string, team github.Team) (*github.Team, error) {
	if org != fakeOrg {
		return nil, fmt.Errorf("org must be %s, not %s", fakeOrg, org)
	}
	if team.Name == "fail" {
		return nil, errors.New("injected CreateTeam error")
	}
	c.max++
	team.ID = c.max
	c.teams[team.Slug] = team
	return &team, nil

}

func (c *fakeTeamClient) ListTeams(name string) ([]github.Team, error) {
	if name == "fail" {
		return nil, errors.New("injected ListTeams error")
	}
	var teams []github.Team
	for _, t := range c.teams {
		teams = append(teams, t)
	}
	return teams, nil
}

func (c *fakeTeamClient) DeleteTeamBySlug(org, teamSlug string) error {
	switch _, ok := c.teams[teamSlug]; {
	case !ok:
		return fmt.Errorf("not found %s", teamSlug)
	case teamSlug == "":
		return errors.New("injected DeleteTeam error")
	}
	delete(c.teams, teamSlug)
	return nil
}

func (c *fakeTeamClient) EditTeam(org string, team github.Team) (*github.Team, error) {
	slug := team.Slug
	t, ok := c.teams[slug]
	if !ok {
		return nil, fmt.Errorf("team %s does not exist", slug)
	}
	switch {
	case team.Description == "fail":
		return nil, errors.New("injected description failure")
	case team.Name == "fail":
		return nil, errors.New("injected name failure")
	case team.Privacy == "fail":
		return nil, errors.New("injected privacy failure")
	}
	if team.Description != "" {
		t.Description = team.Description
	}
	if team.Name != "" {
		t.Name = team.Name
	}
	if team.Privacy != "" {
		t.Privacy = team.Privacy
	}
	if team.ParentTeamID != nil {
		t.Parent = &github.Team{
			ID: *team.ParentTeamID,
		}
	} else {
		t.Parent = nil
	}
	c.teams[slug] = t
	return &t, nil
}

func TestFindTeam(t *testing.T) {
	cases := []struct {
		name     string
		teams    map[string]github.Team
		current  string
		previous []string
		expected int
	}{
		{
			name: "will find current team",
			teams: map[string]github.Team{
				"hello": {ID: 17},
			},
			current:  "hello",
			expected: 17,
		},
		{
			name: "team does not exist returns nil",
			teams: map[string]github.Team{
				"unrelated": {ID: 5},
			},
			current: "hypothetical",
		},
		{
			name: "will find previous name",
			teams: map[string]github.Team{
				"deprecated name": {ID: 1},
			},
			current:  "current name",
			previous: []string{"archaic name", "deprecated name"},
			expected: 1,
		},
		{
			name: "prioritize current when previous also exists",
			teams: map[string]github.Team{
				"deprecated": {ID: 1},
				"current":    {ID: 2},
			},
			current:  "current",
			previous: []string{"deprecated"},
			expected: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := findTeam(tc.teams, tc.current, tc.previous...)
			switch {
			case actual == nil:
				if tc.expected != 0 {
					t.Errorf("failed to find team %d", tc.expected)
				}
			case tc.expected == 0:
				t.Errorf("unexpected team returned: %v", *actual)
			case actual.ID != tc.expected:
				t.Errorf("team %v != expected ID %d", actual, tc.expected)
			}
		})
	}
}

func TestConfigureTeams(t *testing.T) {
	desc := "so interesting"
	priv := org.Secret
	cases := []struct {
		name              string
		err               bool
		orgNameOverride   string
		ignoreSecretTeams bool
		config            org.Config
		teams             []github.Team
		expected          map[string]github.Team
		deleted           []string
		delta             float64
	}{
		{
			name: "do nothing without error",
		},
		{
			name: "reject duplicated team names (different teams)",
			err:  true,
			config: org.Config{
				Teams: map[string]org.Team{
					"hello": {},
					"there": {Previously: []string{"hello"}},
				},
			},
		},
		{
			name: "reject duplicated team names (single team)",
			err:  true,
			config: org.Config{
				Teams: map[string]org.Team{
					"hello": {Previously: []string{"hello"}},
				},
			},
		},
		{
			name:            "fail to list teams",
			orgNameOverride: "fail",
			err:             true,
		},
		{
			name: "fail to create team",
			config: org.Config{
				Teams: map[string]org.Team{
					"fail": {},
				},
			},
			err: true,
		},
		{
			name: "fail to delete team",
			teams: []github.Team{
				{Name: "fail", ID: -55},
			},
			err: true,
		},
		{
			name: "create missing team",
			teams: []github.Team{
				{Name: "old", ID: 1},
			},
			config: org.Config{
				Teams: map[string]org.Team{
					"new": {},
					"old": {},
				},
			},
			expected: map[string]github.Team{
				"old": {Name: "old", ID: 1},
				"new": {Name: "new", ID: 3},
			},
		},
		{
			name: "reuse existing teams",
			teams: []github.Team{
				{Name: "current", Slug: "current", ID: 1},
				{Name: "deprecated", Slug: "deprecated", ID: 5},
			},
			config: org.Config{
				Teams: map[string]org.Team{
					"current": {},
					"updated": {Previously: []string{"deprecated"}},
				},
			},
			expected: map[string]github.Team{
				"current": {Name: "current", Slug: "current", ID: 1},
				"updated": {Name: "deprecated", Slug: "deprecated", ID: 5},
			},
		},
		{
			name: "delete unused teams",
			teams: []github.Team{
				{
					Name: "unused",
					Slug: "unused",
					ID:   1,
				},
				{
					Name: "used",
					Slug: "used",
					ID:   2,
				},
			},
			config: org.Config{
				Teams: map[string]org.Team{
					"used": {},
				},
			},
			expected: map[string]github.Team{
				"used": {ID: 2, Name: "used", Slug: "used"},
			},
			deleted: []string{"unused"},
		},
		{
			name: "create team with metadata",
			config: org.Config{
				Teams: map[string]org.Team{
					"new": {
						TeamMetadata: org.TeamMetadata{
							Description: &desc,
							Privacy:     &priv,
						},
					},
				},
			},
			expected: map[string]github.Team{
				"new": {ID: 1, Name: "new", Description: desc, Privacy: string(priv)},
			},
		},
		{
			name: "allow deleting many teams",
			teams: []github.Team{
				{
					Name: "unused",
					Slug: "unused",
					ID:   1,
				},
				{
					Name: "used",
					Slug: "used",
					ID:   2,
				},
			},
			config: org.Config{
				Teams: map[string]org.Team{
					"used": {},
				},
			},
			expected: map[string]github.Team{
				"used": {ID: 2, Name: "used", Slug: "used"},
			},
			deleted: []string{"unused"},
			delta:   0.6,
		},
		{
			name: "refuse to delete too many teams",
			teams: []github.Team{
				{
					Name: "unused",
					Slug: "unused",
					ID:   1,
				},
				{
					Name: "used",
					Slug: "used",
					ID:   2,
				},
			},
			config: org.Config{
				Teams: map[string]org.Team{
					"used": {},
				},
			},
			err:   true,
			delta: 0.1,
		},
		{
			name:              "refuse to delete private teams if ignoring them",
			ignoreSecretTeams: true,
			teams: []github.Team{
				{
					Name:    "secret",
					Slug:    "secret",
					ID:      1,
					Privacy: string(org.Secret),
				},
				{
					Name:    "closed",
					Slug:    "closed",
					ID:      2,
					Privacy: string(org.Closed),
				},
			},
			config:   org.Config{Teams: map[string]org.Team{}},
			err:      false,
			expected: map[string]github.Team{},
			deleted:  []string{"closed"},
			delta:    1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fc := makeFakeTeamClient(tc.teams...)
			orgName := tc.orgNameOverride
			if orgName == "" {
				orgName = fakeOrg
			}
			if tc.expected == nil {
				tc.expected = map[string]github.Team{}
			}
			if tc.delta == 0 {
				tc.delta = 1
			}
			actual, err := configureTeams(fc, orgName, tc.config, tc.delta, tc.ignoreSecretTeams)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Errorf("failed to receive error")
			case !reflect.DeepEqual(actual, tc.expected):
				t.Errorf("%#v != actual %#v", tc.expected, actual)
			}
			for _, slug := range tc.deleted {
				if team, ok := fc.teams[slug]; ok {
					t.Errorf("%s still present: %#v", slug, team)
				}
			}
			original, current, deleted := sets.New[string](), sets.New[string](), sets.New[string](tc.deleted...)
			for _, team := range tc.teams {
				original.Insert(team.Slug)
			}
			for slug := range fc.teams {
				current.Insert(slug)
			}
			if unexpected := original.Difference(current).Difference(deleted); unexpected.Len() > 0 {
				t.Errorf("the following teams were unexpectedly deleted: %v", sets.List(unexpected))
			}
		})
	}
}

func TestConfigureTeam(t *testing.T) {
	old := "old value"
	cur := "current value"
	fail := "fail"
	pfail := org.Privacy(fail)
	whatev := "whatever"
	secret := org.Secret
	parent := 2
	cases := []struct {
		name     string
		err      bool
		teamName string
		parent   *int
		config   org.Team
		github   github.Team
		expected github.Team
	}{
		{
			name:     "patch team when name changes",
			teamName: cur,
			config: org.Team{
				Previously: []string{old},
			},
			github: github.Team{
				ID:   1,
				Name: old,
			},
			expected: github.Team{
				ID:   1,
				Name: cur,
			},
		},
		{
			name:     "patch team when description changes",
			teamName: whatev,
			parent:   nil,
			config: org.Team{
				TeamMetadata: org.TeamMetadata{
					Description: &cur,
				},
			},
			github: github.Team{
				ID:          2,
				Name:        whatev,
				Description: old,
			},
			expected: github.Team{
				ID:          2,
				Name:        whatev,
				Description: cur,
			},
		},
		{
			name:     "patch team when privacy changes",
			teamName: whatev,
			parent:   nil,
			config: org.Team{
				TeamMetadata: org.TeamMetadata{
					Privacy: &secret,
				},
			},
			github: github.Team{
				ID:      3,
				Name:    whatev,
				Privacy: string(org.Closed),
			},
			expected: github.Team{
				ID:      3,
				Name:    whatev,
				Privacy: string(secret),
			},
		},
		{
			name:     "patch team when parent changes",
			teamName: whatev,
			parent:   &parent,
			config:   org.Team{},
			github: github.Team{
				ID:   3,
				Name: whatev,
				Parent: &github.Team{
					ID: 4,
				},
			},
			expected: github.Team{
				ID:   3,
				Name: whatev,
				Parent: &github.Team{
					ID: 2,
				},
				Privacy: string(org.Closed),
			},
		},
		{
			name:     "patch team when parent removed",
			teamName: whatev,
			parent:   nil,
			config:   org.Team{},
			github: github.Team{
				ID:   3,
				Name: whatev,
				Parent: &github.Team{
					ID: 2,
				},
			},
			expected: github.Team{
				ID:     3,
				Name:   whatev,
				Parent: nil,
			},
		},
		{
			name:     "do not patch team when values are the same",
			teamName: fail,
			parent:   &parent,
			config: org.Team{
				TeamMetadata: org.TeamMetadata{
					Description: &fail,
					Privacy:     &pfail,
				},
			},
			github: github.Team{
				ID:          4,
				Name:        fail,
				Description: fail,
				Privacy:     fail,
				Parent: &github.Team{
					ID: 2,
				},
			},
			expected: github.Team{
				ID:          4,
				Name:        fail,
				Description: fail,
				Privacy:     fail,
				Parent: &github.Team{
					ID: 2,
				},
			},
		},
		{
			name:     "fail to patch team",
			teamName: "team",
			parent:   nil,
			config: org.Team{
				TeamMetadata: org.TeamMetadata{
					Description: &fail,
				},
			},
			github: github.Team{
				ID:          1,
				Name:        "team",
				Description: whatev,
			},
			err: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fc := makeFakeTeamClient(tc.github)
			err := configureTeam(fc, fakeOrg, tc.teamName, tc.config, tc.github, tc.parent)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Errorf("failed to receive expected error")
			case !reflect.DeepEqual(fc.teams[tc.expected.Slug], tc.expected):
				t.Errorf("actual %+v != expected %+v", fc.teams[tc.expected.Slug], tc.expected)
			}
		})
	}
}

func TestConfigureTeamMembers(t *testing.T) {
	cases := []struct {
		name           string
		err            bool
		members        sets.Set[string]
		maintainers    sets.Set[string]
		remove         sets.Set[string]
		addMembers     sets.Set[string]
		addMaintainers sets.Set[string]
		ignoreInvitees bool
		invitees       sets.Set[string]
		team           org.Team
		slug           string
	}{
		{
			name: "fail when listing fails",
			slug: "some-slug",
			err:  true,
		},
		{
			name:    "fail when removal fails",
			members: sets.New[string]("fail"),
			err:     true,
		},
		{
			name: "fail when add fails",
			team: org.Team{
				Maintainers: []string{"fail"},
			},
			err: true,
		},
		{
			name: "some of everything",
			team: org.Team{
				Maintainers: []string{"keep-maintainer", "new-maintainer"},
				Members:     []string{"keep-member", "new-member"},
			},
			maintainers:    sets.New[string]("keep-maintainer", "drop-maintainer"),
			members:        sets.New[string]("keep-member", "drop-member"),
			remove:         sets.New[string]("drop-maintainer", "drop-member"),
			addMembers:     sets.New[string]("new-member"),
			addMaintainers: sets.New[string]("new-maintainer"),
		},
		{
			name: "do not reinvitee invitees",
			team: org.Team{
				Maintainers: []string{"invited-maintainer", "newbie"},
				Members:     []string{"invited-member"},
			},
			invitees:       sets.New[string]("invited-maintainer", "invited-member"),
			addMaintainers: sets.New[string]("newbie"),
		},
		{
			name: "do not remove pending invitees",
			team: org.Team{
				Maintainers: []string{"keep-maintainer"},
				Members:     []string{"invited-member"},
			},
			maintainers: sets.New[string]("keep-maintainer"),
			invitees:    sets.New[string]("invited-member"),
			remove:      sets.Set[string]{},
		},
		{
			name: "ignore invitees",
			team: org.Team{
				Maintainers: []string{"keep-maintainer"},
				Members:     []string{"keep-member", "new-member"},
			},
			maintainers:    sets.New[string]("keep-maintainer"),
			members:        sets.New[string]("keep-member"),
			invitees:       sets.Set[string]{},
			remove:         sets.Set[string]{},
			addMembers:     sets.New[string]("new-member"),
			ignoreInvitees: true,
		},
	}

	for _, tc := range cases {
		gt := github.Team{
			Slug: configuredTeamSlug,
			Name: "whatev",
		}
		if tc.slug != "" {
			gt.Slug = tc.slug
		}
		t.Run(tc.name, func(t *testing.T) {
			fc := &fakeClient{
				admins:     sets.KeySet[string](tc.maintainers),
				members:    sets.KeySet[string](tc.members),
				invitees:   sets.KeySet[string](tc.invitees),
				removed:    sets.Set[string]{},
				newAdmins:  sets.Set[string]{},
				newMembers: sets.Set[string]{},
			}
			err := configureTeamMembers(fc, "", gt, tc.team, tc.ignoreInvitees)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("Unexpected error: %v", err)
				}
			case tc.err:
				t.Errorf("Failed to receive error")
			default:
				if err := cmpLists(sets.List(tc.remove), sets.List(fc.removed)); err != nil {
					t.Errorf("Wrong users removed: %v", err)
				} else if err := cmpLists(sets.List(tc.addMembers), sets.List(fc.newMembers)); err != nil {
					t.Errorf("Wrong members added: %v", err)
				} else if err := cmpLists(sets.List(tc.addMaintainers), sets.List(fc.newAdmins)); err != nil {
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

type fakeOrgClient struct {
	current github.Organization
	changed bool
}

func (o *fakeOrgClient) GetOrg(name string) (*github.Organization, error) {
	if name == "fail" {
		return nil, errors.New("injected GetOrg error")
	}
	return &o.current, nil
}

func (o *fakeOrgClient) EditOrg(name string, org github.Organization) (*github.Organization, error) {
	if org.Description == "fail" {
		return nil, errors.New("injected EditOrg error")
	}
	o.current = org
	o.changed = true
	return &o.current, nil
}

func TestUpdateBool(t *testing.T) {
	yes := true
	no := false
	cases := []struct {
		name string
		have *bool
		want *bool
		end  bool
		ret  *bool
	}{
		{
			name: "panic on nil have",
			want: &no,
		},
		{
			name: "never change on nil want",
			want: nil,
			have: &yes,
			end:  yes,
			ret:  &no,
		},
		{
			name: "do not change if same",
			want: &yes,
			have: &yes,
			end:  yes,
			ret:  &no,
		},
		{
			name: "change if different",
			want: &no,
			have: &yes,
			end:  no,
			ret:  &yes,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				wantPanic := tc.ret == nil
				r := recover()
				gotPanic := r != nil
				switch {
				case gotPanic && !wantPanic:
					t.Errorf("unexpected panic: %v", r)
				case wantPanic && !gotPanic:
					t.Errorf("failed to receive panic")
				}
			}()
			if tc.have != nil { // prevent overwriting what tc.have points to for next test case
				have := *tc.have
				tc.have = &have
			}
			ret := updateBool(tc.have, tc.want)
			switch {
			case ret != *tc.ret:
				t.Errorf("return value %t != expected %t", ret, *tc.ret)
			case *tc.have != tc.end:
				t.Errorf("end value %t != expected %t", *tc.have, tc.end)
			}
		})
	}
}

func TestUpdateString(t *testing.T) {
	no := false
	yes := true
	hello := "hello"
	world := "world"
	empty := ""
	cases := []struct {
		name     string
		have     *string
		want     *string
		expected string
		ret      *bool
	}{
		{
			name: "panic on nil have",
			want: &hello,
		},
		{
			name:     "never change on nil want",
			want:     nil,
			have:     &hello,
			expected: hello,
			ret:      &no,
		},
		{
			name:     "do not change if same",
			want:     &world,
			have:     &world,
			expected: world,
			ret:      &no,
		},
		{
			name:     "change if different",
			want:     &empty,
			have:     &hello,
			expected: empty,
			ret:      &yes,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				wantPanic := tc.ret == nil
				r := recover()
				gotPanic := r != nil
				switch {
				case gotPanic && !wantPanic:
					t.Errorf("unexpected panic: %v", r)
				case wantPanic && !gotPanic:
					t.Errorf("failed to receive panic")
				}
			}()
			if tc.have != nil { // prevent overwriting what tc.have points to for next test case
				have := *tc.have
				tc.have = &have
			}
			ret := updateString(tc.have, tc.want)
			switch {
			case ret != *tc.ret:
				t.Errorf("return value %t != expected %t", ret, *tc.ret)
			case *tc.have != tc.expected:
				t.Errorf("end value %s != expected %s", *tc.have, tc.expected)
			}
		})
	}
}

func TestConfigureOrgMeta(t *testing.T) {
	filled := github.Organization{
		BillingEmail:                 "be",
		Company:                      "co",
		Email:                        "em",
		Location:                     "lo",
		Name:                         "na",
		Description:                  "de",
		HasOrganizationProjects:      true,
		HasRepositoryProjects:        true,
		DefaultRepositoryPermission:  "not-a-real-value",
		MembersCanCreateRepositories: true,
	}
	yes := true
	no := false
	str := "random-letters"
	fail := "fail"
	read := github.Read

	cases := []struct {
		name     string
		orgName  string
		want     org.Metadata
		have     github.Organization
		expected github.Organization
		err      bool
		change   bool
	}{
		{
			name:     "no want means no change",
			have:     filled,
			expected: filled,
			change:   false,
		},
		{
			name:    "fail if GetOrg fails",
			orgName: fail,
			err:     true,
		},
		{
			name: "fail if EditOrg fails",
			want: org.Metadata{Description: &fail},
			err:  true,
		},
		{
			name: "billing diff causes change",
			want: org.Metadata{BillingEmail: &str},
			expected: github.Organization{
				BillingEmail: str,
			},
			change: true,
		},
		{
			name: "company diff causes change",
			want: org.Metadata{Company: &str},
			expected: github.Organization{
				Company: str,
			},
			change: true,
		},
		{
			name: "email diff causes change",
			want: org.Metadata{Email: &str},
			expected: github.Organization{
				Email: str,
			},
			change: true,
		},
		{
			name: "location diff causes change",
			want: org.Metadata{Location: &str},
			expected: github.Organization{
				Location: str,
			},
			change: true,
		},
		{
			name: "name diff causes change",
			want: org.Metadata{Name: &str},
			expected: github.Organization{
				Name: str,
			},
			change: true,
		},
		{
			name: "org projects diff causes change",
			want: org.Metadata{HasOrganizationProjects: &yes},
			expected: github.Organization{
				HasOrganizationProjects: yes,
			},
			change: true,
		},
		{
			name: "repo projects diff causes change",
			want: org.Metadata{HasRepositoryProjects: &yes},
			expected: github.Organization{
				HasRepositoryProjects: yes,
			},
			change: true,
		},
		{
			name: "default permission diff causes change",
			want: org.Metadata{DefaultRepositoryPermission: &read},
			expected: github.Organization{
				DefaultRepositoryPermission: string(read),
			},
			change: true,
		},
		{
			name: "members can create diff causes change",
			want: org.Metadata{MembersCanCreateRepositories: &yes},
			expected: github.Organization{
				MembersCanCreateRepositories: yes,
			},
			change: true,
		},
		{
			name: "change all values at once",
			have: filled,
			want: org.Metadata{
				BillingEmail:                 &str,
				Company:                      &str,
				Email:                        &str,
				Location:                     &str,
				Name:                         &str,
				Description:                  &str,
				HasOrganizationProjects:      &no,
				HasRepositoryProjects:        &no,
				MembersCanCreateRepositories: &no,
				DefaultRepositoryPermission:  &read,
			},
			expected: github.Organization{
				BillingEmail:                 str,
				Company:                      str,
				Email:                        str,
				Location:                     str,
				Name:                         str,
				Description:                  str,
				HasOrganizationProjects:      no,
				HasRepositoryProjects:        no,
				MembersCanCreateRepositories: no,
				DefaultRepositoryPermission:  string(read),
			},
			change: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.orgName == "" {
				tc.orgName = "whatever"
			}
			fc := fakeOrgClient{
				current: tc.have,
			}
			err := configureOrgMeta(&fc, tc.orgName, tc.want)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Errorf("failed to receive error")
			case tc.change != fc.changed:
				t.Errorf("changed %t != expected %t", fc.changed, tc.change)
			case !reflect.DeepEqual(fc.current, tc.expected):
				t.Errorf("current %#v != expected %#v", fc.current, tc.expected)
			}
		})
	}
}

func TestDumpOrgConfig(t *testing.T) {
	empty := ""
	hello := "Hello"
	details := "wise and brilliant exemplary human specimens"
	yes := true
	no := false
	perm := github.Write
	pub := org.Privacy("")
	secret := org.Secret
	closed := org.Closed
	repoName := "project"
	repoDescription := "awesome testing project"
	repoHomepage := "https://www.somewhe.re/something/"
	master := "master-branch"
	cases := []struct {
		name              string
		orgOverride       string
		ignoreSecretTeams bool
		meta              github.Organization
		members           []string
		admins            []string
		teams             []github.Team
		teamMembers       map[string][]string
		maintainers       map[string][]string
		repoPermissions   map[string][]github.Repo
		repos             []github.FullRepo
		expected          org.Config
		err               bool
	}{
		{
			name:        "fails if GetOrg fails",
			orgOverride: "fail",
			err:         true,
		},
		{
			name:    "fails if ListOrgMembers fails",
			err:     true,
			members: []string{"hello", "fail"},
		},
		{
			name: "fails if ListTeams fails",
			err:  true,
			teams: []github.Team{
				{
					Name: "fail",
					ID:   3,
				},
			},
		},
		{
			name: "fails if ListTeamMembersFails",
			err:  true,
			teams: []github.Team{
				{
					Name: "fred",
					ID:   -1,
				},
			},
		},
		{
			name: "fails if GetTeams fails",
			err:  true,
			repos: []github.FullRepo{
				{
					Repo: github.Repo{
						Name: "fail",
					},
				},
			},
		},
		{
			name:   "fails if not as an admin of the org",
			err:    true,
			admins: []string{"not-admin"},
		},
		{
			name: "basically works",
			meta: github.Organization{
				Name:                         hello,
				MembersCanCreateRepositories: yes,
				DefaultRepositoryPermission:  string(perm),
			},
			members: []string{"george", "jungle", "banana"},
			admins:  []string{"admin", "james", "giant", "peach"},
			teams: []github.Team{
				{
					ID:          5,
					Slug:        "team-5",
					Name:        "friends",
					Description: details,
				},
				{
					ID:   6,
					Slug: "team-6",
					Name: "enemies",
				},
				{
					ID:   7,
					Slug: "team-7",
					Name: "archenemies",
					Parent: &github.Team{
						ID:   6,
						Slug: "team-6",
						Name: "enemies",
					},
					Privacy: string(org.Secret),
				},
			},
			teamMembers: map[string][]string{
				"team-5": {"george", "james"},
				"team-6": {"george"},
				"team-7": {},
			},
			maintainers: map[string][]string{
				"team-5": {},
				"team-6": {"giant", "jungle"},
				"team-7": {"banana"},
			},
			repoPermissions: map[string][]github.Repo{
				"team-5": {},
				"team-6": {{Name: "pull-repo", Permissions: github.RepoPermissions{Pull: true}}},
				"team-7": {{Name: "pull-repo", Permissions: github.RepoPermissions{Pull: true}}, {Name: "admin-repo", Permissions: github.RepoPermissions{Admin: true}}},
			},
			repos: []github.FullRepo{
				{
					Repo: github.Repo{
						Name:          repoName,
						Description:   repoDescription,
						Homepage:      repoHomepage,
						Private:       false,
						HasIssues:     true,
						HasProjects:   true,
						HasWiki:       true,
						Archived:      true,
						DefaultBranch: master,
					},
				},
			},
			expected: org.Config{
				Metadata: org.Metadata{
					Name:                         &hello,
					BillingEmail:                 &empty,
					Company:                      &empty,
					Email:                        &empty,
					Description:                  &empty,
					Location:                     &empty,
					HasOrganizationProjects:      &no,
					HasRepositoryProjects:        &no,
					DefaultRepositoryPermission:  &perm,
					MembersCanCreateRepositories: &yes,
				},
				Teams: map[string]org.Team{
					"friends": {
						TeamMetadata: org.TeamMetadata{
							Description: &details,
							Privacy:     &pub,
						},
						Members:     []string{"george", "james"},
						Maintainers: []string{},
						Children:    map[string]org.Team{},
						Repos:       map[string]github.RepoPermissionLevel{},
					},
					"enemies": {
						TeamMetadata: org.TeamMetadata{
							Description: &empty,
							Privacy:     &pub,
						},
						Members:     []string{"george"},
						Maintainers: []string{"giant", "jungle"},
						Repos: map[string]github.RepoPermissionLevel{
							"pull-repo": github.Read,
						},
						Children: map[string]org.Team{
							"archenemies": {
								TeamMetadata: org.TeamMetadata{
									Description: &empty,
									Privacy:     &secret,
								},
								Members:     []string{},
								Maintainers: []string{"banana"},
								Repos: map[string]github.RepoPermissionLevel{
									"pull-repo":  github.Read,
									"admin-repo": github.Admin,
								},
								Children: map[string]org.Team{},
							},
						},
					},
				},
				Members: []string{"george", "jungle", "banana"},
				Admins:  []string{"admin", "james", "giant", "peach"},
				Repos: map[string]org.Repo{
					"project": {
						Description:      &repoDescription,
						HomePage:         &repoHomepage,
						HasProjects:      &yes,
						AllowMergeCommit: &no,
						AllowRebaseMerge: &no,
						AllowSquashMerge: &no,
						Archived:         &yes,
						DefaultBranch:    &master,
					},
				},
			},
		},
		{
			name:              "ignores private teams when expected to",
			ignoreSecretTeams: true,
			meta: github.Organization{
				Name:                         hello,
				MembersCanCreateRepositories: yes,
				DefaultRepositoryPermission:  string(perm),
			},
			members: []string{"george", "jungle", "banana"},
			admins:  []string{"admin", "james", "giant", "peach"},
			teams: []github.Team{
				{
					ID:          5,
					Slug:        "team-5",
					Name:        "friends",
					Description: details,
				},
				{
					ID:   6,
					Slug: "team-6",
					Name: "enemies",
				},
				{
					ID:   7,
					Slug: "team-7",
					Name: "archenemies",
					Parent: &github.Team{
						Slug: "team-6",
						Name: "enemies",
					},
					Privacy: string(org.Secret),
				},
				{
					ID:   8,
					Slug: "team-8",
					Name: "frenemies",
					Parent: &github.Team{
						ID:   6,
						Slug: "team-6",
						Name: "enemies",
					},
					Privacy: string(org.Closed),
				},
			},
			teamMembers: map[string][]string{
				"team-5": {"george", "james"},
				"team-6": {"george"},
				"team-7": {},
				"team-8": {"patrick"},
			},
			maintainers: map[string][]string{
				"team-5": {},
				"team-6": {"giant", "jungle"},
				"team-7": {"banana"},
				"team-8": {"starfish"},
			},
			expected: org.Config{
				Metadata: org.Metadata{
					Name:                         &hello,
					BillingEmail:                 &empty,
					Company:                      &empty,
					Email:                        &empty,
					Description:                  &empty,
					Location:                     &empty,
					HasOrganizationProjects:      &no,
					HasRepositoryProjects:        &no,
					DefaultRepositoryPermission:  &perm,
					MembersCanCreateRepositories: &yes,
				},
				Teams: map[string]org.Team{
					"friends": {
						TeamMetadata: org.TeamMetadata{
							Description: &details,
							Privacy:     &pub,
						},
						Members:     []string{"george", "james"},
						Maintainers: []string{},
						Children:    map[string]org.Team{},
						Repos:       map[string]github.RepoPermissionLevel{},
					},
					"enemies": {
						TeamMetadata: org.TeamMetadata{
							Description: &empty,
							Privacy:     &pub,
						},
						Members:     []string{"george"},
						Maintainers: []string{"giant", "jungle"},
						Children: map[string]org.Team{
							"frenemies": {
								TeamMetadata: org.TeamMetadata{
									Description: &empty,
									Privacy:     &closed,
								},
								Members:     []string{"patrick"},
								Maintainers: []string{"starfish"},
								Children:    map[string]org.Team{},
								Repos:       map[string]github.RepoPermissionLevel{},
							},
						},
						Repos: map[string]github.RepoPermissionLevel{},
					},
				},
				Members: []string{"george", "jungle", "banana"},
				Admins:  []string{"admin", "james", "giant", "peach"},
				Repos:   map[string]org.Repo{},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			orgName := "random-org"
			if tc.orgOverride != "" {
				orgName = tc.orgOverride
			}
			fc := fakeDumpClient{
				name:            orgName,
				members:         tc.members,
				admins:          tc.admins,
				meta:            tc.meta,
				teams:           tc.teams,
				teamMembers:     tc.teamMembers,
				maintainers:     tc.maintainers,
				repoPermissions: tc.repoPermissions,
				repos:           tc.repos,
			}
			actual, err := dumpOrgConfig(fc, orgName, tc.ignoreSecretTeams, "")
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Errorf("failed to receive error")
			default:
				fixup(actual)
				fixup(&tc.expected)
				if diff := cmp.Diff(actual, &tc.expected); diff != "" {
					t.Errorf("did not get correct config, diff: %s", diff)
				}

			}
		})
	}
}

type fakeDumpClient struct {
	name            string
	members         []string
	admins          []string
	meta            github.Organization
	teams           []github.Team
	teamMembers     map[string][]string
	maintainers     map[string][]string
	repoPermissions map[string][]github.Repo
	repos           []github.FullRepo
}

func (c fakeDumpClient) GetOrg(name string) (*github.Organization, error) {
	if name != c.name {
		return nil, errors.New("bad name")
	}
	if name == "fail" {
		return nil, errors.New("injected GetOrg error")
	}
	return &c.meta, nil
}

func (c fakeDumpClient) makeMembers(people []string) ([]github.TeamMember, error) {
	var ret []github.TeamMember
	for _, p := range people {
		if p == "fail" {
			return nil, errors.New("injected makeMembers error")
		}
		ret = append(ret, github.TeamMember{Login: p})
	}
	return ret, nil
}

func (c fakeDumpClient) ListOrgMembers(name, role string) ([]github.TeamMember, error) {
	switch {
	case name != c.name:
		return nil, fmt.Errorf("bad org: %s", name)
	case role == github.RoleAdmin:
		return c.makeMembers(c.admins)
	case role == github.RoleMember:
		return c.makeMembers(c.members)
	}
	return nil, fmt.Errorf("bad role: %s", role)
}

func (c fakeDumpClient) ListTeams(name string) ([]github.Team, error) {
	if name != c.name {
		return nil, fmt.Errorf("bad org: %s", name)
	}

	for _, t := range c.teams {
		if t.Name == "fail" {
			return nil, errors.New("injected ListTeams error")
		}
	}
	return c.teams, nil
}

func (c fakeDumpClient) ListTeamMembersBySlug(org, teamSlug, role string) ([]github.TeamMember, error) {
	var mapping map[string][]string
	switch {
	case teamSlug == "":
		return nil, errors.New("injected ListTeamMembers error")
	case role == github.RoleMaintainer:
		mapping = c.maintainers
	case role == github.RoleMember:
		mapping = c.teamMembers
	default:
		return nil, fmt.Errorf("bad role: %s", role)
	}
	people, ok := mapping[teamSlug]
	if !ok {
		return nil, fmt.Errorf("team does not exist: %s", teamSlug)
	}
	return c.makeMembers(people)
}

func (c fakeDumpClient) ListTeamReposBySlug(org, teamSlug string) ([]github.Repo, error) {
	if teamSlug == "" {
		return nil, errors.New("injected ListTeamRepos error")
	}

	return c.repoPermissions[teamSlug], nil
}

func (c fakeDumpClient) GetRepos(org string, isUser bool) ([]github.Repo, error) {
	var repos []github.Repo
	for _, repo := range c.repos {
		if repo.Name == "fail" {
			return nil, fmt.Errorf("injected GetRepos error")
		}
		repos = append(repos, repo.Repo)
	}

	return repos, nil
}

func (c fakeDumpClient) GetRepo(owner, repo string) (github.FullRepo, error) {
	for _, r := range c.repos {
		switch {
		case r.Name == "fail":
			return r, fmt.Errorf("injected GetRepo error")
		case r.Name == repo:
			return r, nil
		}
	}

	return github.FullRepo{}, fmt.Errorf("not found")
}

func (c fakeDumpClient) BotUser() (*github.UserData, error) {
	return &github.UserData{Login: "admin"}, nil
}

func fixup(ret *org.Config) {
	if ret == nil {
		return
	}
	sort.Strings(ret.Members)
	sort.Strings(ret.Admins)
	for name, team := range ret.Teams {
		sort.Strings(team.Members)
		sort.Strings(team.Maintainers)
		sort.Strings(team.Previously)
		ret.Teams[name] = team
	}
}

func TestOrgInvitations(t *testing.T) {
	cases := []struct {
		name     string
		opt      options
		invitees sets.Set[string] // overrides
		expected sets.Set[string]
		err      bool
	}{
		{
			name:     "do not call on empty options",
			invitees: sets.New[string]("him", "her", "them"),
			expected: sets.Set[string]{},
		},
		{
			name: "call if fixOrgMembers",
			opt: options{
				fixOrgMembers: true,
			},
			invitees: sets.New[string]("him", "her", "them"),
			expected: sets.New[string]("him", "her", "them"),
		},
		{
			name: "call if fixTeamMembers",
			opt: options{
				fixTeamMembers: true,
			},
			invitees: sets.New[string]("him", "her", "them"),
			expected: sets.New[string]("him", "her", "them"),
		},
		{
			name: "ensure case normalization",
			opt: options{
				fixOrgMembers:  true,
				fixTeamMembers: true,
			},
			invitees: sets.New[string]("MiXeD", "lower", "UPPER"),
			expected: sets.New[string]("mixed", "lower", "upper"),
		},
		{
			name: "error if list fails",
			opt: options{
				fixTeamMembers: true,
				fixOrgMembers:  true,
			},
			invitees: sets.New[string]("erick", "fail"),
			err:      true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fc := &fakeClient{
				invitees: tc.invitees,
			}
			actual, err := orgInvitations(tc.opt, fc, "random-org")
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Errorf("failed to receive an error")
			case !reflect.DeepEqual(actual, tc.expected):
				t.Errorf("%#v != expected %#v", actual, tc.expected)
			}
		})
	}
}

type fakeTeamRepoClient struct {
	repos                            map[string][]github.Repo
	failList, failUpdate, failRemove bool
}

func (c *fakeTeamRepoClient) ListTeamReposBySlug(org, teamSlug string) ([]github.Repo, error) {
	if c.failList {
		return nil, errors.New("injected failure to ListTeamRepos")
	}
	return c.repos[teamSlug], nil
}

func (c *fakeTeamRepoClient) UpdateTeamRepoBySlug(org, teamSlug, repo string, permission github.TeamPermission) error {
	if c.failUpdate {
		return errors.New("injected failure to UpdateTeamRepos")
	}

	permissions := github.PermissionsFromTeamPermission(permission)
	updated := false
	for i, repository := range c.repos[teamSlug] {
		if repository.Name == repo {
			c.repos[teamSlug][i].Permissions = permissions
			updated = true
			break
		}
	}

	if !updated {
		c.repos[teamSlug] = append(c.repos[teamSlug], github.Repo{Name: repo, Permissions: permissions})
	}

	return nil
}

func (c *fakeTeamRepoClient) RemoveTeamRepoBySlug(org, teamSlug, repo string) error {
	if c.failRemove {
		return errors.New("injected failure to RemoveTeamRepos")
	}

	for i, repository := range c.repos[teamSlug] {
		if repository.Name == repo {
			c.repos[teamSlug] = append(c.repos[teamSlug][:i], c.repos[teamSlug][i+1:]...)
			break
		}
	}

	return nil
}

func TestConfigureTeamRepos(t *testing.T) {
	var testCases = []struct {
		name          string
		githubTeams   map[string]github.Team
		teamName      string
		team          org.Team
		existingRepos map[string][]github.Repo
		failList      bool
		failUpdate    bool
		failRemove    bool
		expected      map[string][]github.Repo
		expectedErr   bool
	}{
		{
			name:        "githubTeams cache not containing team errors",
			githubTeams: map[string]github.Team{},
			teamName:    "team",
			expectedErr: true,
		},
		{
			name:        "listing repos failing errors",
			githubTeams: map[string]github.Team{"team": {ID: 1, Slug: "team"}},
			teamName:    "team",
			failList:    true,
			expectedErr: true,
		},
		{
			name:        "nothing to do",
			githubTeams: map[string]github.Team{"team": {ID: 1, Slug: "team"}},
			teamName:    "team",
			team: org.Team{
				Repos: map[string]github.RepoPermissionLevel{
					"read":     github.Read,
					"triage":   github.Triage,
					"write":    github.Write,
					"maintain": github.Maintain,
					"admin":    github.Admin,
				},
			},
			existingRepos: map[string][]github.Repo{"team": {
				{Name: "read", Permissions: github.RepoPermissions{Pull: true}},
				{Name: "triage", Permissions: github.RepoPermissions{Pull: true, Triage: true}},
				{Name: "write", Permissions: github.RepoPermissions{Pull: true, Triage: true, Push: true}},
				{Name: "maintain", Permissions: github.RepoPermissions{Pull: true, Triage: true, Push: true, Maintain: true}},
				{Name: "admin", Permissions: github.RepoPermissions{Pull: true, Triage: true, Push: true, Maintain: true, Admin: true}},
			}},
			expected: map[string][]github.Repo{"team": {
				{Name: "read", Permissions: github.RepoPermissions{Pull: true}},
				{Name: "triage", Permissions: github.RepoPermissions{Pull: true, Triage: true}},
				{Name: "write", Permissions: github.RepoPermissions{Pull: true, Triage: true, Push: true}},
				{Name: "maintain", Permissions: github.RepoPermissions{Pull: true, Triage: true, Push: true, Maintain: true}},
				{Name: "admin", Permissions: github.RepoPermissions{Pull: true, Triage: true, Push: true, Maintain: true, Admin: true}},
			}},
		},
		{
			name:        "new requirement in org config gets added",
			githubTeams: map[string]github.Team{"team": {ID: 1, Slug: "team"}},
			teamName:    "team",
			team: org.Team{
				Repos: map[string]github.RepoPermissionLevel{
					"read":        github.Read,
					"write":       github.Write,
					"admin":       github.Admin,
					"other-admin": github.Admin,
				},
			},
			existingRepos: map[string][]github.Repo{"team": {
				{Name: "read", Permissions: github.RepoPermissions{Pull: true}},
				{Name: "write", Permissions: github.RepoPermissions{Pull: true, Triage: true, Push: true}},
				{Name: "admin", Permissions: github.RepoPermissions{Pull: true, Triage: true, Push: true, Maintain: true, Admin: true}},
			}},
			expected: map[string][]github.Repo{"team": {
				{Name: "read", Permissions: github.RepoPermissions{Pull: true}},
				{Name: "write", Permissions: github.RepoPermissions{Pull: true, Triage: true, Push: true}},
				{Name: "admin", Permissions: github.RepoPermissions{Pull: true, Triage: true, Push: true, Maintain: true, Admin: true}},
				{Name: "other-admin", Permissions: github.RepoPermissions{Pull: true, Triage: true, Push: true, Maintain: true, Admin: true}},
			}},
		},
		{
			name:        "change in permission on existing gets updated",
			githubTeams: map[string]github.Team{"team": {ID: 1, Slug: "team"}},
			teamName:    "team",
			team: org.Team{
				Repos: map[string]github.RepoPermissionLevel{
					"read":  github.Read,
					"write": github.Write,
					"admin": github.Read,
				},
			},
			existingRepos: map[string][]github.Repo{"team": {
				{Name: "read", Permissions: github.RepoPermissions{Pull: true}},
				{Name: "write", Permissions: github.RepoPermissions{Pull: true, Triage: true, Push: true}},
				{Name: "admin", Permissions: github.RepoPermissions{Pull: true, Triage: true, Push: true, Maintain: true, Admin: true}},
			}},
			expected: map[string][]github.Repo{"team": {
				{Name: "read", Permissions: github.RepoPermissions{Pull: true}},
				{Name: "write", Permissions: github.RepoPermissions{Pull: true, Triage: true, Push: true}},
				{Name: "admin", Permissions: github.RepoPermissions{Pull: true}},
			}},
		},
		{
			name:        "omitted requirement gets removed",
			githubTeams: map[string]github.Team{"team": {ID: 1, Slug: "team"}},
			teamName:    "team",
			team: org.Team{
				Repos: map[string]github.RepoPermissionLevel{
					"write": github.Write,
					"admin": github.Read,
				},
			},
			existingRepos: map[string][]github.Repo{"team": {
				{Name: "read", Permissions: github.RepoPermissions{Pull: true}},
				{Name: "write", Permissions: github.RepoPermissions{Pull: true, Triage: true, Push: true}},
				{Name: "admin", Permissions: github.RepoPermissions{Pull: true, Triage: true, Push: true, Maintain: true, Admin: true}},
			}},
			expected: map[string][]github.Repo{"team": {
				{Name: "write", Permissions: github.RepoPermissions{Pull: true, Triage: true, Push: true}},
				{Name: "admin", Permissions: github.RepoPermissions{Pull: true}},
			}},
		},
		{
			name:        "failed update errors",
			failUpdate:  true,
			githubTeams: map[string]github.Team{"team": {ID: 1}},
			teamName:    "team",
			team: org.Team{
				Repos: map[string]github.RepoPermissionLevel{
					"will-fail": github.Write,
				},
			},
			existingRepos: map[string][]github.Repo{"some-team": {}},
			expected:      map[string][]github.Repo{"some-team": {}},
			expectedErr:   true,
		},
		{
			name:        "failed delete errors",
			failRemove:  true,
			githubTeams: map[string]github.Team{"team": {ID: 1, Slug: "team"}},
			teamName:    "team",
			team: org.Team{
				Repos: map[string]github.RepoPermissionLevel{},
			},
			existingRepos: map[string][]github.Repo{"team": {
				{Name: "needs-deletion", Permissions: github.RepoPermissions{Pull: true}},
			}},
			expected: map[string][]github.Repo{"team": {
				{Name: "needs-deletion", Permissions: github.RepoPermissions{Pull: true}},
			}},
			expectedErr: true,
		},
		{
			name:        "new requirement in child team config gets added",
			githubTeams: map[string]github.Team{"team": {ID: 1, Slug: "team"}, "child": {ID: 2, Slug: "child"}},
			teamName:    "team",
			team: org.Team{
				Children: map[string]org.Team{
					"child": {
						Repos: map[string]github.RepoPermissionLevel{
							"read":        github.Read,
							"write":       github.Write,
							"admin":       github.Admin,
							"other-admin": github.Admin,
						},
					},
				},
			},
			existingRepos: map[string][]github.Repo{"child": {
				{Name: "read", Permissions: github.RepoPermissions{Pull: true}},
				{Name: "write", Permissions: github.RepoPermissions{Pull: true, Triage: true, Push: true}},
				{Name: "admin", Permissions: github.RepoPermissions{Pull: true, Triage: true, Push: true, Maintain: true, Admin: true}},
			}},
			expected: map[string][]github.Repo{"child": {
				{Name: "read", Permissions: github.RepoPermissions{Pull: true}},
				{Name: "write", Permissions: github.RepoPermissions{Pull: true, Triage: true, Push: true}},
				{Name: "admin", Permissions: github.RepoPermissions{Pull: true, Triage: true, Push: true, Maintain: true, Admin: true}},
				{Name: "other-admin", Permissions: github.RepoPermissions{Pull: true, Triage: true, Push: true, Maintain: true, Admin: true}},
			}},
		},
		{
			name:        "failure in a child errors",
			failRemove:  true,
			githubTeams: map[string]github.Team{"team": {ID: 1, Slug: "team"}, "child": {ID: 2, Slug: "child"}},
			teamName:    "team",
			team: org.Team{
				Repos: map[string]github.RepoPermissionLevel{},
				Children: map[string]org.Team{
					"child": {
						Repos: map[string]github.RepoPermissionLevel{},
					},
				},
			},
			existingRepos: map[string][]github.Repo{"child": {
				{Name: "needs-deletion", Permissions: github.RepoPermissions{Pull: true}},
			}},
			expected: map[string][]github.Repo{"child": {
				{Name: "needs-deletion", Permissions: github.RepoPermissions{Pull: true}},
			}},
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		client := fakeTeamRepoClient{
			repos:      testCase.existingRepos,
			failList:   testCase.failList,
			failUpdate: testCase.failUpdate,
			failRemove: testCase.failRemove,
		}
		err := configureTeamRepos(&client, testCase.githubTeams, testCase.teamName, "org", testCase.team)
		if err == nil && testCase.expectedErr {
			t.Errorf("%s: expected an error but got none", testCase.name)
		}
		if err != nil && !testCase.expectedErr {
			t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
		}
		if diff := cmp.Diff(client.repos, testCase.expected); diff != "" {
			t.Errorf("%s: got incorrect team repos: %s", testCase.name, diff)
		}
	}
}

type fakeRepoClient struct {
	t     *testing.T
	repos map[string]github.FullRepo
}

func (f fakeRepoClient) GetRepo(owner, name string) (github.FullRepo, error) {
	repo, ok := f.repos[name]
	if !ok {
		return repo, fmt.Errorf("repo not found")
	}
	return repo, nil
}

func (f fakeRepoClient) GetRepos(orgName string, isUser bool) ([]github.Repo, error) {
	if orgName == "fail" {
		return nil, fmt.Errorf("injected GetRepos failure")
	}

	repos := make([]github.Repo, 0, len(f.repos))
	for _, repo := range f.repos {
		repos = append(repos, repo.Repo)
	}

	// sort for deterministic output
	sort.Slice(repos, func(i, j int) bool {
		return repos[i].Name < repos[j].Name
	})

	return repos, nil
}

func (f fakeRepoClient) CreateRepo(owner string, isUser bool, repoReq github.RepoCreateRequest) (*github.FullRepo, error) {
	if *repoReq.Name == "fail" {
		return nil, fmt.Errorf("injected CreateRepo failure")
	}

	if _, hasRepo := f.repos[*repoReq.Name]; hasRepo {
		f.t.Errorf("CreateRepo() called on repo that already exists")
		return nil, fmt.Errorf("CreateRepo() called on repo that already exists")
	}

	repo := repoReq.ToRepo()
	f.repos[*repoReq.Name] = *repo

	return repo, nil
}

func (f fakeRepoClient) UpdateRepo(owner, name string, want github.RepoUpdateRequest) (*github.FullRepo, error) {
	if name == "fail" {
		return nil, fmt.Errorf("injected UpdateRepo failure")
	}
	if want.Archived != nil && !*want.Archived {
		f.t.Errorf("UpdateRepo() called to unarchive a repo (not supported by API)")
		return nil, fmt.Errorf("UpdateRepo() called to unarchive a repo (not supported by API)")
	}

	have, exists := f.repos[name]
	if !exists {
		f.t.Errorf("UpdateRepo() called on repo that does not exists")
		return nil, fmt.Errorf("UpdateRepo() called on repo that does not exist")
	}

	if have.Archived {
		return nil, fmt.Errorf("Repository was archived so is read-only.")
	}

	updateString := func(have, want *string) {
		if want != nil {
			*have = *want
		}
	}

	updateBool := func(have, want *bool) {
		if want != nil {
			*have = *want
		}
	}

	updateString(&have.Name, want.Name)
	updateString(&have.DefaultBranch, want.DefaultBranch)
	updateString(&have.Homepage, want.Homepage)
	updateString(&have.Description, want.Description)
	updateBool(&have.Archived, want.Archived)
	updateBool(&have.Private, want.Private)
	updateBool(&have.HasIssues, want.HasIssues)
	updateBool(&have.HasProjects, want.HasProjects)
	updateBool(&have.HasWiki, want.HasWiki)
	updateBool(&have.AllowSquashMerge, want.AllowSquashMerge)
	updateBool(&have.AllowMergeCommit, want.AllowMergeCommit)
	updateBool(&have.AllowRebaseMerge, want.AllowRebaseMerge)
	updateString(&have.SquashMergeCommitTitle, want.SquashMergeCommitTitle)
	updateString(&have.SquashMergeCommitMessage, want.SquashMergeCommitMessage)

	f.repos[name] = have
	return &have, nil
}

func makeFakeRepoClient(t *testing.T, repos ...github.FullRepo) fakeRepoClient {
	fc := fakeRepoClient{
		repos: make(map[string]github.FullRepo, len(repos)),
		t:     t,
	}
	for _, repo := range repos {
		fc.repos[repo.Name] = repo
	}

	return fc
}

func TestConfigureRepos(t *testing.T) {
	orgName := "test-org"
	isOrg := false
	no := false
	yes := true
	updated := "UPDATED"

	oldName := "old"
	oldRepo := github.Repo{
		Name:        oldName,
		FullName:    fmt.Sprintf("%s/%s", orgName, oldName),
		Description: "An old existing repository",
	}

	newName := "new"
	newDescription := "A new repository."
	newConfigRepo := org.Repo{
		Description: &newDescription,
	}
	newRepo := github.Repo{
		Name:        newName,
		Description: newDescription,
	}

	fail := "fail"
	failRepo := github.Repo{
		Name: fail,
	}

	testCases := []struct {
		description     string
		opts            options
		orgConfig       org.Config
		orgNameOverride string
		repos           []github.FullRepo

		expectError   bool
		expectedRepos []github.Repo
	}{
		{
			description:   "survives empty config",
			expectedRepos: []github.Repo{},
		},
		{
			description: "survives nil repos config",
			orgConfig: org.Config{
				Repos: nil,
			},
			expectedRepos: []github.Repo{},
		},
		{
			description: "survives empty repos config",
			orgConfig: org.Config{
				Repos: map[string]org.Repo{},
			},
			expectedRepos: []github.Repo{},
		},
		{
			description: "nonexistent repo is created",
			orgConfig: org.Config{
				Repos: map[string]org.Repo{
					newName: newConfigRepo,
				},
			},
			repos: []github.FullRepo{{Repo: oldRepo}},

			expectedRepos: []github.Repo{newRepo, oldRepo},
		},
		{
			description:     "GetRepos failure is propagated",
			orgNameOverride: "fail",
			orgConfig: org.Config{
				Repos: map[string]org.Repo{
					newName: newConfigRepo,
				},
			},
			repos: []github.FullRepo{{Repo: oldRepo}},

			expectError:   true,
			expectedRepos: []github.Repo{oldRepo},
		},
		{
			description: "CreateRepo failure is propagated",
			orgConfig: org.Config{
				Repos: map[string]org.Repo{
					fail: newConfigRepo,
				},
			},
			repos: []github.FullRepo{{Repo: oldRepo}},

			expectError:   true,
			expectedRepos: []github.Repo{oldRepo},
		},
		{
			description: "duplicate repo names different only by case are detected",
			orgConfig: org.Config{
				Repos: map[string]org.Repo{
					"repo": newConfigRepo,
					"REPO": newConfigRepo,
				},
			},
			repos: []github.FullRepo{{Repo: oldRepo}},

			expectError:   true,
			expectedRepos: []github.Repo{oldRepo},
		},
		{
			description: "existing repo is updated",
			orgConfig: org.Config{
				Repos: map[string]org.Repo{
					oldName: newConfigRepo,
				},
			},
			repos: []github.FullRepo{{Repo: oldRepo}},
			expectedRepos: []github.Repo{
				{
					Name:        oldName,
					Description: newDescription,
					FullName:    fmt.Sprintf("%s/%s", orgName, oldName),
				},
			},
		},
		{
			description: "UpdateRepo failure is propagated",
			orgConfig: org.Config{
				Repos: map[string]org.Repo{
					"fail": newConfigRepo,
				},
			},
			repos:         []github.FullRepo{{Repo: failRepo}},
			expectError:   true,
			expectedRepos: []github.Repo{failRepo},
		},
		{
			// https://developer.github.com/v3/repos/#edit
			// "Note: You cannot unarchive repositories through the API."
			// Archived repositories are read-only, and updates fail with 403:
			// "Repository was archived so is read-only."
			description: "request to unarchive a repo fails, repo is read-only",
			orgConfig: org.Config{
				Repos: map[string]org.Repo{
					oldName: {Archived: &no, Description: &updated},
				},
			},
			repos:         []github.FullRepo{{Repo: github.Repo{Name: oldName, Archived: true, Description: "OLD"}}},
			expectError:   true,
			expectedRepos: []github.Repo{{Name: oldName, Archived: true, Description: "OLD"}},
		},
		{
			// https://developer.github.com/v3/repos/#edit
			// "Note: You cannot unarchive repositories through the API."
			// Archived repositories are read-only, and updates fail with 403:
			// "Repository was archived so is read-only."
			description: "no field changes on archived repo",
			orgConfig: org.Config{
				Repos: map[string]org.Repo{
					oldName: {Archived: &yes, Description: &updated},
				},
			},
			repos:         []github.FullRepo{{Repo: github.Repo{Name: oldName, Archived: true, Description: "OLD"}}},
			expectError:   false,
			expectedRepos: []github.Repo{{Name: oldName, Archived: true, Description: "OLD"}},
		},
		{
			description: "request to archive repo fails when not allowed, but updates other fields",
			orgConfig: org.Config{
				Repos: map[string]org.Repo{
					oldName: {Archived: &yes, Description: &updated},
				},
			},
			repos:         []github.FullRepo{{Repo: github.Repo{Name: oldName, Archived: false, Description: "OLD"}}},
			expectError:   true,
			expectedRepos: []github.Repo{{Name: oldName, Archived: false, Description: updated}},
		},
		{
			description: "request to archive repo succeeds when allowed",
			opts: options{
				allowRepoArchival: true,
			},
			orgConfig: org.Config{
				Repos: map[string]org.Repo{
					oldName: {Archived: &yes},
				},
			},
			repos:         []github.FullRepo{{Repo: github.Repo{Name: oldName, Archived: false}}},
			expectedRepos: []github.Repo{{Name: oldName, Archived: true}},
		},
		{
			description: "request to publish a private repo fails when not allowed, but updates other fields",
			orgConfig: org.Config{
				Repos: map[string]org.Repo{
					oldName: {Private: &no, Description: &updated},
				},
			},
			repos:         []github.FullRepo{{Repo: github.Repo{Name: oldName, Private: true, Description: "OLD"}}},
			expectError:   true,
			expectedRepos: []github.Repo{{Name: oldName, Private: true, Description: updated}},
		},
		{
			description: "request to publish a private repo succeeds when allowed",
			opts: options{
				allowRepoPublish: true,
			},
			orgConfig: org.Config{
				Repos: map[string]org.Repo{
					oldName: {Private: &no},
				},
			},
			repos:         []github.FullRepo{{Repo: github.Repo{Name: oldName, Private: true}}},
			expectedRepos: []github.Repo{{Name: oldName, Private: false}},
		},
		{
			description: "renaming a repo is successful",
			orgConfig: org.Config{
				Repos: map[string]org.Repo{
					newName: {Previously: []string{oldName}},
				},
			},
			repos:         []github.FullRepo{{Repo: github.Repo{Name: oldName, Description: "renamed repo"}}},
			expectedRepos: []github.Repo{{Name: newName, Description: "renamed repo"}},
		},
		{
			description: "renaming a repo by just changing case is successful",
			orgConfig: org.Config{
				Repos: map[string]org.Repo{
					"repo": {Previously: []string{"REPO"}},
				},
			},
			repos:         []github.FullRepo{{Repo: github.Repo{Name: "REPO", Description: "renamed repo"}}},
			expectedRepos: []github.Repo{{Name: "repo", Description: "renamed repo"}},
		},
		{
			description: "dup between a repo name and a previous name is detected",
			orgConfig: org.Config{
				Repos: map[string]org.Repo{
					newName: {Previously: []string{oldName}},
					oldName: {Description: &newDescription},
				},
			},
			repos:         []github.FullRepo{{Repo: github.Repo{Name: oldName, Description: "this repo shall not be touched"}}},
			expectError:   true,
			expectedRepos: []github.Repo{{Name: oldName, Description: "this repo shall not be touched"}},
		},
		{
			description: "dup between two previous names is detected",
			orgConfig: org.Config{
				Repos: map[string]org.Repo{
					"wants-projects": {Previously: []string{oldName}, HasProjects: &yes, HasWiki: &no},
					"wants-wiki":     {Previously: []string{oldName}, HasProjects: &no, HasWiki: &yes},
				},
			},
			repos:         []github.FullRepo{{Repo: github.Repo{Name: oldName, Description: "this repo shall not be touched"}}},
			expectError:   true,
			expectedRepos: []github.Repo{{Name: oldName, Description: "this repo shall not be touched"}},
		},
		{
			description: "error detected when both a repo and a repo of its previous name exist",
			orgConfig: org.Config{
				Repos: map[string]org.Repo{
					newName: {Previously: []string{oldName}, Description: &newDescription},
				},
			},
			repos: []github.FullRepo{
				{Repo: github.Repo{Name: oldName, Description: "this repo shall not be touched"}},
				{Repo: github.Repo{Name: newName, Description: "this repo shall not be touched too"}},
			},
			expectError: true,
			expectedRepos: []github.Repo{
				{Name: newName, Description: "this repo shall not be touched too"},
				{Name: oldName, Description: "this repo shall not be touched"},
			},
		},
		{
			description: "error detected when multiple previous repos exist",
			orgConfig: org.Config{
				Repos: map[string]org.Repo{
					newName: {Previously: []string{oldName, "even-older"}, Description: &newDescription},
				},
			},
			repos: []github.FullRepo{
				{Repo: github.Repo{Name: oldName, Description: "this repo shall not be touched"}},
				{Repo: github.Repo{Name: "even-older", Description: "this repo shall not be touched too"}},
			},
			expectError: true,
			expectedRepos: []github.Repo{
				{Name: "even-older", Description: "this repo shall not be touched too"},
				{Name: oldName, Description: "this repo shall not be touched"},
			},
		},
		{
			description: "repos are renamed to defined case even without explicit `previously` field",
			orgConfig: org.Config{
				Repos: map[string]org.Repo{
					"CamelCase": {Description: &newDescription},
				},
			},
			repos:         []github.FullRepo{{Repo: github.Repo{Name: "CAMELCASE", Description: newDescription}}},
			expectedRepos: []github.Repo{{Name: "CamelCase", Description: newDescription}},
		},
		{
			description: "avoid creating archived repo",
			orgConfig: org.Config{
				Repos: map[string]org.Repo{
					oldName: {Archived: &yes},
				},
			},
			repos:         []github.FullRepo{},
			expectError:   true,
			expectedRepos: []github.Repo{},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			fc := makeFakeRepoClient(t, tc.repos...)
			var err error
			if len(tc.orgNameOverride) > 0 {
				err = configureRepos(tc.opts, fc, tc.orgNameOverride, tc.orgConfig)
			} else {
				err = configureRepos(tc.opts, fc, orgName, tc.orgConfig)
			}
			if err != nil && !tc.expectError {
				t.Errorf("%s: unexpected error: %v", tc.description, err)
			}
			if err == nil && tc.expectError {
				t.Errorf("%s: expected error, got none", tc.description)
			}

			reposAfter, err := fc.GetRepos(orgName, isOrg)
			if err != nil {
				t.Fatalf("%s: unexpected GetRepos error: %v", tc.description, err)
			}
			if !reflect.DeepEqual(reposAfter, tc.expectedRepos) {
				t.Errorf("%s: unexpected repos after configureRepos():\n%s", tc.description, cmp.Diff(reposAfter, tc.expectedRepos))
			}
		})
	}
}

func TestValidateRepos(t *testing.T) {
	description := "cool repo"
	testCases := []struct {
		description string
		config      map[string]org.Repo
		expectError bool
	}{
		{
			description: "handles nil map",
		},
		{
			description: "handles empty map",
			config:      map[string]org.Repo{},
		},
		{
			description: "handles valid config",
			config: map[string]org.Repo{
				"repo": {Description: &description},
			},
		},
		{
			description: "finds repo names duplicate when normalized",
			config: map[string]org.Repo{
				"repo": {Description: &description},
				"Repo": {Description: &description},
			},
			expectError: true,
		},
		{
			description: "finds name confict between previous and current names",
			config: map[string]org.Repo{
				"repo":     {Previously: []string{"conflict"}},
				"conflict": {Description: &description},
			},
			expectError: true,
		},
		{
			description: "finds name confict between two previous names",
			config: map[string]org.Repo{
				"repo":         {Previously: []string{"conflict"}},
				"another-repo": {Previously: []string{"conflict"}},
			},
			expectError: true,
		},
		{
			description: "allows case-duplicate name between former and current name",
			config: map[string]org.Repo{
				"repo": {Previously: []string{"REPO"}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			err := validateRepos(tc.config)
			if err == nil && tc.expectError {
				t.Errorf("%s: expected error, got none", tc.description)
			} else if err != nil && !tc.expectError {
				t.Errorf("%s: unexpected error: %v", tc.description, err)
			}
		})
	}
}

func TestNewRepoUpdateRequest(t *testing.T) {
	repoName := "repo-name"
	newRepoName := "renamed-repo"
	description := "description of repo-name"
	homepage := "https://somewhe.re"
	master := "master"
	branch := "branch"
	squashMergeCommitTitle := "PR_TITLE"
	squashMergeCommitMessage := "COMMIT_MESSAGES"

	testCases := []struct {
		description string
		current     github.FullRepo
		name        string
		newState    org.Repo

		expected github.RepoUpdateRequest
	}{
		{
			description: "update is just a delta from current state",
			current: github.FullRepo{
				Repo: github.Repo{
					Name:          repoName,
					Description:   description,
					Homepage:      homepage,
					DefaultBranch: master,
				},
			},
			name: repoName,
			newState: org.Repo{
				Description:   &description,
				DefaultBranch: &branch,
			},
			expected: github.RepoUpdateRequest{
				DefaultBranch: &branch,
			},
		},
		{
			description: "empty delta is returned when no update is needed",
			current: github.FullRepo{Repo: github.Repo{
				Name:        repoName,
				Description: description,
			}},
			name: repoName,
			newState: org.Repo{
				Description: &description,
			},
		},
		{
			description: "request to rename a repo works",
			current: github.FullRepo{Repo: github.Repo{
				Name: repoName,
			}},
			name: newRepoName,
			newState: org.Repo{
				Description: &description,
			},
			expected: github.RepoUpdateRequest{
				RepoRequest: github.RepoRequest{
					Name:        &newRepoName,
					Description: &description,
				},
			},
		},
		{
			description: "request to update commit messages works",
			current: github.FullRepo{
				Repo: github.Repo{
					Name: repoName,
				},
				SquashMergeCommitTitle:   "COMMIT_MESSAGES",
				SquashMergeCommitMessage: "COMMIT_OR_PR_TITLE",
			},
			name: newRepoName,
			newState: org.Repo{
				Description:              &description,
				SquashMergeCommitTitle:   &squashMergeCommitTitle,
				SquashMergeCommitMessage: &squashMergeCommitMessage,
			},
			expected: github.RepoUpdateRequest{
				RepoRequest: github.RepoRequest{
					Name:                     &newRepoName,
					Description:              &description,
					SquashMergeCommitTitle:   &squashMergeCommitTitle,
					SquashMergeCommitMessage: &squashMergeCommitMessage,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			update := newRepoUpdateRequest(tc.current, tc.name, tc.newState)
			if !reflect.DeepEqual(tc.expected, update) {
				t.Errorf("%s: update request differs from expected:%s", tc.description, cmp.Diff(tc.expected, update))
			}
		})
	}
}
