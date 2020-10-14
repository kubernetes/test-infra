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

// Commenter provides a way to --query for issues and append a --comment to matches.
//
// The --token determines who interacts with github.
// By default commenter runs in dry mode, add --confirm to make it leave comments.
// The --updated, --include-closed, --ceiling options provide minor safeguards
// around leaving excessive comments.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"k8s.io/test-infra/prow/config/secret"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"
)

const (
	templateHelp = `--comment is a golang text/template if set.
	Valid placeholders:
		.Org - github org
		.Repo - github repo
		.Number - issue number
	Advanced (see kubernetes/test-infra/prow/github/types.go):
		.Issue.User.Login - github account
		.Issue.Title
		.Issue.State
		.Issue.HTMLURL
		.Issue.Assignees - list of assigned .Users
		.Issue.Labels - list of applied labels (.Name)
`
)

func flagOptions() options {
	o := options{
		endpoint: flagutil.NewStrings(github.DefaultAPIEndpoint),
	}
	flag.StringVar(&o.query, "query", "", "See https://help.github.com/articles/searching-issues-and-pull-requests/")
	flag.DurationVar(&o.updated, "updated", 2*time.Hour, "Filter to issues unmodified for at least this long if set")
	flag.BoolVar(&o.includeArchived, "include-archived", false, "Match archived issues if set")
	flag.BoolVar(&o.includeClosed, "include-closed", false, "Match closed issues if set")
	flag.BoolVar(&o.confirm, "confirm", false, "Mutate github if set")
	flag.StringVar(&o.comment, "comment", "", "Append the following comment to matching issues")
	flag.BoolVar(&o.useTemplate, "template", false, templateHelp)
	flag.IntVar(&o.ceiling, "ceiling", 3, "Maximum number of issues to modify, 0 for infinite")
	flag.Var(&o.endpoint, "endpoint", "GitHub's API endpoint")
	flag.StringVar(&o.graphqlEndpoint, "graphql-endpoint", github.DefaultGraphQLEndpoint, "GitHub's GraphQL API Endpoint")
	flag.StringVar(&o.token, "token", "", "Path to github token")
	flag.BoolVar(&o.random, "random", false, "Choose random issues to comment on from the query")
	flag.Parse()
	return o
}

type meta struct {
	Number int
	Org    string
	Repo   string
	Issue  github.Issue
}

type options struct {
	asc             bool
	ceiling         int
	comment         string
	includeArchived bool
	includeClosed   bool
	useTemplate     bool
	query           string
	sort            string
	endpoint        flagutil.Strings
	graphqlEndpoint string
	token           string
	updated         time.Duration
	confirm         bool
	random          bool
}

func parseHTMLURL(url string) (string, string, int, error) {
	// Example: https://github.com/batterseapower/pinyin-toolkit/issues/132
	re := regexp.MustCompile(`.+/(.+)/(.+)/(issues|pull)/(\d+)$`)
	mat := re.FindStringSubmatch(url)
	if mat == nil {
		return "", "", 0, fmt.Errorf("failed to parse: %s", url)
	}
	n, err := strconv.Atoi(mat[4])
	if err != nil {
		return "", "", 0, err
	}
	return mat[1], mat[2], n, nil
}

func makeQuery(query string, includeArchived, includeClosed bool, minUpdated time.Duration) (string, error) {
	parts := []string{query}
	if !includeArchived {
		if strings.Contains(query, "archived:true") {
			return "", errors.New("archived:true requires --include-archived")
		}
		parts = append(parts, "archived:false")
	} else if strings.Contains(query, "archived:false") {
		return "", errors.New("archived:false conflicts with --include-archived")
	}
	if !includeClosed {
		if strings.Contains(query, "is:closed") {
			return "", errors.New("is:closed requires --include-closed")
		}
		parts = append(parts, "is:open")
	} else if strings.Contains(query, "is:open") {
		return "", errors.New("is:open conflicts with --include-closed")
	}
	if minUpdated != 0 {
		latest := time.Now().Add(-minUpdated)
		parts = append(parts, "updated:<="+latest.Format(time.RFC3339))
	}
	return strings.Join(parts, " "), nil
}

