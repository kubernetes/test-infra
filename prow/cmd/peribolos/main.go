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
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/yaml"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"

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
	config            string
	confirm           bool
	dump              string
	dumpFull          bool
	maximumDelta      float64
	minAdmins         int
	requireSelf       bool
	requiredAdmins    flagutil.Strings
	fixOrg            bool
	fixOrgMembers     bool
	fixTeamMembers    bool
	fixTeams          bool
	fixTeamRepos      bool
	fixRepos          bool
	ignoreSecretTeams bool
	allowRepoArchival bool
	allowRepoPublish  bool
	github            flagutil.GitHubOptions
	tokenBurst        int
	tokensPerHour     int
	logLevel          string
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
	flags.StringVar(&o.config, "config-path", "", "Path to org config.yaml")
	flags.BoolVar(&o.confirm, "confirm", false, "Mutate github if set")
	flags.IntVar(&o.tokensPerHour, "tokens", defaultTokens, "Throttle hourly token consumption (0 to disable)")
	flags.IntVar(&o.tokenBurst, "token-burst", defaultBurst, "Allow consuming a subset of hourly tokens in a short burst")
	flags.StringVar(&o.dump, "dump", "", "Output current config of this org if set")
	flags.BoolVar(&o.dumpFull, "dump-full", false, "Output current config of the org as a valid input config file instead of a snippet")
	flags.BoolVar(&o.ignoreSecretTeams, "ignore-secret-teams", false, "Do not dump or update secret teams if set")
	flags.BoolVar(&o.fixOrg, "fix-org", false, "Change org metadata if set")
	flags.BoolVar(&o.fixOrgMembers, "fix-org-members", false, "Add/remove org members if set")
	flags.BoolVar(&o.fixTeams, "fix-teams", false, "Create/delete/update teams if set")
	flags.BoolVar(&o.fixTeamMembers, "fix-team-members", false, "Add/remove team members if set")
	flags.BoolVar(&o.fixTeamRepos, "fix-team-repos", false, "Add/remove team permissions on repos if set")
	flags.BoolVar(&o.fixRepos, "fix-repos", false, "Create/update repositories if set")
	flags.BoolVar(&o.allowRepoArchival, "allow-repo-archival", false, "If set, archiving repos is allowed while updating repos")
	flags.BoolVar(&o.allowRepoPublish, "allow-repo-publish", false, "If set, making private repos public is allowed while updating repos")
	flags.StringVar(&o.logLevel, "log-level", logrus.InfoLevel.String(), fmt.Sprintf("Logging level, one of %v", logrus.AllLevels))
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

	if o.dumpFull && o.dump == "" {
		return errors.New("--dump-full can't be used without --dump")
	}

	if o.fixTeamMembers && !o.fixTeams {
		return fmt.Errorf("--fix-team-members requires --fix-teams")
	}

	if o.fixTeamRepos && !o.fixTeams {
		return fmt.Errorf("--fix-team-repos requires --fix-teams")
	}

	level, err := logrus.ParseLevel(o.logLevel)
	if err != nil {
		return fmt.Errorf("--log-level invalid: %v", err)
	}
	logrus.SetLevel(level)

	return nil
}

func main() {
	logrusutil.ComponentInit()

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
		ret, err := dumpOrgConfig(githubClient, o.dump, o.ignoreSecretTeams)
		if err != nil {
			logrus.WithError(err).Fatalf("Dump %s failed to collect current data.", o.dump)
		}
		var output interface{}
		if o.dumpFull {
			output = org.FullConfig{
				Orgs: map[string]org.Config{o.dump: *ret},
			}
		} else {
			output = ret
		}
		out, err := yaml.Marshal(output)
		if err != nil {
			logrus.WithError(err).Fatalf("Dump %s failed to marshal output.", o.dump)
		}
		logrus.Infof("Dumping orgs[\"%s\"]:", o.dump)
		fmt.Println(string(out))
		return
	}

	raw, err := ioutil.ReadFile(o.config)
	if err != nil {
		logrus.WithError(err).Fatal("Could not read --config-path file")
	}

	var cfg org.FullConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		logrus.WithError(err).Fatal("Failed to load configuration")
	}

	for name, orgcfg := range cfg.Orgs {
		if err := configureOrg(o, githubClient, name, orgcfg); err != nil {
			logrus.Fatalf("Configuration failed: %v", err)
		}
	}
	logrus.Info("Finished syncing configuration.")
}

