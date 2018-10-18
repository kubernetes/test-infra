/*
Copyright 2017 The Kubernetes Authors.

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

// PR labeler provides a way to add a missing ok-to-test label on trusted PRs.
//
// The --token-path determines who interacts with github.
// By default PR labeler runs in dry mode, add --confirm to make it leave comments.
package main

import (
	"flag"
	"log"
	"net/url"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"
)

const (
	needsOkToTest = "needs-ok-to-test"
	okToTest      = "ok-to-test"
)

type client interface {
	AddLabel(org, repo string, number int, label string) error
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
	GetPullRequests(org, repo string) ([]github.PullRequest, error)
	ListCollaborators(org, repo string) ([]github.User, error)
	ListOrgMembers(org, role string) ([]github.TeamMember, error)
}

type options struct {
	confirm            bool
	endpoint           flagutil.Strings
	org                string
	repo               string
	tokenPath          string
	trustCollaborators bool
}

func flagOptions() options {
	o := options{
		endpoint: flagutil.NewStrings("https://api.github.com"),
	}
	flag.BoolVar(&o.confirm, "confirm", false, "Mutate github if set")
	flag.StringVar(&o.org, "org", "", "github org")
	flag.StringVar(&o.repo, "repo", "", "github repo")
	flag.StringVar(&o.tokenPath, "token-path", "", "Path to github token")
	flag.BoolVar(&o.trustCollaborators, "trust-collaborators", false, "Also trust PRs from collaborators")
	flag.Parse()
	return o
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	o := flagOptions()

	if o.org == "" {
		log.Fatal("empty --org")
	}
	if o.repo == "" {
		log.Fatal("empty --repo")
	}
	if o.tokenPath == "" {
		log.Fatal("empty --token-path")
	}

	secretAgent := &config.SecretAgent{}
	if err := secretAgent.Start([]string{o.tokenPath}); err != nil {
		log.Fatalf("Error starting secrets agent: %v", err)
	}

	var err error
	for _, ep := range o.endpoint.Strings() {
		_, err = url.ParseRequestURI(ep)
		if err != nil {
			log.Fatalf("Invalid --endpoint URL %q: %v.", ep, err)
		}
	}

	var c client
	if o.confirm {
		c = github.NewClient(secretAgent.GetTokenGenerator(o.tokenPath), o.endpoint.Strings()...)
	} else {
		c = github.NewDryRunClient(secretAgent.GetTokenGenerator(o.tokenPath), o.endpoint.Strings()...)
	}

	// get all open PRs
	prs, err := c.GetPullRequests(o.org, o.repo)
	if err != nil {
		log.Fatal(err)
	}

	// get the list of authors to skip once and use a set for lookups
	skipAuthors := sets.NewString()

	// skip org members
	members, err := c.ListOrgMembers(o.org, "all")
	if err != nil {
		log.Fatal(err)
	}
	for _, member := range members {
		skipAuthors.Insert(member.Login)
	}

	// eventually also skip collaborators
	if o.trustCollaborators {
		collaborators, err := c.ListCollaborators(o.org, o.repo)
		if err != nil {
			log.Fatal(err)
		}
		for _, collaborator := range collaborators {
			skipAuthors.Insert(collaborator.Login)
		}
	}

	for _, pr := range prs {
		// skip PRs from these authors
		if skipAuthors.Has(pr.User.Login) {
			continue
		}
		// skip PRs with *ok-to-test labels
		labels, err := c.GetIssueLabels(o.org, o.repo, pr.Number)
		if err != nil {
			log.Fatal(err)
		}
		if github.HasLabel(okToTest, labels) || github.HasLabel(needsOkToTest, labels) {
			continue
		}
		// only add ok-to-test with --confirm
		if !o.confirm {
			log.Println("Use --confirm to add", okToTest, "to", pr.HTMLURL)
			continue
		}
		if err := c.AddLabel(o.org, o.repo, pr.Number, okToTest); err != nil {
			log.Fatal(err)
		}
	}
}
