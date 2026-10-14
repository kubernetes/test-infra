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
// The --github-token-path (or --github-app-id / --github-app-private-key-path)
// determines who interacts with github.
// By default commenter runs in dry mode, add --confirm to make it leave comments.
// The --updated, --include-closed, --ceiling options provide minor safeguards
// around leaving excessive comments.
//
// By default commenter does not check for duplicate comments (--dedupe=none).
// With --dedupe=latest it skips issues whose most recent comment is identical,
// even if the --query text filter fails to exclude them (GitHub search does not
// reliably match free-text phrases inside existing comments). An issue that
// was reopened and went through a lifecycle again can be commented on a second
// time. Skipped issues do not count towards --ceiling.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"sigs.k8s.io/prow/pkg/flagutil"
	"sigs.k8s.io/prow/pkg/github"
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

const (
	dedupeNone   = "none"
	dedupeLatest = "latest"
)

func flagOptions() options {
	o := options{
		endpoint: flagutil.NewStrings(github.DefaultAPIEndpoint),
	}
	flag.StringVar(&o.query, "query", "", "See https://help.github.com/articles/searching-issues-and-pull-requests/")
	flag.DurationVar(&o.updated, "updated", 2*time.Hour, "Filter to issues unmodified for at least this long if set")
	flag.BoolVar(&o.includeArchived, "include-archived", false, "Match archived issues if set")
	flag.BoolVar(&o.includeClosed, "include-closed", false, "Match closed issues if set")
	flag.BoolVar(&o.includeLocked, "include-locked", false, "Match locked issues if set")
	flag.BoolVar(&o.confirm, "confirm", false, "Mutate github if set")
	flag.StringVar(&o.comment, "comment", "", "Append the following comment to matching issues")
	flag.BoolVar(&o.useTemplate, "template", false, templateHelp)
	flag.IntVar(&o.ceiling, "ceiling", 3, "Maximum number of issues to modify, 0 for infinite")
	flag.Var(&o.endpoint, "endpoint", "Deprecated: use --github-endpoint instead. GitHub's API endpoint")
	flag.StringVar(&o.graphqlEndpoint, "graphql-endpoint", github.DefaultGraphQLEndpoint, "Deprecated: use --github-graphql-endpoint instead. GitHub's GraphQL API Endpoint")
	flag.StringVar(&o.token, "token", "", "Deprecated: use --github-token-path instead. Path to github token")
	flag.StringVar(&o.dedupe, "dedupe", dedupeNone, "Skip an issue if an identical comment exists: 'none' disables deduplication, 'latest' checks only the most recent comment")
	flag.BoolVar(&o.random, "random", false, "Choose random issues to comment on from the query")
	flag.StringVar(&o.githubOrg, "github-org", "", "GitHub org used for search-API routing. Required when authenticating as a GitHub App via --github-app-id.")
	o.gitHubOptions.AddFlags(flag.CommandLine)
	flag.Parse()
	return o
}

