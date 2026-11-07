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
//
// By default commenter does not check for duplicate comments (--dedupe=none).
// With --dedupe=any it skips issues that already have an identical comment,
// even if the --query text filter fails to exclude them (GitHub search does not
// reliably match free-text phrases inside existing comments).
// With --dedupe=latest it only skips issues whose most recent comment is
// identical, so an issue that was reopened and went through a lifecycle again
// can be commented on a second time. Skipped issues do not count towards --ceiling.
//
// By default --updated relies on GitHub's updated timestamp, which any activity
// bumps, including comments from bots such as CI failure reports. With
// --ignore-activity-from, an issue is instead only considered inactive if it
// has no comments, reviews or commits within --updated by anyone not in the
// list. Issues that are still active by that measure are skipped and do not
// count towards --ceiling. As the search can then no longer filter on --updated,
// and GitHub returns at most 1000 search results, --query should be narrow,
// e.g. a single repo.
package main

import (
	"bytes"
	"context"
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

	githubql "github.com/shurcooL/githubv4"

	"sigs.k8s.io/prow/pkg/config/secret"
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
	dedupeAny    = "any"
)

// activityBatchSize is how many issues are looked up per GraphQL query. GitHub
// accepts up to 100 IDs in nodes(ids:), but fewer keep the query well within
// GitHub's GraphQL timeout for issues with a lot of activity.
const activityBatchSize = 50

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
	flag.Var(&o.endpoint, "endpoint", "GitHub's API endpoint")
	flag.StringVar(&o.graphqlEndpoint, "graphql-endpoint", github.DefaultGraphQLEndpoint, "GitHub's GraphQL API Endpoint")
	flag.StringVar(&o.token, "token", "", "Path to github token")
	flag.StringVar(&o.dedupe, "dedupe", dedupeNone, "Skip an issue if an identical comment exists: 'none' disables deduplication, 'latest' checks only the most recent comment, 'any' checks all comments")
	flag.BoolVar(&o.random, "random", false, "Choose random issues to comment on from the query")
	flag.StringVar(&o.ignoreActivityFrom, "ignore-activity-from", "", "Comma-separated logins whose activity does not count towards --updated, e.g. CI bots. When set, --updated is checked against comments, reviews and commits by anyone else instead of GitHub's updated timestamp. Use with a narrow --query, as the search no longer filters on --updated and GitHub returns at most 1000 results")
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
	ceiling            int
	comment            string
	includeArchived    bool
	includeClosed      bool
	includeLocked      bool
	useTemplate        bool
	query              string
	endpoint           flagutil.Strings
	graphqlEndpoint    string
	token              string
	updated            time.Duration
	confirm            bool
	random             bool
	dedupe             string
	ignoreActivityFrom string
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

func makeQuery(query string, includeArchived, includeClosed, includeLocked bool, minUpdated time.Duration, byActivity bool) (string, error) {
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
		if byActivity {
			// Bot activity bumps the updated timestamp, so it cannot be used to
			// filter here. Inactivity is checked after the search instead, and
			// an issue cannot be inactive for longer than it exists.
			parts = append(parts, "created:<="+latest.Format(time.RFC3339))
		} else {
			parts = append(parts, "updated:<="+latest.Format(time.RFC3339))
		}
	}
	return strings.Join(parts, " "), nil
}

type client interface {
	CreateComment(owner, repo string, number int, comment string) error
	FindIssues(query, sort string, asc bool) ([]github.Issue, error)
	ListIssueComments(org, repo string, number int) ([]github.IssueComment, error)
	QueryWithGitHubAppsSupport(ctx context.Context, q interface{}, vars map[string]interface{}, org string) error
}

// activityFilter skips issues that had activity within the last minInactive,
// not counting activity by the ignored logins. The zero value disables it.
type activityFilter struct {
	ignored     map[string]bool
	minInactive time.Duration
	now         time.Time
}

func newActivityFilter(logins string, minInactive time.Duration, now time.Time) activityFilter {
	f := activityFilter{minInactive: minInactive, now: now}
	for _, l := range strings.Split(logins, ",") {
		if l = strings.TrimSpace(l); l != "" {
			if f.ignored == nil {
				f.ignored = map[string]bool{}
			}
			f.ignored[normalizeLogin(l)] = true
		}
	}
	return f
}