type dumpClient interface {
	GetOrg(name string) (*github.Organization, error)
	ListOrgMembers(org, role string) ([]github.TeamMember, error)
	ListTeams(org string) ([]github.Team, error)
	ListTeamMembers(org string, id int, role string) ([]github.TeamMember, error)
	ListTeamRepos(org string, id int) ([]github.Repo, error)
	GetRepo(owner, name string) (github.FullRepo, error)
	GetRepos(org string, isUser bool) ([]github.Repo, error)
	BotUser() (*github.UserData, error)
}

func dumpOrgConfig(client dumpClient, orgName string, ignoreSecretTeams bool) (*org.Config, error) {
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
	drp := github.RepoPermissionLevel(meta.DefaultRepositoryPermission)
	out.Metadata.DefaultRepositoryPermission = &drp
	out.Metadata.MembersCanCreateRepositories = &meta.MembersCanCreateRepositories

	var runningAsAdmin bool
	runningAs, err := client.BotUser()
	if err != nil {
		return nil, fmt.Errorf("failed to obtain username for this token")
	}
	admins, err := client.ListOrgMembers(orgName, github.RoleAdmin)
	if err != nil {
		return nil, fmt.Errorf("failed to list org admins: %v", err)
	}
	logrus.Debugf("Found %d admins", len(admins))
	for _, m := range admins {
		logrus.WithField("login", m.Login).Debug("Recording admin.")
		out.Admins = append(out.Admins, m.Login)
		if runningAs.Login == m.Login {
			runningAsAdmin = true
		}
	}

	if !runningAsAdmin {
		return nil, fmt.Errorf("--dump must be run with admin:org scope token")
	}

	orgMembers, err := client.ListOrgMembers(orgName, github.RoleMember)
	if err != nil {
		return nil, fmt.Errorf("failed to list org members: %v", err)
	}
	logrus.Debugf("Found %d members", len(orgMembers))
	for _, m := range orgMembers {
		logrus.WithField("login", m.Login).Debug("Recording member.")
		out.Members = append(out.Members, m.Login)
	}

	teams, err := client.ListTeams(orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to list teams: %v", err)
	}
	logrus.Debugf("Found %d teams", len(teams))

	names := map[int]string{}   // what's the name of a team?
	idMap := map[int]org.Team{} // metadata for a team
	children := map[int][]int{} // what children does it have
	var tops []int              // what are the top-level teams

	for _, t := range teams {
		logger := logrus.WithFields(logrus.Fields{"id": t.ID, "name": t.Name})
		p := org.Privacy(t.Privacy)
		if ignoreSecretTeams && p == org.Secret {
			logger.Debug("Ignoring secret team.")
			continue
		}
		d := t.Description
		nt := org.Team{
			TeamMetadata: org.TeamMetadata{
				Description: &d,
				Privacy:     &p,
			},
			Maintainers: []string{},
			Members:     []string{},
			Children:    map[string]org.Team{},
			Repos:       map[string]github.RepoPermissionLevel{},
		}
		maintainers, err := client.ListTeamMembers(orgName, t.ID, github.RoleMaintainer)
		if err != nil {
			return nil, fmt.Errorf("failed to list team %d(%s) maintainers: %v", t.ID, t.Name, err)
		}
		logger.Debugf("Found %d maintainers.", len(maintainers))
		for _, m := range maintainers {
			logger.WithField("login", m.Login).Debug("Recording maintainer.")
			nt.Maintainers = append(nt.Maintainers, m.Login)
		}
		teamMembers, err := client.ListTeamMembers(orgName, t.ID, github.RoleMember)
		if err != nil {
			return nil, fmt.Errorf("failed to list team %d(%s) members: %v", t.ID, t.Name, err)
		}
		logger.Debugf("Found %d members.", len(teamMembers))
		for _, m := range teamMembers {
			logger.WithField("login", m.Login).Debug("Recording member.")
			nt.Members = append(nt.Members, m.Login)
		}

		names[t.ID] = t.Name
		idMap[t.ID] = nt

		if t.Parent == nil { // top level team
			logger.Debug("Marking as top-level team.")
			tops = append(tops, t.ID)
		} else { // add this id to the list of the parent's children
			logger.Debugf("Marking as child team of %d.", t.Parent.ID)
			children[t.Parent.ID] = append(children[t.Parent.ID], t.ID)
		}

		repos, err := client.ListTeamRepos(orgName, t.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to list team %d(%s) repos: %v", t.ID, t.Name, err)
		}
		logger.Debugf("Found %d repo permissions.", len(repos))
		for _, repo := range repos {
			level := github.LevelFromPermissions(repo.Permissions)
			logger.WithFields(logrus.Fields{"repo": repo, "permission": level}).Debug("Recording repo permission.")
			nt.Repos[repo.Name] = level
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

	out.Teams = make(map[string]org.Team, len(tops))
	for _, id := range tops {
		out.Teams[names[id]] = makeChild(id)
	}

	repos, err := client.GetRepos(orgName, false)
	if err != nil {
		return nil, fmt.Errorf("failed to list org repos: %v", err)
	}
	logrus.Debugf("Found %d repos", len(repos))
	out.Repos = make(map[string]org.Repo, len(repos))
	for _, repo := range repos {
		full, err := client.GetRepo(orgName, repo.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to get repo: %v", err)
		}
		logrus.WithField("repo", full.FullName).Debug("Recording repo.")
		out.Repos[full.Name] = org.PruneRepoDefaults(org.Repo{
			Description:      &full.Description,
			HomePage:         &full.Homepage,
			Private:          &full.Private,
			HasIssues:        &full.HasIssues,
			HasProjects:      &full.HasProjects,
			HasWiki:          &full.HasWiki,
			AllowMergeCommit: &full.AllowMergeCommit,
			AllowSquashMerge: &full.AllowSquashMerge,
			AllowRebaseMerge: &full.AllowRebaseMerge,
			Archived:         &full.Archived,
			DefaultBranch:    &full.DefaultBranch,
		})
	}

	return &out, nil
}

type orgClient interface {
	BotUser() (*github.UserData, error)
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
		if me, err := client.BotUser(); err != nil {
			return fmt.Errorf("cannot determine user making requests for %s: %v", opt.github.TokenPath, err)
		} else if !wantAdmins.Has(me.Login) {
			return fmt.Errorf("authenticated user %s is not an admin of %s", me.Login, orgName)
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
			if github.IsNotFound(err) {
				// this could be caused by someone removing their account
				// or a typo in the configuration but should not crash the sync
				err = nil
			}
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

	return utilerrors.NewAggregate(errs)
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
	DeleteTeam(org string, id int) error
}

// configureTeams returns the ids for all expected team names, creating/deleting teams as necessary.
func configureTeams(client teamClient, orgName string, orgConfig org.Config, maxDelta float64, ignoreSecretTeams bool) (map[string]github.Team, error) {
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
	logrus.Debugf("Found %d teams", len(teamList))
	for _, t := range teamList {
		if ignoreSecretTeams && org.Privacy(t.Privacy) == org.Secret {
			continue
		}
		ids[t.ID] = t
		ints.Insert(t.ID)
	}
	if ignoreSecretTeams {
		logrus.Debugf("Found %d non-secret teams", len(teamList))
	}

	// What is the lowest ID for each team?
	older := map[string][]github.Team{}
	names := map[string]github.Team{}
	for _, t := range ids {
		logger := logrus.WithFields(logrus.Fields{"id": t.ID, "name": t.Name})
		n := t.Name
		switch val, ok := names[n]; {
		case !ok: // first occurrence of the name
			logger.Debug("First occurrence of this team name.")
			names[n] = t
		case ok && t.ID < val.ID: // t has the lower ID, replace and send current to older set
			logger.Debugf("Replacing previous recorded team (%d) with this one due to smaller ID.", val.ID)
			names[n] = t
			older[n] = append(older[n], val)
		default: // t does not have smallest id, add it to older set
			logger.Debugf("Adding team (%d) to older set as a smaller ID is already recoded for it.", val.ID)
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
			logger := logrus.WithField("name", name)
			match(orgTeam.Children)
			t := findTeam(names, name, orgTeam.Previously...)
			if t == nil {
				missing[name] = orgTeam
				logger.Debug("Could not find team in GitHub for this configuration.")
				continue
			}
			matches[name] = *t // t.Name != name if we matched on orgTeam.Previously
			logger.WithField("id", t.ID).Debug("Found a team in GitHub for this configuration.")
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
		if err := client.DeleteTeam(orgName, id); err != nil {
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
		return false // already have it
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
		change = updateString(&cur.DefaultRepositoryPermission, &w) || change
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

func configureOrg(opt options, client github.Client, orgName string, orgConfig org.Config) error {
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

	// Create repositories in the org
	if !opt.fixRepos {
		logrus.Info("Skipping org repositories configuration")
	} else if err := configureRepos(opt, client, orgName, orgConfig); err != nil {
		return fmt.Errorf("failed to configure %s repos: %v", orgName, err)
	}

	if !opt.fixTeams {
		logrus.Infof("Skipping team and team member configuration")
		return nil
	}

	// Find the id and current state of each declared team (create/delete as necessary)
	githubTeams, err := configureTeams(client, orgName, orgConfig, opt.maximumDelta, opt.ignoreSecretTeams)
	if err != nil {
		return fmt.Errorf("failed to configure %s teams: %v", orgName, err)
	}

	for name, team := range orgConfig.Teams {
		err := configureTeamAndMembers(opt, client, githubTeams, name, orgName, team, nil)
		if err != nil {
			return fmt.Errorf("failed to configure %s teams: %v", orgName, err)
		}

		if !opt.fixTeamRepos {
			logrus.Infof("Skipping team repo permissions configuration")
			continue
		}
		if err := configureTeamRepos(client, githubTeams, name, orgName, team); err != nil {
			return fmt.Errorf("failed to configure %s team %s repos: %v", orgName, name, err)
		}
	}
	return nil
}

type repoClient interface {
	GetRepo(orgName, repo string) (github.FullRepo, error)
	GetRepos(orgName string, isUser bool) ([]github.Repo, error)
	CreateRepo(owner string, isUser bool, repo github.RepoCreateRequest) (*github.FullRepo, error)
	UpdateRepo(owner, name string, repo github.RepoUpdateRequest) (*github.FullRepo, error)
}

func newRepoCreateRequest(name string, definition org.Repo) github.RepoCreateRequest {
	repoCreate := github.RepoCreateRequest{
		RepoRequest: github.RepoRequest{
			Name:             &name,
			Description:      definition.Description,
			Homepage:         definition.HomePage,
			Private:          definition.Private,
			HasIssues:        definition.HasIssues,
			HasProjects:      definition.HasProjects,
			HasWiki:          definition.HasWiki,
			AllowSquashMerge: definition.AllowSquashMerge,
			AllowMergeCommit: definition.AllowMergeCommit,
			AllowRebaseMerge: definition.AllowRebaseMerge,
		},
	}

	if definition.OnCreate != nil {
		repoCreate.AutoInit = definition.OnCreate.AutoInit
		repoCreate.GitignoreTemplate = definition.OnCreate.GitignoreTemplate
		repoCreate.LicenseTemplate = definition.OnCreate.LicenseTemplate
	}

	return repoCreate
}

func validateRepos(repos map[string]org.Repo) error {
	seen := map[string]string{}
	var dups []string

	for wantName, repo := range repos {
		toCheck := append([]string{wantName}, repo.Previously...)
		for _, name := range toCheck {
			normName := strings.ToLower(name)
			if seenName, have := seen[normName]; have {
				dups = append(dups, fmt.Sprintf("%s/%s", seenName, name))
			}
		}
		for _, name := range toCheck {
			normName := strings.ToLower(name)
			seen[normName] = name
		}

	}

	if len(dups) > 0 {
		return fmt.Errorf("found duplicate repo names (GitHub repo names are case-insensitive): %s", strings.Join(dups, ", "))
	}

	return nil
}

// newRepoUpdateRequest creates a minimal github.RepoUpdateRequest instance
// needed to update the current repo into the target state.
func newRepoUpdateRequest(current github.FullRepo, name string, repo org.Repo) github.RepoUpdateRequest {
	setString := func(current string, want *string) *string {
		if want != nil && *want != current {
			return want
		}
		return nil
	}
	setBool := func(current bool, want *bool) *bool {
		if want != nil && *want != current {
			return want
		}
		return nil
	}
	repoUpdate := github.RepoUpdateRequest{
		RepoRequest: github.RepoRequest{
			Name:             setString(current.Name, &name),
			Description:      setString(current.Description, repo.Description),
			Homepage:         setString(current.Homepage, repo.HomePage),
			Private:          setBool(current.Private, repo.Private),
			HasIssues:        setBool(current.HasIssues, repo.HasIssues),
			HasProjects:      setBool(current.HasProjects, repo.HasProjects),
			HasWiki:          setBool(current.HasWiki, repo.HasWiki),
			AllowSquashMerge: setBool(current.AllowSquashMerge, repo.AllowSquashMerge),
			AllowMergeCommit: setBool(current.AllowMergeCommit, repo.AllowMergeCommit),
			AllowRebaseMerge: setBool(current.AllowRebaseMerge, repo.AllowRebaseMerge),
		},
		DefaultBranch: setString(current.DefaultBranch, repo.DefaultBranch),
		Archived:      setBool(current.Archived, repo.Archived),
	}

	return repoUpdate

}

func sanitizeRepoDelta(opt options, delta *github.RepoUpdateRequest) []error {
	var errs []error
	if delta.Archived != nil && !*delta.Archived {
		delta.Archived = nil
		errs = append(errs, fmt.Errorf("asked to unarchive an archived repo, unsupported by GH API"))
	}
	if delta.Archived != nil && *delta.Archived && !opt.allowRepoArchival {
		delta.Archived = nil
		errs = append(errs, fmt.Errorf("asked to archive a repo but this is not allowed by default (see --allow-repo-archival)"))
	}
	if delta.Private != nil && !(*delta.Private || opt.allowRepoPublish) {
		delta.Private = nil
		errs = append(errs, fmt.Errorf("asked to publish a private repo but this is not allowed by default (see --allow-repo-publish)"))
	}

	return errs
}

func configureRepos(opt options, client repoClient, orgName string, orgConfig org.Config) error {
	if err := validateRepos(orgConfig.Repos); err != nil {
		return err
	}

	repoList, err := client.GetRepos(orgName, false)
	if err != nil {
		return fmt.Errorf("failed to get repos: %v", err)
	}
	logrus.Debugf("Found %d repositories", len(repoList))
	byName := make(map[string]github.Repo, len(repoList))
	for _, repo := range repoList {
		byName[strings.ToLower(repo.Name)] = repo
	}

	var allErrors []error

	for wantName, wantRepo := range orgConfig.Repos {
		repoLogger := logrus.WithField("repo", wantName)
		pastErrors := len(allErrors)
		var existing *github.FullRepo = nil
		for _, possibleName := range append([]string{wantName}, wantRepo.Previously...) {
			if repo, exists := byName[strings.ToLower(possibleName)]; exists {
				switch {
				case existing == nil:
					if full, err := client.GetRepo(orgName, repo.Name); err != nil {
						repoLogger.WithError(err).Error("failed to get repository data")
						allErrors = append(allErrors, err)
					} else {
						existing = &full
					}
				case existing.Name != repo.Name:
					err := fmt.Errorf("different repos already exist for current and previous names: %s and %s", existing.Name, repo.Name)
					allErrors = append(allErrors, err)
				}
			}
		}

		if len(allErrors) > pastErrors {
			continue
		}

		if existing == nil {
			if wantRepo.Archived != nil && *wantRepo.Archived {
				repoLogger.Error("repo does not exist but is configured as archived: not creating")
				allErrors = append(allErrors, fmt.Errorf("nonexistent repo configured as archived: %s", wantName))
				continue
			}
			repoLogger.Info("repo does not exist, creating")
			created, err := client.CreateRepo(orgName, false, newRepoCreateRequest(wantName, wantRepo))
			if err != nil {
				repoLogger.WithError(err).Error("failed to create repository")
				allErrors = append(allErrors, err)
			} else {
				existing = created
			}
		}

		if existing != nil {
			if existing.Archived {
				if wantRepo.Archived != nil && *wantRepo.Archived {
					repoLogger.Infof("repo %q is archived, skipping changes", wantName)
					continue
				}
			}
			repoLogger.Info("repo exists, considering an update")
			delta := newRepoUpdateRequest(*existing, wantName, wantRepo)
			if deltaErrors := sanitizeRepoDelta(opt, &delta); len(deltaErrors) > 0 {
				for _, err := range deltaErrors {
					repoLogger.WithError(err).Error("requested repo change is not allowed, removing from delta")
				}
				allErrors = append(allErrors, deltaErrors...)
			}
			if delta.Defined() {
				repoLogger.Info("repo exists and differs from desired state, updating")
				if _, err := client.UpdateRepo(orgName, existing.Name, delta); err != nil {
					repoLogger.WithError(err).Error("failed to update repository")
					allErrors = append(allErrors, err)
				}
			}
		}
	}

	return utilerrors.NewAggregate(allErrors)
}

func configureTeamAndMembers(opt options, client github.Client, githubTeams map[string]github.Team, name, orgName string, team org.Team, parent *int) error {
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
	if !opt.fixTeamMembers {
		logrus.Infof("Skipping %s member configuration", name)
	} else if err = configureTeamMembers(client, orgName, gt, team); err != nil {
		return fmt.Errorf("failed to update %s members: %v", name, err)
	}

	for childName, childTeam := range team.Children {
		err = configureTeamAndMembers(opt, client, githubTeams, childName, orgName, childTeam, &gt.ID)
		if err != nil {
			return fmt.Errorf("failed to update %s child teams: %v", name, err)
		}
	}

	return nil
}

type editTeamClient interface {
	EditTeam(org string, team github.Team) (*github.Team, error)
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
		if _, err := client.EditTeam(orgName, gt); err != nil {
			return fmt.Errorf("failed to edit %s team %d(%s): %v", orgName, gt.ID, gt.Name, err)
		}
	}
	return nil
}

type teamRepoClient interface {
	ListTeamRepos(org string, id int) ([]github.Repo, error)
	UpdateTeamRepo(id int, org, repo string, permission github.TeamPermission) error
	RemoveTeamRepo(id int, org, repo string) error
}

// configureTeamRepos updates the list of repos that the team has permissions for when necessary
func configureTeamRepos(client teamRepoClient, githubTeams map[string]github.Team, name, orgName string, team org.Team) error {
	gt, ok := githubTeams[name]
	if !ok { // configureTeams is buggy if this is the case
		return fmt.Errorf("%s not found in id list", name)
	}

	want := team.Repos
	have := map[string]github.RepoPermissionLevel{}
	repos, err := client.ListTeamRepos(orgName, gt.ID)
	if err != nil {
		return fmt.Errorf("failed to list team %d(%s) repos: %v", gt.ID, name, err)
	}
	for _, repo := range repos {
		have[repo.Name] = github.LevelFromPermissions(repo.Permissions)
	}

	actions := map[string]github.RepoPermissionLevel{}
	for wantRepo, wantPermission := range want {
		if havePermission, haveRepo := have[wantRepo]; haveRepo && havePermission == wantPermission {
			// nothing to do
			continue
		}
		// create or update this permission
		actions[wantRepo] = wantPermission
	}

	for haveRepo := range have {
		if _, wantRepo := want[haveRepo]; !wantRepo {
			// should remove these permissions
			actions[haveRepo] = github.None
		}
	}

	var updateErrors []error
	for repo, permission := range actions {
		var err error
		switch permission {
		case github.None:
			err = client.RemoveTeamRepo(gt.ID, orgName, repo)
		case github.Admin:
			err = client.UpdateTeamRepo(gt.ID, orgName, repo, github.RepoAdmin)
		case github.Write:
			err = client.UpdateTeamRepo(gt.ID, orgName, repo, github.RepoPush)
		case github.Read:
			err = client.UpdateTeamRepo(gt.ID, orgName, repo, github.RepoPull)
		}

		if err != nil {
			updateErrors = append(updateErrors, fmt.Errorf("failed to update team %d(%s) permissions on repo %s to %s: %v", gt.ID, name, repo, permission, err))
		}
	}

	for childName, childTeam := range team.Children {
		if err := configureTeamRepos(client, githubTeams, childName, orgName, childTeam); err != nil {
			updateErrors = append(updateErrors, fmt.Errorf("failed to configure %s child team %s repos: %v", orgName, childName, err))
		}
	}

	return utilerrors.NewAggregate(updateErrors)
}

// teamMembersClient can list/remove/update people to a team.
type teamMembersClient interface {
	ListTeamMembers(org string, id int, role string) ([]github.TeamMember, error)
	ListTeamInvitations(org string, id int) ([]github.OrgInvitation, error)
	RemoveTeamMembership(org string, id int, user string) error
	UpdateTeamMembership(org string, id int, user string, maintainer bool) (*github.TeamMembership, error)
}

func teamInvitations(client teamMembersClient, orgName string, teamID int) (sets.String, error) {
	invitees := sets.String{}
	is, err := client.ListTeamInvitations(orgName, teamID)
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

// configureTeamMembers will add/update people to the appropriate role on the team, and remove anyone else.
func configureTeamMembers(client teamMembersClient, orgName string, gt github.Team, team org.Team) error {
	// Get desired state
	wantMaintainers := sets.NewString(team.Maintainers...)
	wantMembers := sets.NewString(team.Members...)

	// Get current state
	haveMaintainers := sets.String{}
	haveMembers := sets.String{}

	members, err := client.ListTeamMembers(orgName, gt.ID, github.RoleMember)
	if err != nil {
		return fmt.Errorf("failed to list %d(%s) members: %v", gt.ID, gt.Name, err)
	}
	for _, m := range members {
		haveMembers.Insert(m.Login)
	}

	maintainers, err := client.ListTeamMembers(orgName, gt.ID, github.RoleMaintainer)
	if err != nil {
		return fmt.Errorf("failed to list %d(%s) maintainers: %v", gt.ID, gt.Name, err)
	}
	for _, m := range maintainers {
		haveMaintainers.Insert(m.Login)
	}

	invitees, err := teamInvitations(client, orgName, gt.ID)
	if err != nil {
		return fmt.Errorf("failed to list %d(%s) invitees: %v", gt.ID, gt.Name, err)
	}

	adder := func(user string, super bool) error {
		if invitees.Has(user) {
			logrus.Infof("Waiting for %s to accept invitation to %d(%s)", user, gt.ID, gt.Name)
			return nil
		}
		role := github.RoleMember
		if super {
			role = github.RoleMaintainer
		}
		tm, err := client.UpdateTeamMembership(orgName, gt.ID, user, super)
		if err != nil {
			// Augment the error with the operation we attempted so that the error makes sense after return
			err = fmt.Errorf("UpdateTeamMembership(%d(%s), %s, %t) failed: %v", gt.ID, gt.Name, user, super, err)
			logrus.Warnf(err.Error())
		} else if tm.State == github.StatePending {
			logrus.Infof("Invited %s to %d(%s) as a %s", user, gt.ID, gt.Name, role)
		} else {
			logrus.Infof("Set %s as a %s of %d(%s)", user, role, gt.ID, gt.Name)
		}
		return err
	}

	remover := func(user string) error {
		err := client.RemoveTeamMembership(orgName, gt.ID, user)
		if err != nil {
			// Augment the error with the operation we attempted so that the error makes sense after return
			err = fmt.Errorf("RemoveTeamMembership(%d(%s), %s) failed: %v", gt.ID, gt.Name, user, err)
			logrus.Warnf(err.Error())
		} else {
			logrus.Infof("Removed %s from team %d(%s)", user, gt.ID, gt.Name)
		}
		return err
	}

	want := memberships{members: wantMembers, super: wantMaintainers}
	have := memberships{members: haveMembers, super: haveMaintainers}
	return configureMembers(have, want, invitees, adder, remover)
}
