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

package plugin

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	githubql "github.com/shurcooL/githubv4"
	"github.com/sirupsen/logrus"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const (
	// PluginName is the name of this plugin
	PluginName         = labels.NeedsRebase
	needsRebaseMessage = "PR needs rebase."
)

var sleep = time.Sleep

type githubClient interface {
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
	CreateComment(org, repo string, number int, comment string) error
	BotUserChecker() (func(candidate string) bool, error)
	AddLabel(org, repo string, number int, label string) error
	RemoveLabel(org, repo string, number int, label string) error
	IsMergeable(org, repo string, number int, sha string) (bool, error)
	DeleteStaleComments(org, repo string, number int, comments []github.IssueComment, isStale func(github.IssueComment) bool) error
	QueryWithGitHubAppsSupport(ctx context.Context, q interface{}, vars map[string]interface{}, org string) error
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
}

// HelpProvider constructs the PluginHelp for this plugin that takes into account enabled repositories.
// HelpProvider defines the type for function that construct the PluginHelp for plugins.
func HelpProvider(_ []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	return &pluginhelp.PluginHelp{
			Description: `The needs-rebase plugin manages the '` + labels.NeedsRebase + `' label by removing it from Pull Requests that are mergeable and adding it to those which are not.
The plugin reacts to commit changes on PRs in addition to periodically scanning all open PRs for any changes to mergeability that could have resulted from changes in other PRs.`,
		},
		nil
}

// HandlePullRequestEvent handles a GitHub pull request event and adds or removes a
// "needs-rebase" label based on whether the GitHub api considers the PR mergeable
func HandlePullRequestEvent(log *logrus.Entry, ghc githubClient, pre *github.PullRequestEvent) error {
	if pre.Action != github.PullRequestActionOpened && pre.Action != github.PullRequestActionSynchronize && pre.Action != github.PullRequestActionReopened {
		return nil
	}

	return handle(log, ghc, &pre.PullRequest)
}

// HandleIssueCommentEvent handles a GitHub issue comment event and adds or removes a
// "needs-rebase" label if the issue is a PR based on whether the GitHub api considers
// the PR mergeable
func HandleIssueCommentEvent(log *logrus.Entry, ghc githubClient, ice *github.IssueCommentEvent) error {
	if !ice.Issue.IsPullRequest() {
		return nil
	}
	pr, err := ghc.GetPullRequest(ice.Repo.Owner.Login, ice.Repo.Name, ice.Issue.Number)
	if err != nil {
		return err
	}

	return handle(log, ghc, pr)
}

// handle handles a GitHub PR to determine if the "needs-rebase"
// label needs to be added or removed. It depends on GitHub mergeability check
// to decide the need for a rebase.
func handle(log *logrus.Entry, ghc githubClient, pr *github.PullRequest) error {
	if pr.Merged {
		return nil
	}
	// Before checking mergeability wait a few seconds to give github a chance to calculate it.
	// This initial delay prevents us from always wasting the first API token.
	sleep(time.Second * 5)

	org := pr.Base.Repo.Owner.Login
	repo := pr.Base.Repo.Name
	number := pr.Number
	sha := pr.Head.SHA
	*log = *log.WithFields(logrus.Fields{
		github.OrgLogField:  org,
		github.RepoLogField: repo,
		github.PrLogField:   number,
		"head-sha":          sha,
	})

	mergeable, err := ghc.IsMergeable(org, repo, number, sha)
	if err != nil {
		return err
	}
	issueLabels, err := ghc.GetIssueLabels(org, repo, number)
	if err != nil {
		return err
	}
	hasLabel := github.HasLabel(labels.NeedsRebase, issueLabels)

	return takeAction(log, ghc, org, repo, number, pr.User.Login, hasLabel, mergeable)
}

const searchQueryPrefix = "archived:false is:pr is:open"