func (f activityFilter) enabled() bool {
	return len(f.ignored) > 0
}

// normalizeLogin makes logins comparable across the REST API, which reports
// GitHub Apps as "name[bot]", and the GraphQL API, which reports them as "name".
func normalizeLogin(login string) string {
	return strings.TrimSuffix(github.NormLogin(login), "[bot]")
}

type activityActor struct {
	Login string
}

type activityComment struct {
	Author    activityActor
	CreatedAt time.Time
}

// activityIssueItem covers the timeline items of issues, which pull requests
// share.
//
// Timeline events such as label changes or force pushes are not counted, as
// GitHub silently leaves out some of them when querying many issues at once.
type activityIssueItem struct {
	Typename     string          `graphql:"__typename"`
	IssueComment activityComment `graphql:"... on IssueComment"`
}

func (i activityIssueItem) activity() activity {
	if i.Typename == "IssueComment" {
		return activity{i.IssueComment.Author.Login, i.IssueComment.CreatedAt}
	}
	// Not expected given the queried item types. An unknown login counts as
	// activity, so an issue is never treated as more inactive than it is.
	return activity{}
}

type activityPullRequestItem struct {
	activityIssueItem
	PullRequestReview activityComment `graphql:"... on PullRequestReview"`
	PullRequestCommit struct {
		Commit struct {
			CommittedDate time.Time
			Author        struct {
				User activityActor
			}
		}
	} `graphql:"... on PullRequestCommit"`
}

func (i activityPullRequestItem) activity() activity {
	switch i.Typename {
	case "PullRequestReview":
		return activity{i.PullRequestReview.Author.Login, i.PullRequestReview.CreatedAt}
	case "PullRequestCommit":
		return activity{i.PullRequestCommit.Commit.Author.User.Login, i.PullRequestCommit.Commit.CommittedDate}
	}
	return i.activityIssueItem.activity()
}

type activityPageInfo struct {
	HasNextPage bool
}

// The timeline items are limited to those since $since, the start of the
// --updated window. For commits, GitHub uses the commit date, which a rebase
// updates too.
type activityPullRequest struct {
	ID            string
	TimelineItems struct {
		PageInfo activityPageInfo
		Nodes    []activityPullRequestItem
	} `graphql:"timelineItems(since: $since, first: 100, itemTypes: [ISSUE_COMMENT, PULL_REQUEST_REVIEW, PULL_REQUEST_COMMIT])"`
}

type activityIssue struct {
	ID            string
	TimelineItems struct {
		PageInfo activityPageInfo
		Nodes    []activityIssueItem
	} `graphql:"timelineItems(since: $since, first: 100, itemTypes: [ISSUE_COMMENT])"`
}

type activityNode struct {
	Typename    string              `graphql:"__typename"`
	PullRequest activityPullRequest `graphql:"... on PullRequest"`
	Issue       activityIssue       `graphql:"... on Issue"`
}

// activity is a single timeline item: who did it and when.
type activity struct {
	login string
	at    time.Time
}

// timeline is the activity on an issue or pull request within the --updated window.
type timeline struct {
	id string
	// items holds the activity, truncated if there is more.
	items     []activity
	truncated bool
}

func newTimeline[T interface{ activity() activity }](id string, page activityPageInfo, nodes []T) timeline {
	t := timeline{id: id, truncated: page.HasNextPage}
	for _, n := range nodes {
		t.items = append(t.items, n.activity())
	}
	return t
}

func (n activityNode) timeline() timeline {
	if n.Typename == "PullRequest" {
		return newTimeline(n.PullRequest.ID, n.PullRequest.TimelineItems.PageInfo, n.PullRequest.TimelineItems.Nodes)
	}
	return newTimeline(n.Issue.ID, n.Issue.TimelineItems.PageInfo, n.Issue.TimelineItems.Nodes)
}

type activityQuery struct {
	Nodes []activityNode `graphql:"nodes(ids: $ids)"`
}

// active returns whether t has activity by anyone but f.ignored.
func (f activityFilter) active(t timeline) bool {
	for _, a := range t.items {
		if !f.ignored[normalizeLogin(a.login)] {
			return true
		}
	}
	// Everything fetched was ignored activity, but there may be more. Treat
	// the issue as active so that it is never treated as more inactive than
	// it is.
	return t.truncated
}