// handleDeprecatedFlags links the deprecated flags to their --github-* replacements, and rejects mixing the deprecated flags with any of the --github-* flags.
func handleDeprecatedFlags(fs *flag.FlagSet, o *options) error {
	flagsSet := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { flagsSet[f.Name] = true })

	usesDeprecated := flagsSet["token"] || flagsSet["endpoint"] || flagsSet["graphql-endpoint"]

	usesGitHubOptions := false
	githubOptionsFlags := flag.NewFlagSet("", flag.ContinueOnError)
	(&flagutil.GitHubOptions{}).AddFlags(githubOptionsFlags)
	githubOptionsFlags.VisitAll(func(f *flag.Flag) {
		if flagsSet[f.Name] {
			usesGitHubOptions = true
		}
	})

	if usesDeprecated && usesGitHubOptions {
		return errors.New("the deprecated --token, --endpoint and --graphql-endpoint flags cannot be combined with the --github-* flags")
	}

	if flagsSet["token"] {
		if err := fs.Set("github-token-path", o.token); err != nil {
			return err
		}
	}
	if flagsSet["graphql-endpoint"] {
		if err := fs.Set("github-graphql-endpoint", o.graphqlEndpoint); err != nil {
			return err
		}
	}
	if flagsSet["endpoint"] {
		for _, endpoint := range o.endpoint.Strings() {
			if err := fs.Set("github-endpoint", endpoint); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateOptions enforces the credential requirements that flagutil.GitHubOptions does not cover on its own.
func validateOptions(o options) error {
	usesApp := o.gitHubOptions.AppID != "" || o.gitHubOptions.AppPrivateKeyPath != ""
	if o.gitHubOptions.TokenPath == "" && !usesApp {
		return errors.New("no GitHub credentials: set --github-token-path (or the deprecated --token), or --github-app-id together with --github-app-private-key-path")
	}
	if usesApp && o.githubOrg == "" {
		return errors.New("--github-org is required when authenticating as a GitHub App")
	}
	return nil
}

type meta struct {
	Number int
	Org    string
	Repo   string
	Issue  github.Issue
}

type options struct {
	ceiling         int
	comment         string
	includeArchived bool
	includeClosed   bool
	includeLocked   bool
	useTemplate     bool
	query           string
	endpoint        flagutil.Strings
	graphqlEndpoint string
	token           string
	updated         time.Duration
	confirm         bool
	random          bool
	dedupe          string
	githubOrg       string
	gitHubOptions   flagutil.GitHubOptions
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

func makeQuery(query string, includeArchived, includeClosed, includeLocked bool, minUpdated time.Duration) (string, error) {
	// GitHub used to allow \n but changed it at some point to result in no results at all
	query = strings.ReplaceAll(query, "\n", " ")
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
	if !includeLocked {
		if strings.Contains(query, "is:locked") {
			return "", errors.New("is:locked requires --include-locked")
		}
		parts = append(parts, "is:unlocked")
	} else if strings.Contains(query, "is:unlocked") {
		return "", errors.New("is:unlocked conflicts with --include-locked")
	}
	if minUpdated != 0 {
		latest := time.Now().Add(-minUpdated)
		parts = append(parts, "updated:<="+latest.Format(time.RFC3339))
	}
	return strings.Join(parts, " "), nil
}

type client interface {
	CreateComment(owner, repo string, number int, comment string) error
	FindIssuesWithOrg(org, query, sort string, asc bool) ([]github.Issue, error)
	ListIssueComments(org, repo string, number int) ([]github.IssueComment, error)
}

// normalizeComment makes comment bodies comparable across GitHub round-trips,
// which can differ in line endings and surrounding whitespace.
func normalizeComment(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	o := flagOptions()
	if err := handleDeprecatedFlags(flag.CommandLine, &o); err != nil {
		log.Fatal(err)
	}

	if o.query == "" {
		log.Fatal("empty --query")
	}
	if o.comment == "" {
		log.Fatal("empty --comment")
	}
	if o.dedupe != dedupeNone && o.dedupe != dedupeLatest {
		log.Fatalf("invalid --dedupe %q, must be %q or %q", o.dedupe, dedupeNone, dedupeLatest)
	}

	if err := validateOptions(o); err != nil {
		log.Fatal(err)
	}
	if err := o.gitHubOptions.Validate(!o.confirm); err != nil {
		log.Fatalf("Invalid GitHub options: %v", err)
	}

	c, err := o.gitHubOptions.GitHubClient(!o.confirm)
	if err != nil {
		log.Fatalf("Failed to construct GitHub client: %v", err)
	}

	query, err := makeQuery(o.query, o.includeArchived, o.includeClosed, o.includeLocked, o.updated)
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
	if err := run(c, o.githubOrg, query, sort, asc, o.random, commenter, o.ceiling, o.dedupe); err != nil {
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

func run(c client, org, query, sort string, asc, random bool, commenter func(meta) (string, error), ceiling int, dedupe string) error {
	log.Printf("Searching: %s", query)
	issues, err := c.FindIssuesWithOrg(org, query, sort, asc)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}
	problems := []string{}
	log.Printf("Found %d matches", len(issues))
	if random {
		rand.Shuffle(len(issues), func(i, j int) {
			issues[i], issues[j] = issues[j], issues[i]
		})

	}
	acted := 0
	for _, i := range issues {
		if ceiling > 0 && acted == ceiling {
			log.Printf("Stopping at --ceiling=%d of %d results", acted, len(issues))
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
		if dedupe == dedupeLatest {
			existing, err := c.ListIssueComments(org, repo, number)
			if err != nil {
				msg := fmt.Sprintf("Failed to list comments for %s/%s#%d: %v", org, repo, number, err)
				log.Print(msg)
				problems = append(problems, msg)
				continue
			}
			want := normalizeComment(comment)
			if len(existing) > 0 && normalizeComment(existing[len(existing)-1].Body) == want {
				log.Printf("Skipping %s: an identical comment already exists (--dedupe=%s)", i.HTMLURL, dedupe)
				continue
			}
		}
		acted++
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
