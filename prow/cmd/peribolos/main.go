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
	"io/ioutil"
	"net/url"
	"os"
	"strings"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/config/org"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/logrusutil"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	defaultEndpoint  = "https://api.github.com"
	defaultMinAdmins = 5
	defaultDelta     = 0.25
)

type options struct {
	config         string
	jobConfig      string
	token          string
	confirm        bool
	minAdmins      int
	requiredAdmins flagutil.Strings
	requireSelf    bool
	maximumDelta   float64
	endpoint       flagutil.Strings
}

func parseOptions() options {
	var o options
	if err := o.parseArgs(flag.CommandLine, os.Args[1:]); err != nil {
		logrus.Fatalf("Invalid flags: %v", err)
	}
	return o
}

func (o *options) parseArgs(flags *flag.FlagSet, args []string) error {
	o.endpoint = flagutil.NewStrings(defaultEndpoint)
	flags.Var(&o.endpoint, "github-endpoint", "Github api endpoint, may differ for enterprise")
	o.requiredAdmins = flagutil.NewStrings()
	flags.Var(&o.requiredAdmins, "required-admins", "Ensure config specifies these users as admins")
	flags.IntVar(&o.minAdmins, "min-admins", defaultMinAdmins, "Ensure config specifies at least this many admins")
	flags.BoolVar(&o.requireSelf, "require-self", true, "Ensure --github-token-path user is an admin")
	flags.Float64Var(&o.maximumDelta, "maximum-removal-delta", defaultDelta, "Fail if config removes more than this fraction of current members")
	flags.StringVar(&o.config, "config-path", "", "Path to prow config.yaml")
	flags.StringVar(&o.jobConfig, "job-config-path", "", "Path to prow job configs.")
	flags.BoolVar(&o.confirm, "confirm", false, "Mutate github if set")
	flags.StringVar(&o.token, "github-token-path", "", "Path to github token")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if o.config == "" {
		return errors.New("empty --config")
	}

	if o.token == "" {
		return errors.New("empty --github-token-path")
	}

	for _, ep := range o.endpoint.Strings() {
		_, err := url.Parse(ep)
		if err != nil {
			return fmt.Errorf("invalid --endpoint URL %q: %v", ep, err)
		}
	}
	if o.minAdmins < 2 {
		return fmt.Errorf("--min-admins=%d must be at least 2", o.minAdmins)
	}
	if o.maximumDelta > 1 || o.maximumDelta < 0 {
		return fmt.Errorf("--maximum-removal-delta=%f must be a non-negative number less than 1.0", o.maximumDelta)
	}

	return nil
}

func main() {
	logrus.SetFormatter(
		logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "peribolos"}),
	)
	o := parseOptions()

	if o.config != "TODO(fejta): implement me" {
		logrus.Fatalf("This program is not yet implemented") // still true
	}

	cfg, err := config.Load(o.config, o.jobConfig)
	if err != nil {
		logrus.Fatalf("Failed to load --config=%s: %v", o.config, err)
	}

	b, err := ioutil.ReadFile(o.token)
	if err != nil {
		logrus.Fatalf("cannot read --token: %v", err)
	}

	var c *github.Client
	tok := strings.TrimSpace(string(b))
	if o.confirm {
		c = github.NewClient(tok, o.endpoint.Strings()...)
	} else {
		c = github.NewDryRunClient(tok, o.endpoint.Strings()...)
	}
	c.Throttle(300, 100) // 300 hourly tokens, bursts of 100

	for name, orgcfg := range cfg.Orgs {
		if err := configureOrg(o, c, name, orgcfg); err != nil {
			logrus.Fatalf("Configuration failed: %v", err)
		}
	}
}

type orgClient interface {
	BotName() (string, error)
	ListOrgMembers(org, role string) ([]github.TeamMember, error)
	RemoveOrgMembership(org, user string) error
	UpdateOrgMembership(org, user string, admin bool) (*github.OrgMembership, error)
}

