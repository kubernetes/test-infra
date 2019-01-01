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
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/yaml"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/config/org"
	"k8s.io/test-infra/prow/config/secret"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/logrusutil"
)

const (
	defaultMinAdmins = 5
	defaultDelta     = 0.25
	defaultTokens    = 300
	defaultBurst     = 100
)

type options struct {
	config               string
	confirm              bool
	dump                 string
	dumpRepos            bool
	repos                string
	jobConfig            string
	maximumDelta         float64
	minAdmins            int
	requireSelf          bool
	requiredAdmins       flagutil.Strings
	fixOrg               bool
	fixOrgMembers        bool
	fixTeamMembers       bool
	fixTeams             bool
	fixRepoCollaborators bool
	github               flagutil.GitHubOptions
	tokenBurst           int
	tokensPerHour        int
}

func parseOptions() options {
	var o options
	if err := o.parseArgs(flag.CommandLine, os.Args[1:]); err != nil {
		logrus.Fatalf("Invalid flags: %v", err)
	}
	return o
}

func (o *options) parseArgs(flags *flag.FlagSet, args []string) error {
	o.requiredAdmins = flagutil.NewStrings()
	flags.Var(&o.requiredAdmins, "required-admins", "Ensure config specifies these users as admins")
	flags.IntVar(&o.minAdmins, "min-admins", defaultMinAdmins, "Ensure config specifies at least this many admins")
	flags.BoolVar(&o.requireSelf, "require-self", true, "Ensure --github-token-path user is an admin")
	flags.Float64Var(&o.maximumDelta, "maximum-removal-delta", defaultDelta, "Fail if config removes more than this fraction of current members")
	flags.StringVar(&o.config, "config-path", "", "Path to prow config.yaml")
	flags.StringVar(&o.jobConfig, "job-config-path", "", "Path to prow job configs.")
	flags.BoolVar(&o.confirm, "confirm", false, "Mutate github if set")
	flags.IntVar(&o.tokensPerHour, "tokens", defaultTokens, "Throttle hourly token consumption (0 to disable)")
	flags.IntVar(&o.tokenBurst, "token-burst", defaultBurst, "Allow consuming a subset of hourly tokens in a short burst")
	flags.StringVar(&o.dump, "dump", "", "Output current config of this org if set")
	flags.BoolVar(&o.dumpRepos, "dump-repos", false, "Output current config of repos under the org if set")
	flags.StringVar(&o.repos, "repos", "", "Add/remove/update/dump collaborators of only these comma separated repos. If empty, all repos are updated")
	flags.BoolVar(&o.fixOrg, "fix-org", false, "Change org metadata if set")
	flags.BoolVar(&o.fixOrgMembers, "fix-org-members", false, "Add/remove org members if set")
	flags.BoolVar(&o.fixTeams, "fix-teams", false, "Create/delete/update teams if set")
	flags.BoolVar(&o.fixTeamMembers, "fix-team-members", false, "Add/remove team members if set")
	flags.BoolVar(&o.fixRepoCollaborators, "fix-repo-collaborators", false, "Add/update/remove repo collaborators if set")
	o.github.AddFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if err := o.github.Validate(!o.confirm); err != nil {
		return err
	}
	if o.tokensPerHour > 0 && o.tokenBurst >= o.tokensPerHour {
		return fmt.Errorf("--tokens=%d must exceed --token-burst=%d", o.tokensPerHour, o.tokenBurst)
	}

	if o.minAdmins < 2 {
		return fmt.Errorf("--min-admins=%d must be at least 2", o.minAdmins)
	}
	if o.maximumDelta > 1 || o.maximumDelta < 0 {
		return fmt.Errorf("--maximum-removal-delta=%f must be a non-negative number less than 1.0", o.maximumDelta)
	}

	if o.confirm && o.dump != "" {
		return fmt.Errorf("--confirm cannot be used with --dump=%s", o.dump)
	}
	if o.config == "" && o.dump == "" {
		return errors.New("--config-path or --dump required")
	}
	if o.config != "" && o.dump != "" {
		return fmt.Errorf("--config-path=%s and --dump=%s cannot both be set", o.config, o.dump)
	}

	if o.dumpRepos && o.dump == "" {
		return fmt.Errorf("--dump-repos requires --dump")
	}

	if o.fixTeamMembers && !o.fixTeams {
		return fmt.Errorf("--fix-team-members requires --fix-teams")
	}

	return nil
}

