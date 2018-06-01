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
			return fmt.Errorf("Invalid --endpoint URL %q: %v.", ep, err)
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

	cfg, err := config.Load(o.config)
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

type configureOrgMembersClient interface {
	BotName() (string, error)
	ListOrgMembers(org, role string) ([]github.TeamMember, error)
	RemoveOrgMembership(org, user string) error
	UpdateOrgMembership(org, user string, admin bool) (*github.OrgMembership, error)
}

func configureOrgMembers(opt options, client configureOrgMembersClient, orgName string, orgConfig org.Config) error {
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

	// Ensure desired state is sane
	both := wantAdmins.Intersection(wantMembers)
	if n := len(both); n > 0 {
		return fmt.Errorf("%d users are both a member and admin: %v", n, both)
	}

	// Figure out who to remove
	have := haveMembers.Union(haveAdmins)
	want := wantMembers.Union(wantAdmins)
	remove := have.Difference(want)

	// Figure out who to invite and/or reconfigure
	makeMember := wantMembers.Difference(haveMembers)
	makeAdmin := wantAdmins.Difference(haveAdmins)

	// Sanity check changes
	if d := float64(len(remove)) / float64(len(have)); d > opt.maximumDelta {
		return fmt.Errorf("cannot delete %d memberships or %.3f of %s (exceeds limit of %.3f)", len(remove), d, orgName, opt.maximumDelta)
	}

	var errs []error

	// March towards desired state
	for u := range remove {
		if err := client.RemoveOrgMembership(orgName, u); err != nil {
			logrus.WithError(err).Warnf("RemoveOrgMembership(%s, %s) failed", orgName, u)
			errs = append(errs, err)
		} else {
			logrus.Infof("Removed %s from %s", u, orgName)
		}
	}

	for u := range makeMember {
		if m, err := client.UpdateOrgMembership(orgName, u, false); err != nil {
			logrus.WithError(err).Warnf("UpdateOrgMembership(%s, %s, false) failed", orgName, u)
			errs = append(errs, err)
		} else if m.State == github.StatePending {
			logrus.Infof("Invited %s to %s", u, orgName)
		} else {
			logrus.Infof("Demoted %s to a %s member", u, orgName)
		}
	}

	for u := range makeAdmin {
		if m, err := client.UpdateOrgMembership(orgName, u, true); err != nil {
			logrus.WithError(err).Warnf("UpdateOrgMembership(%s, %s, true) failed", orgName, u)
			errs = append(errs, err)
		} else if m.State == github.StatePending {
			logrus.Infof("Invited %s to %s", u, orgName)
		} else {
			logrus.Infof("Promoted %s to a %s admin", u, orgName)
		}
	}

	if n := len(errs); n > 0 {
		return fmt.Errorf("%d errors: %v", n, errs)
	}
	return nil
}

func configureOrg(opt options, client *github.Client, orgName string, orgConfig org.Config) error {
	// get meta
	// diff meta
	// patch meta

	if err := configureOrgMembers(opt, client, orgName, orgConfig); err != nil {
		return fmt.Errorf("failed to configure %s members: %v", orgName, err)
	}

	// teams = set(all teams)
	for name, team := range orgConfig.Teams {
		team.Name = name
		num := -1
		// find team name in teams (or previous name)
		if err := configureTeam(client, num, team); err != nil {
			return fmt.Errorf("failed to update %s: %v", orgName, err)
		}
	}
	return nil
}

func configureTeam(client *github.Client, num int, team org.Team) error {
	// create team if num < 0
	// else diff and patch

	// people = set(all org members)
	// remove = people - orgConfig.members - orgConfig.admins
	// update = oc.members - [p for p in people if member]
	// update = oc.admins - [p for p in people if admin]
	return errors.New("implement")
}