// inactive returns the issues that have had no counted activity for at least
// f.minInactive.
func (f activityFilter) inactive(c client, issues []github.Issue) (map[string]bool, error) {
	ids := make([]githubql.ID, 0, len(issues))
	for _, i := range issues {
		ids = append(ids, githubql.ID(i.NodeID))
	}
	vars := map[string]interface{}{
		"ids":   ids,
		"since": githubql.DateTime{Time: f.now.Add(-f.minInactive)},
	}
	var q activityQuery
	if err := c.QueryWithGitHubAppsSupport(context.Background(), &q, vars, ""); err != nil {
		return nil, err
	}
	inactive := map[string]bool{}
	for _, n := range q.Nodes {
		if t := n.timeline(); !f.active(t) {
			inactive[t.id] = true
		}
	}
	return inactive, nil
}

// normalizeComment makes comment bodies comparable across GitHub round-trips,
// which can differ in line endings and surrounding whitespace.
func normalizeComment(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
}

// hasComment returns true if existing contains comment, only checking the most
// recent comment for --dedupe=latest
func hasComment(existing []github.IssueComment, comment, dedupe string) bool {
	if dedupe == dedupeLatest && len(existing) > 0 {
		existing = existing[len(existing)-1:]
	}
	want := normalizeComment(comment)
	for _, ec := range existing {
		if normalizeComment(ec.Body) == want {
			return true
		}
	}
	return false
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
	if o.dedupe != dedupeNone && o.dedupe != dedupeLatest && o.dedupe != dedupeAny {
		log.Fatalf("invalid --dedupe %q, must be %q, %q or %q", o.dedupe, dedupeNone, dedupeLatest, dedupeAny)
	}
	activity := newActivityFilter(o.ignoreActivityFrom, o.updated, time.Now())
	if activity.enabled() && o.updated <= 0 {
		log.Fatal("--ignore-activity-from requires --updated")
	}

	if err := secret.Add(o.token); err != nil {
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
		c, err = github.NewClient(secret.GetTokenGenerator(o.token), secret.Censor, o.graphqlEndpoint, o.endpoint.Strings()...)
	} else {
		c, err = github.NewDryRunClient(secret.GetTokenGenerator(o.token), secret.Censor, o.graphqlEndpoint, o.endpoint.Strings()...)
	}
	if err != nil {
		log.Fatalf("Failed to construct GitHub client: %v", err)
	}

	query, err := makeQuery(o.query, o.includeArchived, o.includeClosed, o.includeLocked, o.updated, activity.enabled())
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
	if err := run(c, query, sort, asc, o.random, commenter, o.ceiling, o.dedupe, activity); err != nil {
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

func run(c client, query, sort string, asc, random bool, commenter func(meta) (string, error), ceiling int, dedupe string, activity activityFilter) error {
	log.Printf("Searching: %s", query)
	issues, err := c.FindIssues(query, sort, asc)
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
	var inactive map[string]bool
	var lookupErr error
	for n, i := range issues {
		if ceiling > 0 && acted == ceiling {
			log.Printf("Stopping at --ceiling=%d of %d results", acted, len(issues))
			break
		}
		if activity.enabled() {
			if n%activityBatchSize == 0 {
				batch := issues[n:min(n+activityBatchSize, len(issues))]
				if inactive, lookupErr = activity.inactive(c, batch); lookupErr != nil {
					// Skip the batch rather than stop, so that one failing
					// batch does not keep the rest from being processed.
					msg := fmt.Sprintf("Failed to look up activity of %d issues from %s: %v", len(batch), i.HTMLURL, lookupErr)
					log.Print(msg)
					problems = append(problems, msg)
				}
			}
			if !inactive[i.NodeID] {
				if lookupErr == nil {
					log.Printf("Skipping %s: active within --updated=%s, ignoring activity by --ignore-activity-from", i.HTMLURL, activity.minInactive)
				}
				continue
			}
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
		if dedupe != dedupeNone {
			existing, err := c.ListIssueComments(org, repo, number)
			if err != nil {
				msg := fmt.Sprintf("Failed to list comments for %s/%s#%d: %v", org, repo, number, err)
				log.Print(msg)
				problems = append(problems, msg)
				continue
			}
			if hasComment(existing, comment, dedupe) {
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