func main() {
	logrus.SetFormatter(
		logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "peribolos"}),
	)
	o := parseOptions()

	secretAgent := &secret.Agent{}
	if err := secretAgent.Start([]string{o.github.TokenPath}); err != nil {
		logrus.WithError(err).Fatal("Error starting secrets agent.")
	}

	githubClient, err := o.github.GitHubClient(secretAgent, !o.confirm)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting GitHub client.")
	}
	if o.tokensPerHour > 0 {
		githubClient.Throttle(o.tokensPerHour, o.tokenBurst) // 300 hourly tokens, bursts of 100 (default)
	}

	if o.dump != "" {
		ret, err := dumpOrgConfig(githubClient, o.dump, o.repos, o.dumpRepos)
		if err != nil {
			logrus.WithError(err).Fatalf("Dump %s failed to collect current data.", o.dump)
		}
		out, err := yaml.Marshal(ret)
		if err != nil {
			logrus.WithError(err).Fatalf("Dump %s failed to marshal output.", o.dump)
		}
		logrus.Infof("Dumping orgs[\"%s\"]:", o.dump)
		fmt.Println(string(out))
		return
	}

	cfg, err := config.Load(o.config, o.jobConfig)
	if err != nil {
		logrus.Fatalf("Failed to load --config=%s: %v", o.config, err)
	}

	for name, orgcfg := range cfg.Orgs {
		if err := configureOrg(o, githubClient, name, orgcfg); err != nil {
			logrus.Fatalf("Configuration failed: %v", err)
		}
	}
}

type dumpClient interface {
	GetOrg(name string) (*github.Organization, error)
	GetRepos(org string, isUser bool) ([]github.Repo, error)
	GetUserPermission(org, repo, user string) (string, error)
	ListOrgMembers(org, role string) ([]github.TeamMember, error)
	ListTeams(org string) ([]github.Team, error)
	ListTeamMembers(id int, role string) ([]github.TeamMember, error)
	ListTeamsForRepo(org, repo string) ([]github.Team, error)
	ListCollaborators(org, repo, affiliation string) ([]github.User, error)
}

func dumpOrgConfig(client dumpClient, orgName, repos string, dumpRepos bool) (*org.Config, error) {
	out := org.Config{}
	meta, err := client.GetOrg(orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to get org: %v", err)
	}
	out.Metadata.BillingEmail = &meta.BillingEmail
	out.Metadata.Company = &meta.Company
	out.Metadata.Email = &meta.Email
	out.Metadata.Name = &meta.Name
	out.Metadata.Description = &meta.Description
	out.Metadata.Location = &meta.Location
	out.Metadata.HasOrganizationProjects = &meta.HasOrganizationProjects
	out.Metadata.HasRepositoryProjects = &meta.HasRepositoryProjects
	drp := org.RepoPermissionLevel(meta.DefaultRepositoryPermission)
	out.Metadata.DefaultRepositoryPermission = &drp
	out.Metadata.MembersCanCreateRepositories = &meta.MembersCanCreateRepositories

	admins, err := client.ListOrgMembers(orgName, github.RoleAdmin)
	if err != nil {
		return nil, fmt.Errorf("failed to list org admins: %v", err)
	}
	for _, m := range admins {
		out.Admins = append(out.Admins, m.Login)
	}

	orgMembers, err := client.ListOrgMembers(orgName, github.RoleMember)
	if err != nil {
		return nil, fmt.Errorf("failed to list org members: %v", err)
	}
	for _, m := range orgMembers {
		out.Members = append(out.Members, m.Login)
	}

	teams, err := client.ListTeams(orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to list teams: %v", err)
	}

	out.Teams, err = createTeamConfig(client, teams)
	if err != nil {
		return nil, fmt.Errorf("failed to create team config: %v", err)
	}

	if dumpRepos {
		out.Repos, err = createRepoConfig(client, orgName, repos)
		if err != nil {
			return nil, fmt.Errorf("failed to create repo config: %v", err)
		}
	}

	return &out, nil
}