func configureOrgMembers(opt options, client orgClient, orgName string, orgConfig org.Config) error {
	// Get desired state
	wantAdmins := sets.NewString(orgConfig.Admins...)
	wantMembers := sets.NewString(orgConfig.Members...)

	// Sanity desired state
	if n := len(wantAdmins); n < opt.minAdmins {
		return fmt.Errorf("%s must specify at least %d admins, only found %d", orgName, opt.minAdmins, n)
	}
	var missing []string
	for _, r := range opt.requiredAdmins.Strings() {
		if !wantAdmins.Has(r) {
			missing = append(missing, r)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s must specify %v as admins, missing %v", orgName, opt.requiredAdmins, missing)
	}
	if opt.requireSelf {
		if me, err := client.BotName(); err != nil {
			return fmt.Errorf("cannot determine user making requests for %s: %v", opt.token, err)
		} else if !wantAdmins.Has(me) {
			return fmt.Errorf("authenticated user %s is not an admin of %s", me, orgName)
		}
	}

	// Get current state
	haveAdmins := sets.String{}
	haveMembers := sets.String{}
	ms, err := client.ListOrgMembers(orgName, "admin")
	if err != nil {
		return fmt.Errorf("failed to list %s admins: %v", orgName, err)
	}
	for _, m := range ms {
		haveAdmins.Insert(m.Login)
	}
	if ms, err = client.ListOrgMembers(orgName, "member"); err != nil {
		return fmt.Errorf("failed to list %s members: %v", orgName, err)
	}
	for _, m := range ms {
		haveMembers.Insert(m.Login)
	}

	have := memberships{members: haveMembers, super: haveAdmins}
	want := memberships{members: wantMembers, super: wantAdmins}
	// Figure out who to remove
	remove := have.all().Difference(want.all())

	// Sanity check changes
	if d := float64(len(remove)) / float64(len(have.all())); d > opt.maximumDelta {
		return fmt.Errorf("cannot delete %d memberships or %.3f of %s (exceeds limit of %.3f)", len(remove), d, orgName, opt.maximumDelta)
	}

	teamMembers := sets.String{}
	teamNames := sets.String{}
	duplicateTeamNames := sets.String{}
	for name, team := range orgConfig.Teams {
		teamMembers.Insert(team.Members...)
		teamMembers.Insert(team.Maintainers...)
		if teamNames.Has(name) {
			duplicateTeamNames.Insert(name)
		}
		teamNames.Insert(name)
		for _, n := range team.Previously {
			if teamNames.Has(n) {
				duplicateTeamNames.Insert(n)
			}
			teamNames.Insert(n)
		}
	}

	if outside := teamMembers.Difference(want.all()); len(outside) > 0 {
		return fmt.Errorf("all team members/maintainers must also be org members: %s", strings.Join(outside.List(), ", "))
	}

	if n := len(duplicateTeamNames); n > 0 {
		return fmt.Errorf("team names must be unique (including previous names), %d duplicated names: %s", n, strings.Join(duplicateTeamNames.List(), ", "))
	}

	adder := func(user string, super bool) error {
		role := "member"
		if super {
			role = "admin"
		}
		om, err := client.UpdateOrgMembership(orgName, user, super)
		if err != nil {
			logrus.WithError(err).Warnf("UpdateOrgMembership(%s, %s, %t) failed", orgName, user, super)
		} else if om.State == github.StatePending {
			logrus.Infof("Invited %s to %s as a %s", user, orgName, role)
		} else {
			logrus.Infof("Set %s as a %s of %s", user, role, orgName)
		}
		return err
	}

	remover := func(user string) error {
		err := client.RemoveOrgMembership(orgName, user)
		if err != nil {
			logrus.WithError(err).Warnf("RemoveOrgMembership(%s, %s) failed", orgName, user)
		}
		return err
	}

	if err = configureMembers(have, want, adder, remover); err != nil {
		return err
	}
	return nil
}

type memberships struct {
	members sets.String
	super   sets.String
}

func (m memberships) all() sets.String {
	return m.members.Union(m.super)
}

func configureMembers(have, want memberships, adder func(user string, super bool) error, remover func(user string) error) error {
	if both := want.super.Intersection(want.members); len(both) > 0 {
		return fmt.Errorf("users in both roles: %s", strings.Join(both.List(), ", "))
	}
	remove := have.all().Difference(want.all())
	members := want.members.Difference(have.members)
	supers := want.super.Difference(have.super)

	var errs []error
	for u := range members {
		if err := adder(u, false); err != nil {
			errs = append(errs, err)
		}
	}
	for u := range supers {
		if err := adder(u, true); err != nil {
			errs = append(errs, err)
		}
	}

	for u := range remove {
		if err := remover(u); err != nil {
			errs = append(errs, err)
		}
	}

	if n := len(errs); n > 0 {
		return fmt.Errorf("%d errors: %v", n, errs)
	}
	return nil
}

// findTeam returns teams[n] for the first n in [name, previousNames, ...] that is in teams.
func findTeam(teams map[string]github.Team, name string, previousNames ...string) *github.Team {
	if t, ok := teams[name]; ok {
		return &t
	}
	for _, p := range previousNames {
		if t, ok := teams[p]; ok {
			return &t
		}
	}
	return nil
}

// validateTeamNames returns an error if any current/previous names are used multiple times in the config.
func validateTeamNames(orgConfig org.Config) error {
	// Does the config duplicate any team names?
	used := sets.String{}
	dups := sets.String{}
	for name, orgTeam := range orgConfig.Teams {
		if used.Has(name) {
			dups.Insert(name)
		} else {
			used.Insert(name)
		}
		for _, n := range orgTeam.Previously {
			if used.Has(n) {
				dups.Insert(n)
			} else {
				used.Insert(n)
			}
		}
	}
	if n := len(dups); n > 0 {
		return fmt.Errorf("%d duplicated names: %s", n, strings.Join(dups.List(), ", "))
	}
	return nil
}

type teamClient interface {
	ListTeams(org string) ([]github.Team, error)
	CreateTeam(org string, team github.Team) (*github.Team, error)
	DeleteTeam(id int) error
}

// configureTeams returns the ids for all expected team names, creating/deleting teams as necessary.
func configureTeams(client teamClient, orgName string, orgConfig org.Config) (map[string]github.Team, error) {
	if err := validateTeamNames(orgConfig); err != nil {
		return nil, err
	}

	// What teams exist?
	ids := map[int]github.Team{}
	ints := sets.Int{}
	teamList, err := client.ListTeams(orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to list teams: %v", err)
	}
	for _, t := range teamList {
		ids[t.ID] = t
		ints.Insert(t.ID)
	}

	// What is the lowest ID for each team?
	older := map[string][]github.Team{}
	names := map[string]github.Team{}
	for _, t := range ids {
		n := t.Name
		switch val, ok := names[n]; {
		case !ok: // first occurrence of the name
			names[n] = t
		case ok && t.ID < val.ID: // t has the lower ID, replace and send current to older set
			names[n] = t
			older[n] = append(older[n], val)
		default: // t does not have smallest id, add it to older set
			older[n] = append(older[n], val)
		}
	}

	// What team are we using for each configured name, and which names are missing?
	matches := map[string]github.Team{}
	missing := map[string]org.Team{}
	used := sets.Int{}
	for name, orgTeam := range orgConfig.Teams {
		t := findTeam(names, name, orgTeam.Previously...)
		if t == nil {
			missing[name] = orgTeam
			continue
		}
		matches[name] = *t // t.Name != name if we matched on orgTeam.Previously
		used.Insert(t.ID)
	}

	// Create any missing team names
	var failures []string
	for name, orgTeam := range missing {
		t := &github.Team{Name: name}
		if orgTeam.Description != nil {
			t.Description = *orgTeam.Description
		}
		if orgTeam.Privacy != nil {
			t.Privacy = string(*orgTeam.Privacy)
		}
		t, err := client.CreateTeam(orgName, *t)
		if err != nil {
			logrus.WithError(err).Warnf("Failed to create %s in %s", name, orgName)
			failures = append(failures, name)
			continue
		}
		matches[name] = *t
		used.Insert(t.ID)
	}
	if n := len(failures); n > 0 {
		return nil, fmt.Errorf("failed to create %d teams: %s", n, strings.Join(failures, ", "))
	}

	// Delete undeclared teams.
	// TODO(fejta): consider ensuring we don't delete too many teams at once
	unused := ints.Difference(used)
	for id := range unused {
		if err := client.DeleteTeam(id); err != nil {
			str := fmt.Sprintf("%d(%s)", id, ids[id].Name)
			logrus.WithError(err).Warnf("Failed to delete team %s from %s", str, orgName)
			failures = append(failures, str)
		}
	}
	if n := len(failures); n > 0 {
		return nil, fmt.Errorf("failed to delete %d teams: %s", n, strings.Join(failures, ", "))
	}

	// Return matches
	return matches, nil
}

func configureOrg(opt options, client *github.Client, orgName string, orgConfig org.Config) error {
	// get meta
	// diff meta
	// patch meta

	// Find the id and current state of each declared team (create/delete as necessary)
	githubTeams, err := configureTeams(client, orgName, orgConfig)
	if err != nil {
		return fmt.Errorf("failed to configure %s teams: %v", orgName, err)
	}

	// Invite/remove/update members to the org.
	if err := configureOrgMembers(opt, client, orgName, orgConfig); err != nil {
		return fmt.Errorf("failed to configure %s members: %v", orgName, err)
	}

	for name, team := range orgConfig.Teams {
		gt, ok := githubTeams[name]
		if !ok { // configureTeams is buggy if this is the case
			return fmt.Errorf("%s not found in id list", name)
		}

		// Configure team metadata
		err := configureTeam(client, orgName, name, team, gt)
		if err != nil {
			return fmt.Errorf("failed to update %s: %v", orgName, err)
		}

		// Add/remove/update members to the team.
		return configureTeamMembers(client, gt.ID, team)
	}
	return nil
}

type editTeamClient interface {
	EditTeam(team github.Team) (*github.Team, error)
}

// configureTeam patches the team name/description/privacy when values differ
func configureTeam(client editTeamClient, orgName, teamName string, team org.Team, gt github.Team) error {
	// Do we need to reconfigure any team settings?
	patch := false
	if gt.Name != teamName {
		patch = true
	}
	gt.Name = teamName
	if team.Description != nil && gt.Description != *team.Description {
		patch = true
		gt.Description = *team.Description
	} else {
		gt.Description = ""
	}
	if team.Privacy != nil && gt.Privacy != string(*team.Privacy) {
		patch = true
		gt.Privacy = string(*team.Privacy)

	} else {
		gt.Privacy = ""
	}

	if patch { // yes we need to patch
		if _, err := client.EditTeam(gt); err != nil {
			return fmt.Errorf("failed to edit %s team %d(%s): %v", orgName, gt.ID, gt.Name, err)
		}
	}
	return nil
}

// teamMembersClient can list/remove/update people to a team.
type teamMembersClient interface {
	ListTeamMembers(id int, role string) ([]github.TeamMember, error)
	RemoveTeamMembership(id int, user string) error
	UpdateTeamMembership(id int, user string, maintainer bool) (*github.TeamMembership, error)
}

// configureTeamMembers will add/update people to the appropriate role on the team, and remove anyone else.
func configureTeamMembers(client teamMembersClient, id int, team org.Team) error {
	// Get desired state
	wantMaintainers := sets.NewString(team.Maintainers...)
	wantMembers := sets.NewString(team.Members...)

	// Get current state
	haveMaintainers := sets.String{}
	haveMembers := sets.String{}

	members, err := client.ListTeamMembers(id, github.RoleMember)
	if err != nil {
		return fmt.Errorf("failed to list %d members: %v", id, err)
	}
	for _, m := range members {
		haveMembers.Insert(m.Login)
	}

	maintainers, err := client.ListTeamMembers(id, github.RoleMaintainer)
	if err != nil {
		return fmt.Errorf("failed to list %d maintainers: %v", id, err)
	}
	for _, m := range maintainers {
		haveMaintainers.Insert(m.Login)
	}

	adder := func(user string, super bool) error {
		role := github.RoleMember
		if super {
			role = github.RoleMaintainer
		}
		tm, err := client.UpdateTeamMembership(id, user, super)
		if err != nil {
			logrus.WithError(err).Warnf("UpdateTeamMembership(%d, %s, %t) failed", id, user, super)
		} else if tm.State == github.StatePending {
			logrus.Infof("Invited %s to %d as a %s", user, id, role)
		} else {
			logrus.Infof("Set %s as a %s of %d", user, role, id)
		}
		return err
	}

	remover := func(user string) error {
		err := client.RemoveTeamMembership(id, user)
		if err != nil {
			logrus.WithError(err).Warnf("RemoveTeamMembership(%d, %s) failed", id, user)
		} else {
			logrus.Infof("Removed %s from team %d", user, id)
		}
		return err
	}

	want := memberships{members: wantMembers, super: wantMaintainers}
	have := memberships{members: haveMembers, super: haveMaintainers}
	return configureMembers(have, want, adder, remover)
}