type client interface {
	CreateComment(owner, repo string, number int, comment string) error
	FindIssues(query, sort string, asc bool) ([]github.Issue, error)
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	o := flagOptions()

	if o.query == "" {
		log.Fatal("empty --query")
	}
	if o.token == "" {
		log.Fatal("empty --token")
	}
	if o.comment == "" {
		log.Fatal("empty --comment")
	}

	secretAgent := &secret.Agent{}
	if err := secretAgent.Start([]string{o.token}); err != nil {
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
		c = github.NewClient(secretAgent.GetTokenGenerator(o.token), secretAgent.Censor, o.graphqlEndpoint, o.endpoint.Strings()...)
	} else {
		c = github.NewDryRunClient(secretAgent.GetTokenGenerator(o.token), secretAgent.Censor, o.graphqlEndpoint, o.endpoint.Strings()...)
	}

	query, err := makeQuery(o.query, o.includeArchived, o.includeClosed, o.updated)
	if err != nil {
		log.Fatalf("Bad query %q: %v", o.query, err)
	}
	sort := ""
	asc := false
	if o.updated > 0 {
		sort = "updated"
		asc = true
	}
	commenter := makeCommenter(o.comment, o.useTemplate)
	if err := run(c, query, sort, asc, o.random, commenter, o.ceiling); err != nil {
		log.Fatalf("Failed run: %v", err)
	}
}

func makeCommenter(comment string, useTemplate bool) func(meta) (string, error) {
	if !useTemplate {
		return func(_ meta) (string, error) {
			return comment, nil
		}
	}
	t := template.Must(template.New("comment").Parse(comment))
	return func(m meta) (string, error) {
		out := bytes.Buffer{}
		err := t.Execute(&out, m)
		return out.String(), err
	}
}

func run(c client, query, sort string, asc, random bool, commenter func(meta) (string, error), ceiling int) error {
	log.Printf("Searching: %s", query)
	issues, err := c.FindIssues(query, sort, asc)
	if err != nil {
		return fmt.Errorf("search failed: %v", err)
	}
	problems := []string{}
	log.Printf("Found %d matches", len(issues))
	if random {
		dest := make([]github.Issue, len(issues))
		perm := rand.Perm(len(issues))
		for i, v := range perm {
			dest[v] = issues[i]
		}
		issues = dest
	}
	for n, i := range issues {
		if ceiling > 0 && n == ceiling {
			log.Printf("Stopping at --ceiling=%d of %d results", n, len(issues))
			break
		}
		log.Printf("Matched %s (%s)", i.HTMLURL, i.Title)
		org, repo, number, err := parseHTMLURL(i.HTMLURL)
		if err != nil {
			msg := fmt.Sprintf("Failed to parse %s: %v", i.HTMLURL, err)
			log.Print(msg)
			problems = append(problems, msg)
		}
		comment, err := commenter(meta{Number: number, Org: org, Repo: repo, Issue: i})
		if err != nil {
			msg := fmt.Sprintf("Failed to create comment for %s/%s#%d: %v", org, repo, number, err)
			log.Print(msg)
			problems = append(problems, msg)
			continue
		}
		if err := c.CreateComment(org, repo, number, comment); err != nil {
			msg := fmt.Sprintf("Failed to apply comment to %s/%s#%d: %v", org, repo, number, err)
			log.Print(msg)
			problems = append(problems, msg)
			continue
		}
		log.Printf("Commented on %s", i.HTMLURL)
	}
	if len(problems) > 0 {
		return fmt.Errorf("encoutered %d failures: %v", len(problems), problems)
	}
	return nil
}