func createTeamConfig(client dumpClient, teams []github.Team) (map[string]org.Team, error) {
	names := map[int]string{}   // what's the name of a team?
	idMap := map[int]org.Team{} // metadata for a team
	children := map[int][]int{} // what children does it have
	var tops []int              // what are the top-level teams

	for _, t := range teams {
		p := org.Privacy(t.Privacy)
		d := t.Description
		nt := org.Team{
			TeamMetadata: org.TeamMetadata{
				Description: &d,
				Privacy:     &p,
			},
			Maintainers: []string{},
			Members:     []string{},
			Children:    map[string]org.Team{},
		}
		maintainers, err := client.ListTeamMembers(t.ID, github.RoleMaintainer)
		if err != nil {
			return nil, fmt.Errorf("failed to list team %d(%s) maintainers: %v", t.ID, t.Name, err)
		}
		for _, m := range maintainers {
			nt.Maintainers = append(nt.Maintainers, m.Login)
		}
		teamMembers, err := client.ListTeamMembers(t.ID, github.RoleMember)
		if err != nil {
			return nil, fmt.Errorf("failed to list team %d(%s) members: %v", t.ID, t.Name, err)
		}
		for _, m := range teamMembers {
			nt.Members = append(nt.Members, m.Login)
		}

		names[t.ID] = t.Name
		idMap[t.ID] = nt

		if t.Parent == nil { // top level team
			tops = append(tops, t.ID)
		} else { // add this id to the list of the parent's children
			children[t.Parent.ID] = append(children[t.Parent.ID], t.ID)
		}
	}

	var makeChild func(id int) org.Team
	makeChild = func(id int) org.Team {
		t := idMap[id]
		for _, cid := range children[id] {
			child := makeChild(cid)
			t.Children[names[cid]] = child
		}
		return t
	}

	result := make(map[string]org.Team, len(tops))
	for _, id := range tops {
		result[names[id]] = makeChild(id)
	}

	return result, nil
}

func createRepoConfig(client dumpClient, orgName, repoList string) (map[string]org.Repo, error) {
	repos := strings.Split(repoList, ",")
	if len(repos) == 0 {
		// get all repos
		allRepos, err := client.GetRepos(orgName, false)
		if err != nil {
			return nil, fmt.Errorf("failed to get repos: %v", err)
		}

		for _, r := range allRepos {
			repos = append(repos, r.Name)
		}
	}

	result := make(map[string]org.Repo, len(repos))
	for _, repo := range repos {
		// get outside collaborators
		oc, err := client.ListCollaborators(orgName, repo, github.AffiliationOutside)
		if err != nil {
			return nil, err
		}
		var outsideCollaborators []string
		for _, u := range oc {
			outsideCollaborators = append(outsideCollaborators, u.Login)
		}

		var ocAdmin, ocWrite, ocRead []string
		for _, user := range outsideCollaborators {
			permission, err := client.GetUserPermission(orgName, repo, user)
			if err != nil {
				return nil, err
			}
			switch permission {
			case string(org.Admin):
				ocAdmin = append(ocAdmin, user)
			case string(org.Write):
				ocWrite = append(ocWrite, user)
			case string(org.Read):
				ocRead = append(ocRead, user)
			}
		}

		// get members with direct collaborator access
		dc, err := client.ListCollaborators(orgName, repo, github.AffiliationDirect)
		if err != nil {
			return nil, err
		}
		var directCollaborators []string
		for _, u := range dc {
			directCollaborators = append(directCollaborators, u.Login)
		}
		directMembers := sets.NewString(directCollaborators...).Difference(sets.NewString(outsideCollaborators...))

		var membersAdmin, membersWrite, membersRead []string
		members := directMembers.List()
		for _, user := range members {
			permission, err := client.GetUserPermission(orgName, repo, user)
			if err != nil {
				return nil, err
			}
			switch permission {
			case string(org.Admin):
				membersAdmin = append(membersAdmin, user)
			case string(org.Write):
				membersWrite = append(membersWrite, user)
			case string(org.Read):
				membersRead = append(membersRead, user)
			}
		}

		// get all teams who have access to this repo
		teams, err := client.ListTeamsForRepo(orgName, repo)
		if err != nil {
			return nil, err
		}
		var teamAdmin, teamWrite, teamRead []string
		for _, team := range teams {
			switch team.Permission {
			case string(org.Admin):
				teamAdmin = append(teamAdmin, team.Name)
			case string("push"):
				teamWrite = append(teamWrite, team.Name)
			case string("pull"):
				teamRead = append(teamRead, team.Name)
			}
		}
		result[repo] = org.Repo{
			Members: org.UserPermissionLevel{
				Admin: membersAdmin,
				Write: membersWrite,
				Read:  membersRead,
			},
			OutsideCollaborators: org.UserPermissionLevel{
				Admin: ocAdmin,
				Write: ocWrite,
				Read:  ocRead,
			},
			Teams: org.UserPermissionLevel{
				Admin: teamAdmin,
				Write: teamWrite,
				Read:  teamRead,
			},
		}
	}
	return result, nil
}

type orgClient interface {
	BotName() (string, error)
	ListOrgMembers(org, role string) ([]github.TeamMember, error)
	RemoveOrgMembership(org, user string) error
	UpdateOrgMembership(org, user string, admin bool) (*github.OrgMembership, error)
}