// HandleAll checks all orgs and repos that enabled this plugin for open PRs to
// determine if the "needs-rebase" label needs to be added or removed. It
// depends on GitHub's mergeability check to decide the need for a rebase.
func HandleAll(log *logrus.Entry, ghc githubClient, config *plugins.Configuration, usesAppsAuth bool) error {
	log.Info("Checking all PRs.")
	orgs, repos := config.EnabledReposForExternalPlugin(PluginName)
	if len(orgs) == 0 && len(repos) == 0 {
		log.Warnf("No repos have been configured for the %s plugin", PluginName)
		return nil
	}

	var prs []pullRequest
	var errs []error
	for org, queries := range constructQueries(log, time.Now(), orgs, repos, usesAppsAuth) {
		// Do _not_ parallelize this. It will trigger GitHubs abuse detection and we don't really care anyways except
		// when developing.
		for _, query := range queries {
			found, err := search(context.Background(), log, ghc, query, org)
			prs = append(prs, found...)
			errs = append(errs, err)
		}
	}
	if err := utilerrors.NewAggregate(errs); err != nil {
		if len(prs) == 0 {
			return err
		}
		log.WithError(err).Error("Encountered errors when querying GitHub but will process received results anyways")
	}
	log.WithField("prs_found_count", len(prs)).Debug("Processing all found PRs")

	for _, pr := range prs {
		// Skip PRs that are calculating mergeability. They will be updated by event or next loop.
		if pr.Mergeable == githubql.MergeableStateUnknown {
			continue
		}
		org := string(pr.Repository.Owner.Login)
		repo := string(pr.Repository.Name)
		num := int(pr.Number)
		var hasLabel bool
		for _, label := range pr.Labels.Nodes {
			if label.Name == labels.NeedsRebase {
				hasLabel = true
				break
			}
		}
		l := log.WithFields(logrus.Fields{
			"org":       org,
			"repo":      repo,
			"pr":        num,
			"mergeable": pr.Mergeable,
			"has_label": hasLabel,
		})
		l.Debug("Processing PR")
		err := takeAction(
			l,
			ghc,
			org,
			repo,
			num,
			string(pr.Author.Login),
			hasLabel,
			pr.Mergeable == githubql.MergeableStateMergeable,
		)
		if err != nil {
			l.WithError(err).Error("Error handling PR.")
		}
	}
	return nil
}

// takeAction adds or removes the "needs-rebase" label based on the current
// state of the PR (hasLabel and mergeable). It also handles adding and
// removing GitHub comments notifying the PR author that a rebase is needed.
func takeAction(log *logrus.Entry, ghc githubClient, org, repo string, num int, author string, hasLabel, mergeable bool) error {
	if !mergeable && !hasLabel {
		if err := ghc.AddLabel(org, repo, num, labels.NeedsRebase); err != nil {
			log.WithError(err).Errorf("Failed to add %q label.", labels.NeedsRebase)
		}
		msg := plugins.FormatSimpleResponse(author, needsRebaseMessage)
		return ghc.CreateComment(org, repo, num, msg)
	} else if mergeable && hasLabel {
		// remove label and prune comment
		if err := ghc.RemoveLabel(org, repo, num, labels.NeedsRebase); err != nil {
			log.WithError(err).Errorf("Failed to remove %q label.", labels.NeedsRebase)
		}
		botUserChecker, err := ghc.BotUserChecker()
		if err != nil {
			return err
		}
		return ghc.DeleteStaleComments(org, repo, num, nil, shouldPrune(botUserChecker))
	}
	return nil
}

func shouldPrune(isBot func(string) bool) func(github.IssueComment) bool {
	return func(ic github.IssueComment) bool {
		return isBot(ic.User.Login) &&
			strings.Contains(ic.Body, needsRebaseMessage)
	}
}

func search(ctx context.Context, log *logrus.Entry, ghc githubClient, q, org string) ([]pullRequest, error) {
	var ret []pullRequest
	vars := map[string]interface{}{
		"query":        githubql.String(q),
		"searchCursor": (*githubql.String)(nil),
	}
	var totalCost int
	var remaining int
	requestStart := time.Now()
	var pageCount int
	for {
		pageCount++
		sq := searchQuery{}
		if err := ghc.QueryWithGitHubAppsSupport(ctx, &sq, vars, org); err != nil {
			return nil, err
		}
		totalCost += int(sq.RateLimit.Cost)
		remaining = int(sq.RateLimit.Remaining)
		for _, n := range sq.Search.Nodes {
			ret = append(ret, n.PullRequest)
		}
		if !sq.Search.PageInfo.HasNextPage {
			break
		}
		vars["searchCursor"] = githubql.NewString(sq.Search.PageInfo.EndCursor)
	}
	log = log.WithFields(logrus.Fields{
		"query":          q,
		"duration":       time.Since(requestStart).String(),
		"pr_found_count": len(ret),
		"search_pages":   pageCount,
		"cost":           totalCost,
		"remaining":      remaining,
	})
	log.Debug("Finished query")

	// https://github.community/t/graphql-github-api-how-to-get-more-than-1000-pull-requests/13838/10
	if len(ret) == 1000 {
		log.Warning("Query returned 1k PRs, which is the max number of results per query allowed by GitHub. This indicates that we were not able to process all PRs.")
	}
	return ret, nil
}

