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
	"log"
	"net/url"
	"os"
	"strings"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/config/org"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"
)

const defaultEndpoint = "https://api.github.com"

type options struct {
	config   string
	token    string
	confirm  bool
	endpoint flagutil.Strings
}

func parseOptions() options {
	var o options
	if err := o.parseArgs(flag.CommandLine, os.Args[1:]); err != nil {
		log.Fatalf("Invalid flags: %v", err)
	}
	return o
}

func (o *options) parseArgs(flags *flag.FlagSet, args []string) error {
	o.endpoint = flagutil.NewStrings(defaultEndpoint)
	flags.StringVar(&o.config, "config-path", "", "Path to prow config.yaml")
	flags.BoolVar(&o.confirm, "confirm", false, "Mutate github if set")
	flags.Var(&o.endpoint, "github-endpoint", "Github api endpoint, may differ for enterprise")
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

	return nil
}

func main() {
	o := parseOptions()

	if o.config != "TODO(fejta): implement me" {
		log.Fatalf("This program is not yet implemented")
	}

	cfg, err := config.Load(o.config, "")
	if err != nil {
		log.Fatalf("Failed to load --config=%s: %v", o.config, err)
	}

	b, err := ioutil.ReadFile(o.token)
	if err != nil {
		log.Fatalf("cannot read --token: %v", err)
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
		if err := configureOrg(c, name, orgcfg); err != nil {
			log.Fatalf("Configuration failed: %v", err)
		}
	}
}

func configureOrg(client *github.Client, orgName string, orgConfig org.Config) error {
	// get meta
	// diff meta
	// patch meta

	// people = set(all org members)
	// remove = people - orgConfig.members - orgConfig.admins
	// update = oc.members - [p for p in people if member]
	// update = oc.admins - [p for p in people if admin]

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