func configureOrgMembers(opt options, client orgClient, orgName string, orgConfig org.Config, invitees sets.String) error {
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
			return fmt.Errorf("cannot determine user making requests for %s: %v", opt.github.TokenPath, err)
		} else if !wantAdmins.Has(me) {
			return fmt.Errorf("authenticated user %s is not an admin of %s", me, orgName)
		}
	}

	// Get current state
	haveAdmins := sets.String{}
	haveMembers := sets.String{}
	ms, err := client.ListOrgMembers(orgName, github.RoleAdmin)
	if err != nil {
		return fmt.Errorf("failed to list %s admins: %v", orgName, err)
	}
	for _, m := range ms {
		haveAdmins.Insert(m.Login)
	}
	if ms, err = client.ListOrgMembers(orgName, github.RoleMember); err != nil {
		return fmt.Errorf("failed to list %s members: %v", orgName, err)
	}
	for _, m := range ms {
		haveMembers.Insert(m.Login)
	}

	have := memberships{members: haveMembers, super: haveAdmins}
	want := memberships{members: wantMembers, super: wantAdmins}
	have.normalize()
	want.normalize()
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

	teamMembers = normalize(teamMembers)
	if outside := teamMembers.Difference(want.all()); len(outside) > 0 {
		return fmt.Errorf("all team members/maintainers must also be org members: %s", strings.Join(outside.List(), ", "))
	}

	if n := len(duplicateTeamNames); n > 0 {
		return fmt.Errorf("team names must be unique (including previous names), %d duplicated names: %s", n, strings.Join(duplicateTeamNames.List(), ", "))
	}

	adder := func(user string, super bool) error {
		if invitees.Has(user) { // Do not add them, as this causes another invite.
			logrus.Infof("Waiting for %s to accept invitation to %s", user, orgName)
			return nil
		}
		role := github.RoleMember
		if super {
			role = github.RoleAdmin
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

	return configureMembers(have, want, invitees, adder, remover)
}

type memberships struct {
	members sets.String
	super   sets.String
}

func (m memberships) all() sets.String {
	return m.members.Union(m.super)
}

func normalize(s sets.String) sets.String {
	out := sets.String{}
	for i := range s {
		out.Insert(github.NormLogin(i))
	}
	return out
}

func (m *memberships) normalize() {
	m.members = normalize(m.members)
	m.super = normalize(m.super)
}

func configureMembers(have, want memberships, invitees sets.String, adder func(user string, super bool) error, remover func(user string) error) error {
	have.normalize()
	want.normalize()
	if both := want.super.Intersection(want.members); len(both) > 0 {
		return fmt.Errorf("users in both roles: %s", strings.Join(both.List(), ", "))
	}
	havePlusInvites := have.all().Union(invitees)
	remove := havePlusInvites.Difference(want.all())
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
func configureTeams(client teamClient, orgName string, orgConfig org.Config, maxDelta float64) (map[string]github.Team, error) {
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
	var match func(teams map[string]org.Team)
	match = func(teams map[string]org.Team) {
		for name, orgTeam := range teams {
			match(orgTeam.Children)
			t := findTeam(names, name, orgTeam.Previously...)
			if t == nil {
				missing[name] = orgTeam
				continue
			}
			matches[name] = *t // t.Name != name if we matched on orgTeam.Previously
			used.Insert(t.ID)
		}
	}
	match(orgConfig.Teams)

	// First compute teams we will delete, ensure we are not deleting too many
	unused := ints.Difference(used)
	if delta := float64(len(unused)) / float64(len(ints)); delta > maxDelta {
		return nil, fmt.Errorf("cannot delete %d teams or %.3f of %s teams (exceeds limit of %.3f)", len(unused), delta, orgName, maxDelta)
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
		// t.ID may include an ID already present in ints if other actors are deleting teams.
		used.Insert(t.ID)
	}
	if n := len(failures); n > 0 {
		return nil, fmt.Errorf("failed to create %d teams: %s", n, strings.Join(failures, ", "))
	}

	// Remove any IDs returned by CreateTeam() that are in the unused set.
	if reused := unused.Intersection(used); len(reused) > 0 {
		// Logically possible for:
		// * another actor to delete team N after the ListTeams() call
		// * github to reuse team N after someone deleted it
		// Therefore used may now include IDs in unused, handle this situation.
		logrus.Warnf("Will not delete %d team IDs reused by github: %v", len(reused), reused.List())
		unused = unused.Difference(reused)
	}
	// Delete undeclared teams.
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

// updateString will return true and set have to want iff they are set and different.
func updateString(have, want *string) bool {
	switch {
	case have == nil:
		panic("have must be non-nil")
	case want == nil:
		return false // do not care what we have
	case *have == *want:
		return false // already have it
	}
	*have = *want // update value
	return true
}

// updateBool will return true and set have to want iff they are set and different.
func updateBool(have, want *bool) bool {
	switch {
	case have == nil:
		panic("have must not be nil")
	case want == nil:
		return false // do not care what we have
	case *have == *want:
		return false //already have it
	}
	*have = *want // update value
	return true
}

type orgMetadataClient interface {
	GetOrg(name string) (*github.Organization, error)
	EditOrg(name string, org github.Organization) (*github.Organization, error)
}

// configureOrgMeta will update github to have the non-nil wanted metadata values.
func configureOrgMeta(client orgMetadataClient, orgName string, want org.Metadata) error {
	cur, err := client.GetOrg(orgName)
	if err != nil {
		return fmt.Errorf("failed to get %s metadata: %v", orgName, err)
	}
	change := false
	change = updateString(&cur.BillingEmail, want.BillingEmail) || change
	change = updateString(&cur.Company, want.Company) || change
	change = updateString(&cur.Email, want.Email) || change
	change = updateString(&cur.Name, want.Name) || change
	change = updateString(&cur.Description, want.Description) || change
	change = updateString(&cur.Location, want.Location) || change
	if want.DefaultRepositoryPermission != nil {
		w := string(*want.DefaultRepositoryPermission)
		change = updateString(&cur.DefaultRepositoryPermission, &w)
	}
	change = updateBool(&cur.HasOrganizationProjects, want.HasOrganizationProjects) || change
	change = updateBool(&cur.HasRepositoryProjects, want.HasRepositoryProjects) || change
	change = updateBool(&cur.MembersCanCreateRepositories, want.MembersCanCreateRepositories) || change
	if change {
		if _, err := client.EditOrg(orgName, *cur); err != nil {
			return fmt.Errorf("failed to edit %s metadata: %v", orgName, err)
		}
	}
	return nil
}

type inviteClient interface {
	ListOrgInvitations(org string) ([]github.OrgInvitation, error)
}

func orgInvitations(opt options, client inviteClient, orgName string) (sets.String, error) {
	invitees := sets.String{}
	if !opt.fixOrgMembers && !opt.fixTeamMembers {
		return invitees, nil
	}
	is, err := client.ListOrgInvitations(orgName)
	if err != nil {
		return nil, err
	}
	for _, i := range is {
		if i.Login == "" {
			continue
		}
		invitees.Insert(github.NormLogin(i.Login))
	}
	return invitees, nil
}

func configureOrg(opt options, client *github.Client, orgName string, orgConfig org.Config) error {
	// Ensure that metadata is configured correctly.
	if !opt.fixOrg {
		logrus.Infof("Skipping org metadata configuration")
	} else if err := configureOrgMeta(client, orgName, orgConfig.Metadata); err != nil {
		return err
	}

	invitees, err := orgInvitations(opt, client, orgName)
	if err != nil {
		return fmt.Errorf("failed to list %s invitations: %v", orgName, err)
	}

	// Invite/remove/update members to the org.
	if !opt.fixOrgMembers {
		logrus.Infof("Skipping org member configuration")
	} else if err := configureOrgMembers(opt, client, orgName, orgConfig, invitees); err != nil {
		return fmt.Errorf("failed to configure %s members: %v", orgName, err)
	}

	if !opt.fixTeams {
		logrus.Infof("Skipping team and team member configuration")
	} else {
		// Find the id and current state of each declared team (create/delete as necessary)
		githubTeams, err := configureTeams(client, orgName, orgConfig, opt.maximumDelta)
		if err != nil {
			return fmt.Errorf("failed to configure %s teams: %v", orgName, err)
		}

		for name, team := range orgConfig.Teams {
			err := configureTeamAndMembers(client, opt.fixTeamMembers, githubTeams, name, orgName, team, invitees, nil)
			if err != nil {
				return fmt.Errorf("failed to configure %s teams: %v", orgName, err)
			}
		}
	}

	if !opt.fixRepoCollaborators {
		logrus.Infof("Skipping repo collaborator configuration")
	} else if err := configureRepoCollaborators(opt, client, orgName, orgConfig, invitees); err != nil {
		return fmt.Errorf("failed to configure repo collaborators for repos in the %s org: %v", orgName, err)
	}

	return nil
}

func configureTeamAndMembers(client *github.Client, fixMembers bool, githubTeams map[string]github.Team, name, orgName string, team org.Team, invitees sets.String, parent *int) error {
	gt, ok := githubTeams[name]
	if !ok { // configureTeams is buggy if this is the case
		return fmt.Errorf("%s not found in id list", name)
	}

	// Configure team metadata
	err := configureTeam(client, orgName, name, team, gt, parent)
	if err != nil {
		return fmt.Errorf("failed to update %s metadata: %v", name, err)
	}

	// Configure team members
	if !fixMembers {
		logrus.Infof("Skipping %s member configuration", name)
	} else if err = configureTeamMembers(client, gt.ID, team, invitees); err != nil {
		return fmt.Errorf("failed to update %s members: %v", name, err)
	}

	for childName, childTeam := range team.Children {
		err = configureTeamAndMembers(client, fixMembers, githubTeams, childName, orgName, childTeam, invitees, &gt.ID)
		if err != nil {
			return fmt.Errorf("failed to update %s child teams: %v", name, err)
		}
	}

	return nil
}

type editTeamClient interface {
	EditTeam(team github.Team) (*github.Team, error)
}

// configureTeam patches the team name/description/privacy when values differ
func configureTeam(client editTeamClient, orgName, teamName string, team org.Team, gt github.Team, parent *int) error {
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
	// doesn't have parent in github, but has parent in config
	if gt.Parent == nil && parent != nil {
		patch = true
		gt.ParentTeamID = parent
	}
	if gt.Parent != nil { // has parent in github ...
		if parent == nil { // ... but doesn't need one
			patch = true
			gt.Parent = nil
			gt.ParentTeamID = parent
		} else if gt.Parent.ID != *parent { // but it's different than the config
			patch = true
			gt.Parent = nil
			gt.ParentTeamID = parent
		}
	}

	if team.Privacy != nil && gt.Privacy != string(*team.Privacy) {
		patch = true
		gt.Privacy = string(*team.Privacy)

	} else if team.Privacy == nil && (parent != nil || len(team.Children) > 0) && gt.Privacy != "closed" {
		patch = true
		gt.Privacy = github.PrivacyClosed // nested teams must be closed
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
func configureTeamMembers(client teamMembersClient, id int, team org.Team, invitees sets.String) error {
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
		if invitees.Has(user) {
			logrus.Infof("Waiting for %s to accept invitation to %d", user, id)
			return nil
		}
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
	return configureMembers(have, want, invitees, adder, remover)
}

type repoClient interface {
	ListCollaborators(org, repo, affiliation string) ([]github.User, error)
	UpdateCollaborator(org, repo, user, permission string) error
	RemoveCollaborator(org, repo, user string) error
	GetUserPermission(org, repo, user string) (string, error)

	ListTeams(org string) ([]github.Team, error)
	ListTeamsForRepo(org, repo string) ([]github.Team, error)
	UpdateTeamForRepo(org, repo, permission string, id int) error
	RemoveTeamFromRepo(org, repo string, id int) error
}

type permissionLevels struct {
	admin sets.String
	write sets.String
	read  sets.String
}

func (p permissionLevels) all() sets.String {
	return p.admin.Union(p.write).Union(p.read)
}

func (p *permissionLevels) normalize() {
	p.admin = normalize(p.admin)
	p.write = normalize(p.write)
	p.read = normalize(p.read)
}

func configureRepoCollaborators(opt options, client repoClient, orgName string, orgConfig org.Config, invitees sets.String) error {
	if orgConfig.Repos == nil {
		return nil
	}

	members := sets.NewString(orgConfig.Members...)
	repos := strings.Split(opt.repos, ",")
	if len(repos) == 0 {
		for r := range orgConfig.Repos {
			repos = append(repos, r)
		}
	}

	var errs []error
	for _, repo := range repos {
		config, ok := orgConfig.Repos[repo]
		if !ok {
			errs = append(errs, fmt.Errorf("config for the %s repo does not exist, retry with --dump-repos", repo))
			continue
		}
		if err := validateRepoCollaborators(config); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := configureUserCollaborators(opt, client, orgName, repo, github.AffiliationDirect, config.Members, invitees, members); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := configureUserCollaborators(opt, client, orgName, repo, github.AffiliationOutside, config.OutsideCollaborators, invitees, members); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := configureTeamCollaborators(opt, client, orgName, repo, config.Teams, invitees); err != nil {
			errs = append(errs, err)
			continue
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("could not update all repos: %v", errs)
	}
	return nil
}

func configureUserCollaborators(opt options, client repoClient, orgName, repo, affiliation string, config org.UserPermissionLevel, invitees, members sets.String) error {
	// get desired state
	wantAdmin := sets.NewString(config.Admin...)
	wantWrite := sets.NewString(config.Write...)
	wantRead := sets.NewString(config.Read...)

	// get current state
	var users []string
	var haveAdmin, haveWrite, haveRead sets.String

	collaborators, err := client.ListCollaborators(orgName, repo, affiliation)
	if err != nil {
		return err
	}
	for _, u := range collaborators {
		users = append(users, u.Login)
	}
	if affiliation == string(github.AffiliationDirect) {
		users = sets.NewString(users...).Intersection(members).List()
	}
	for _, user := range users {
		permission, err := client.GetUserPermission(orgName, repo, user)
		if err != nil {
			return err
		}
		switch permission {
		case string(org.Admin):
			haveAdmin.Insert(user)
		case string(org.Write):
			haveWrite.Insert(user)
		case string(org.Read):
			haveRead.Insert(user)
		}
	}

	have := permissionLevels{admin: haveAdmin, write: haveWrite, read: haveRead}
	want := permissionLevels{admin: wantAdmin, write: wantWrite, read: wantRead}
	have.normalize()
	want.normalize()

	// figure out who to remove
	remove := have.all().Difference(want.all())

	// sanity check changes
	if d := float64(len(remove)) / float64(len(have.all())); d > opt.maximumDelta {
		return fmt.Errorf("cannot delete %d repo collaborators or %.3f of %s/%s (exceeds limit of %.3f)", len(remove), d, orgName, repo, opt.maximumDelta)
	}

	adder := func(user, permission string) error {
		if invitees.Has(user) { // Do not add them, as this causes another invite.
			logrus.Infof("Waiting for %s to accept invitation as a collaborator to %s/%s", user, orgName, repo)
			return nil
		}

		isMember := members.Has(user)
		if isMember && affiliation == string(github.AffiliationOutside) {
			logrus.Infof("Cannot add/update %s as outside collaborator to %s/%s since they are a member", user, orgName, repo)
			return nil
		}

		if !isMember && affiliation == string(github.AffiliationDirect) {
			logrus.Infof("%s is not a member of the org %s, retry as an outside collaborator or apply for membership", user, orgName)
			return nil
		}

		err := client.UpdateCollaborator(orgName, repo, user, permission)
		if err != nil {
			logrus.WithError(err).Warnf("UpdateCollaborator(%s, %s, %s, %s) failed", orgName, repo, user, permission)
		} else {
			logrus.Infof("Set %s with %s access to %s/%s", user, permission, orgName, repo)
		}
		return err
	}

	remover := func(user string) error {
		err := client.RemoveCollaborator(orgName, repo, user)
		if err != nil {
			logrus.WithError(err).Warnf("RemoveCollaborator(%s, %s, %s) failed", orgName, repo, user)
		}
		return err
	}

	return configureCollaborators(have, want, invitees, adder, remover)
}

func configureTeamCollaborators(opt options, client repoClient, orgName, repo string, config org.UserPermissionLevel, invitees sets.String) error {
	// get desired state
	wantAdmin := sets.NewString(config.Admin...)
	wantWrite := sets.NewString(config.Write...)
	wantRead := sets.NewString(config.Read...)

	// get current state
	teams, err := client.ListTeamsForRepo(orgName, repo)
	if err != nil {
		return err
	}
	var haveAdmin, haveWrite, haveRead sets.String
	namesToID := map[string]int{}
	for _, team := range teams {
		namesToID[team.Name] = team.ID
		switch team.Permission {
		case string(org.Admin):
			haveAdmin.Insert(team.Name)
		case string("push"):
			haveWrite.Insert(team.Name)
		case string("pull"):
			haveRead.Insert(team.Name)
		}
	}

	have := permissionLevels{admin: haveAdmin, write: haveWrite, read: haveRead}
	want := permissionLevels{admin: wantAdmin, write: wantWrite, read: wantRead}
	have.normalize()
	want.normalize()

	// figure out who to remove
	remove := have.all().Difference(want.all())

	// sanity check changes
	if d := float64(len(remove)) / float64(len(have.all())); d > opt.maximumDelta {
		return fmt.Errorf("cannot delete %d teams as collaborators or %.3f of %s/%s (exceeds limit of %.3f)", len(remove), d, orgName, repo, opt.maximumDelta)
	}

	adder := func(team, permission string) error {
		var teamID int
		teamID, ok := namesToID[team]
		if !ok {
			// this means we are adding a new team to the repo
			// list all teams to get its ID
			allTeams, err := client.ListTeams(orgName)
			if err != nil {
				return fmt.Errorf("failed to list teams: %v", err)
			}

			for _, t := range allTeams {
				if _, ok := namesToID[t.Name]; !ok {
					namesToID[t.Name] = t.ID
				}
			}

			// look for the ID again
			id, ok := namesToID[team]
			if !ok {
				return fmt.Errorf("invalid team name %s", team)
			}
			teamID = id
		}

		err := client.UpdateTeamForRepo(orgName, repo, permission, teamID)
		if err != nil {
			logrus.WithError(err).Warnf("UpdateTeamForRepo(%s, %s, %s, %s) failed", orgName, repo, team, permission)
		} else {
			logrus.Infof("Set %s with the permission \"%s\" to %s/%s", team, permission, orgName, repo)
		}
		return err
	}

	remover := func(team string) error {
		teamID, ok := namesToID[team]
		if !ok {
			return fmt.Errorf("invalid team name %s specified to be removed", team)
		}

		err := client.RemoveTeamFromRepo(orgName, repo, teamID)
		if err != nil {
			logrus.WithError(err).Warnf("RemoveTeamFromRepo(%s, %s, %s) failed", orgName, repo, team)
		}
		return err
	}

	return configureCollaborators(have, want, invitees, adder, remover)
}

func configureCollaborators(have, want permissionLevels, invitees sets.String, adder func(user, permission string) error, remover func(user string) error) error {
	have.normalize()
	want.normalize()

	// make sure a user is not mentioned in more than one role
	if err := validatePermissionLevels(have, want); err != nil {
		return err
	}

	havePlusInvites := have.all().Union(invitees)
	remove := havePlusInvites.Difference(want.all())
	admin := want.admin.Difference(have.admin)
	write := want.write.Difference(have.write)
	read := want.read.Difference(have.read)

	var errs []error
	for u := range admin {
		if err := adder(u, "admin"); err != nil {
			errs = append(errs, err)
		}
	}
	for u := range write {
		if err := adder(u, "push"); err != nil {
			errs = append(errs, err)
		}
	}
	for u := range read {
		if err := adder(u, "pull"); err != nil {
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

// validateRepoCollaborators returns an error if any names are used multiples in the config..
func validateRepoCollaborators(config org.Repo) error {
	members := sets.NewString(config.Members.Admin...).Union(sets.NewString(config.Members.Write...)).Union(sets.NewString(config.Members.Read...))
	oc := sets.NewString(config.OutsideCollaborators.Admin...).Union(sets.NewString(config.OutsideCollaborators.Write...)).Union(sets.NewString(config.OutsideCollaborators.Read...))
	teams := sets.NewString(config.Teams.Admin...).Union(sets.NewString(config.Teams.Write...)).Union(sets.NewString(config.Teams.Read...))

	if all := members.Intersection(oc).Intersection(teams); len(all) > 0 {
		return fmt.Errorf("users in all of members, outside_collaborators and teams: %s", strings.Join(all.List(), ", "))
	}
	if both := members.Intersection(oc); len(both) > 0 {
		return fmt.Errorf("users in both members and outside_collaborators: %s", strings.Join(both.List(), ", "))
	}
	if both := members.Intersection(teams); len(both) > 0 {
		return fmt.Errorf("users in both members and teams: %s", strings.Join(both.List(), ", "))
	}
	if both := teams.Intersection(oc); len(both) > 0 {
		return fmt.Errorf("users in both teams and outside_collaborators: %s", strings.Join(both.List(), ", "))
	}
	return nil
}

// validatePermissionLevels returns an error if any names are used multiple times in the config.
func validatePermissionLevels(have, want permissionLevels) error {
	if all := want.admin.Intersection(want.write).Intersection(want.read); len(all) > 0 {
		return fmt.Errorf("users in all of admin, write and read: %s", strings.Join(all.List(), ", "))
	}
	if both := want.admin.Intersection(want.write); len(both) > 0 {
		return fmt.Errorf("users in both admin and write: %s", strings.Join(both.List(), ", "))
	}
	if both := want.admin.Intersection(want.read); len(both) > 0 {
		return fmt.Errorf("users in both admin and read: %s", strings.Join(both.List(), ", "))
	}
	if both := want.write.Intersection(want.read); len(both) > 0 {
		return fmt.Errorf("users in both write and read: %s", strings.Join(both.List(), ", "))
	}
	return nil
}