// See: https://developer.github.com/v4/object/pullrequest/.
type pullRequest struct {
	Number githubql.Int
	Author struct {
		Login githubql.String
	}
	Repository struct {
		Name  githubql.String
		Owner struct {
			Login githubql.String
		}
	}
	Labels struct {
		Nodes []struct {
			Name githubql.String
		}
	} `graphql:"labels(first:100)"`
	Mergeable githubql.MergeableState
}

// See: https://developer.github.com/v4/query/.
type searchQuery struct {
	RateLimit struct {
		Cost      githubql.Int
		Remaining githubql.Int
	}
	Search struct {
		PageInfo struct {
			HasNextPage githubql.Boolean
			EndCursor   githubql.String
		}
		Nodes []struct {
			PullRequest pullRequest `graphql:"... on PullRequest"`
		}
	} `graphql:"search(type: ISSUE, first: 100, after: $searchCursor, query: $query)"`
}

// constructQueries constructs the v4 queries for the peridic scan.
// It returns a map[org][]query.
func constructQueries(log *logrus.Entry, now time.Time, orgs, repos []string, usesGitHubAppsAuth bool) map[string][]string {
	result := map[string][]string{}

	// GitHub hard caps queries at 1k results, so always do one query per org and one for
	// all repos. Ref: https://github.community/t/graphql-github-api-how-to-get-more-than-1000-pull-requests/13838/11
	for _, org := range orgs {
		// https://img.17qq.com/images/crqhcuueqhx.jpeg
		if org == "kubernetes" {
			result[org] = append(result[org], searchQueryPrefix+` org:"kubernetes" -repo:"kubernetes/kubernetes"`)

			// Sharding by creation time > 2 months ago gives us around 50% of PRs per query (585 for the newer ones, 538 for the older ones when testing)
			twoMonthsAgoISO8601 := now.Add(-2 * 30 * 24 * time.Hour).Format("2006-01-02")
			result[org] = append(result[org], searchQueryPrefix+` repo:"kubernetes/kubernetes" created:>=`+twoMonthsAgoISO8601)
			result[org] = append(result[org], searchQueryPrefix+` repo:"kubernetes/kubernetes" created:<`+twoMonthsAgoISO8601)
		} else {
			result[org] = append(result[org], searchQueryPrefix+` org:"`+org+`"`)
		}
	}

	reposQueries := map[string]*bytes.Buffer{}
	for _, repo := range repos {
		slashSplit := strings.Split(repo, "/")
		if n := len(slashSplit); n != 2 {
			log.WithField("repo", repo).Warn("Found repo that was not in org/repo format, ignoring...")
			continue
		}
		org := slashSplit[0]
		if _, hasOrgQuery := result[org]; hasOrgQuery {
			log.WithField("repo", repo).Warn("Plugin was enabled for repo even though it is already enabled for the org, ignoring...")
			continue
		}
		var b *bytes.Buffer
		if usesGitHubAppsAuth {
			if reposQueries[org] == nil {
				reposQueries[org] = bytes.NewBufferString(searchQueryPrefix)
			}
			b = reposQueries[org]
		} else {
			if reposQueries[""] == nil {
				reposQueries[""] = bytes.NewBufferString(searchQueryPrefix)
			}
			b = reposQueries[""]
		}
		fmt.Fprintf(b, " repo:\"%s\"", repo)
	}
	for org, repoQuery := range reposQueries {
		result[org] = append(result[org], repoQuery.String())
	}

	return result
}
