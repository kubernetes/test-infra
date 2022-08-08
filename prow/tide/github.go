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

package tide

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git/types"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/tide/blockers"

	githubql "github.com/shurcooL/githubv4"
	"github.com/sirupsen/logrus"
)

type querier func(ctx context.Context, q interface{}, vars map[string]interface{}, org string) error

func datedQuery(q string, start, end time.Time) string {
	return fmt.Sprintf("%s %s", q, dateToken(start, end))
}

// Enforcing interface implementation check at compile time
var _ provider = (*GitHubProvider)(nil)

// GitHubProvider implements provider, used by tide Controller for
// interacting directly with GitHub.
//
// Tide Controller should only use GitHubProvider for communicating with GitHub.
type GitHubProvider struct {
	cfg                config.Getter
	ghc                githubClient
	usesGitHubAppsAuth bool

	*mergeChecker
	logger *logrus.Entry
}

func newGitHubProvider(
	logger *logrus.Entry,
	ghc githubClient,
	cfg config.Getter,
	mergeChecker *mergeChecker,
	usesGitHubAppsAuth bool,
) *GitHubProvider {
	return &GitHubProvider{
		logger:             logger,
		ghc:                ghc,
		cfg:                cfg,
		usesGitHubAppsAuth: usesGitHubAppsAuth,
		mergeChecker:       mergeChecker,
	}
}

func (gi *GitHubProvider) blockers() (blockers.Blockers, error) {
	label := gi.cfg().Tide.BlockerLabel
	if label == "" {
		return blockers.Blockers{}, nil
	}

	gi.logger.WithField("blocker_label", label).Debug("Searching for blocker issues")
	orgExcepts, repos := gi.cfg().Tide.Queries.OrgExceptionsAndRepos()
	orgs := make([]string, 0, len(orgExcepts))
	for org := range orgExcepts {
		orgs = append(orgs, org)
	}
	orgRepoQuery := orgRepoQueryStrings(orgs, repos.UnsortedList(), orgExcepts)
	return blockers.FindAll(gi.ghc, gi.logger, label, orgRepoQuery, gi.usesGitHubAppsAuth)
}

// Query gets all open PRs based on tide configuration.
func (gi *GitHubProvider) Query() (map[string]CodeReviewCommon, error) {
	lock := sync.Mutex{}
	wg := sync.WaitGroup{}
	prs := make(map[string]CodeReviewCommon)
	var errs []error
	for i, query := range gi.cfg().Tide.Queries {

		// Use org-sharded queries only when GitHub apps auth is in use
		var queries map[string]string
		if gi.usesGitHubAppsAuth {
			queries = query.OrgQueries()
		} else {
			queries = map[string]string{"": query.Query()}
		}

		for org, q := range queries {
			org, q, i := org, q, i
			wg.Add(1)
			go func() {
				defer wg.Done()
				results, err := gi.search(gi.ghc.QueryWithGitHubAppsSupport, gi.logger, q, time.Time{}, time.Now(), org)

				resultString := "success"
				if err != nil {
					resultString = "error"
				}
				tideMetrics.queryResults.WithLabelValues(strconv.Itoa(i), org, resultString).Inc()

				lock.Lock()
				defer lock.Unlock()
				if err != nil && len(results) == 0 {
					gi.logger.WithField("query", q).WithError(err).Warn("Failed to execute query.")
					errs = append(errs, fmt.Errorf("query %d, err: %w", i, err))
					return
				}
				if err != nil {
					gi.logger.WithError(err).WithField("query", q).Warning("found partial results")
				}

				for _, pr := range results {
					crc := CodeReviewCommonFromPullRequest(&pr)
					prs[prKey(crc)] = *crc
				}
			}()
		}
	}
	wg.Wait()

	return prs, utilerrors.NewAggregate(errs)
}

func (gi *GitHubProvider) GetRef(org, repo, ref string) (string, error) {
	return gi.ghc.GetRef(org, repo, ref)
}

func (gi *GitHubProvider) GetTideContextPolicy(gitClient git.ClientFactory, org, repo, branch string, baseSHAGetter config.RefGetter, headSHA string) (contextChecker, error) {
	return gi.cfg().GetTideContextPolicy(gitClient, org, repo, branch, baseSHAGetter, headSHA)
}

func (gi *GitHubProvider) refsForJob(sp subpool, prs []CodeReviewCommon) prowapi.Refs {
	refs := prowapi.Refs{
		Org:     sp.org,
		Repo:    sp.repo,
		BaseRef: sp.branch,
		BaseSHA: sp.sha,
	}
	for _, pr := range prs {
		refs.Pulls = append(
			refs.Pulls,
			prowapi.Pull{
				Number: pr.Number,
				Title:  pr.Title,
				Author: string(pr.AuthorLogin),
				SHA:    pr.HeadRefOID,
			},
		)
	}
	return refs
}

func (gi *GitHubProvider) prMergeMethod(crc *CodeReviewCommon) (types.PullRequestMergeType, error) {
	return gi.mergeChecker.prMergeMethod(gi.cfg().Tide, crc)
}

func (gi *GitHubProvider) search(query querier, log *logrus.Entry, q string, start, end time.Time, org string) ([]PullRequest, error) {
	start = floor(start)
	end = floor(end)
	log = log.WithFields(logrus.Fields{
		"query": q,
		"start": start.String(),
		"end":   end.String(),
	})
	requestStart := time.Now()
	var cursor *githubql.String
	vars := map[string]interface{}{
		"query":        githubql.String(datedQuery(q, start, end)),
		"searchCursor": cursor,
	}

	var totalCost, remaining int
	var ret []PullRequest
	var sq searchQuery
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	for {
		log.Debug("Sending query")
		if err := query(ctx, &sq, vars, org); err != nil {
			if cursor != nil {
				err = fmt.Errorf("cursor: %q, err: %w", *cursor, err)
			}
			return ret, err
		}
		totalCost += int(sq.RateLimit.Cost)
		remaining = int(sq.RateLimit.Remaining)
		for _, n := range sq.Search.Nodes {
			ret = append(ret, n.PullRequest)
		}
		if !sq.Search.PageInfo.HasNextPage {
			break
		}
		cursor = &sq.Search.PageInfo.EndCursor
		vars["searchCursor"] = cursor
		log = log.WithField("searchCursor", *cursor)
	}
	log.WithFields(logrus.Fields{
		"duration":       time.Since(requestStart).String(),
		"pr_found_count": len(ret),
		"cost":           totalCost,
		"remaining":      remaining,
	}).Debug("Finished query")
	return ret, nil
}

func (gi *GitHubProvider) prepareMergeDetails(commitTemplates config.TideMergeCommitTemplate, pr CodeReviewCommon, mergeMethod types.PullRequestMergeType) github.MergeDetails {
	ghMergeDetails := github.MergeDetails{
		SHA:         pr.HeadRefOID,
		MergeMethod: string(mergeMethod),
	}

	if commitTemplates.Title != nil {
		var b bytes.Buffer

		if err := commitTemplates.Title.Execute(&b, pr); err != nil {
			gi.logger.Errorf("error executing commit title template: %v", err)
		} else {
			ghMergeDetails.CommitTitle = b.String()
		}
	}

	if commitTemplates.Body != nil {
		var b bytes.Buffer

		if err := commitTemplates.Body.Execute(&b, pr); err != nil {
			gi.logger.Errorf("error executing commit body template: %v", err)
		} else {
			ghMergeDetails.CommitMessage = b.String()
		}
	}

	return ghMergeDetails
}

func (gi *GitHubProvider) mergePRs(sp subpool, prs []CodeReviewCommon, dontUpdateStatus *threadSafePRSet) error {
	var merged, failed []int
	defer func() {
		if len(merged) == 0 {
			return
		}
		tideMetrics.merges.WithLabelValues(sp.org, sp.repo, sp.branch).Observe(float64(len(merged)))
	}()

	var errs []error
	log := sp.log.WithField("merge-targets", prNumbers(prs))
	tideConfig := gi.cfg().Tide

	for i, pr := range prs {
		log := log.WithFields(pr.logFields())
		mergeMethod, err := gi.prMergeMethod(&pr)
		if err != nil {
			log.WithError(err).Error("Failed to determine merge method.")
			errs = append(errs, err)
			failed = append(failed, pr.Number)
			continue
		}

		// Ensure tide context has success state, otherwise PR merge will fail if branch protection
		// in github is enabled and the loop to change tide context hasn't done it already
		dontUpdateStatus.insert(sp.org, sp.repo, pr.Number)
		if err := setTideStatusSuccess(pr, gi.ghc, gi.cfg(), log); err != nil {
			log.WithError(err).Error("Unable to set tide context to SUCCESS.")
			errs = append(errs, err)
			failed = append(failed, pr.Number)
			continue
		}

		commitTemplates := tideConfig.MergeCommitTemplate(config.OrgRepo{Org: sp.org, Repo: sp.repo})
		keepTrying, err := tryMerge(func() error {
			ghMergeDetails := gi.prepareMergeDetails(commitTemplates, pr, mergeMethod)
			return gi.ghc.Merge(sp.org, sp.repo, pr.Number, ghMergeDetails)
		})
		if err != nil {
			// These are user errors, shouldn't be printed as tide errors
			log.WithError(err).Debug("Merge failed.")
		} else {
			log.Info("Merged.")
			merged = append(merged, pr.Number)
		}
		if !keepTrying {
			break
		}
		// If we successfully merged this PR and have more to merge, sleep to give
		// GitHub time to recalculate mergeability.
		if err == nil && i+1 < len(prs) {
			sleep(time.Second * 5)
		}
	}

	if len(errs) == 0 {
		return nil
	}

	// Construct a more informative error.
	var batch string
	if len(prs) > 1 {
		batch = fmt.Sprintf(" from batch %v", prNumbers(prs))
		if len(merged) > 0 {
			batch = fmt.Sprintf("%s, partial merge %v", batch, merged)
		}
	}
	return fmt.Errorf("failed merging %v%s: %w", failed, batch, utilerrors.NewAggregate(errs))
}

// headContexts gets the status contexts for the commit with OID == pr.HeadRefOID
//
// First, we try to get this value from the commits we got with the PR query.
// Unfortunately the 'last' commit ordering is determined by author date
// not commit date so if commits are reordered non-chronologically on the PR
// branch the 'last' commit isn't necessarily the logically last commit.
// We list multiple commits with the query to increase our chance of success,
// but if we don't find the head commit we have to ask GitHub for it
// specifically (this costs an API token).
//
// This function is very GitHub centric, make sure this that is only referenced
// by GitHub interactor.
func (gi *GitHubProvider) headContexts(pr *CodeReviewCommon) ([]Context, error) {
	log := gi.logger
	commits := pr.GitHubCommits()
	if commits != nil {
		for _, node := range commits.Nodes {
			if string(node.Commit.OID) == pr.HeadRefOID {
				return append(node.Commit.Status.Contexts, checkRunNodesToContexts(log, node.Commit.StatusCheckRollup.Contexts.Nodes)...), nil
			}
		}
	}
	// We didn't get the head commit from the query (the commits must not be
	// logically ordered) so we need to specifically ask GitHub for the status
	// and coerce it to a graphql type.
	org := pr.Org
	repo := pr.Repo
	// Log this event so we can tune the number of commits we list to minimize this.
	// TODO alvaroaleman: Add checkrun support here. Doesn't seem to happen often though,
	// openshift doesn't have a single occurrence of this in the past seven days.
	log.Warnf("'last' %d commits didn't contain logical last commit. Querying GitHub...", len(commits.Nodes))
	combined, err := gi.ghc.GetCombinedStatus(org, repo, pr.HeadRefOID)
	if err != nil {
		return nil, fmt.Errorf("failed to get the combined status: %w", err)
	}
	checkRunList, err := gi.ghc.ListCheckRuns(org, repo, pr.HeadRefOID)
	if err != nil {
		return nil, fmt.Errorf("Failed to list checkruns: %w", err)
	}
	checkRunNodes := make([]CheckRunNode, 0, len(checkRunList.CheckRuns))
	for _, checkRun := range checkRunList.CheckRuns {
		checkRunNodes = append(checkRunNodes, CheckRunNode{CheckRun: CheckRun{
			Name: githubql.String(checkRun.Name),
			// They are uppercase in the V4 api and lowercase in the V3 api
			Conclusion: githubql.String(strings.ToUpper(checkRun.Conclusion)),
			Status:     githubql.String(strings.ToUpper(checkRun.Status)),
		}})
	}

	contexts := make([]Context, 0, len(combined.Statuses)+len(checkRunNodes))
	for _, status := range combined.Statuses {
		contexts = append(contexts, Context{
			Context:     githubql.String(status.Context),
			Description: githubql.String(status.Description),
			State:       githubql.StatusState(strings.ToUpper(status.State)),
		})
	}
	contexts = append(contexts, checkRunNodesToContexts(log, checkRunNodes)...)

	// Add a commit with these contexts to pr for future look ups.
	if commits := pr.GitHubCommits(); commits != nil {
		commits.Nodes = append(commits.Nodes,
			struct{ Commit Commit }{
				Commit: Commit{
					OID:    githubql.String(pr.HeadRefOID),
					Status: struct{ Contexts []Context }{Contexts: contexts},
				},
			},
		)
	}
	return contexts, nil
}

// dateToken generates a GitHub search query token for the specified date range.
// See: https://help.github.com/articles/understanding-the-search-syntax/#query-for-dates
func dateToken(start, end time.Time) string {
	// GitHub's GraphQL API silently fails if you provide it with an invalid time
	// string.
	// Dates before 1970 (unix epoch) are considered invalid.
	startString, endString := "*", "*"
	if start.Year() >= 1970 {
		startString = start.Format(github.SearchTimeFormat)
	}
	if end.Year() >= 1970 {
		endString = end.Format(github.SearchTimeFormat)
	}
	return fmt.Sprintf("updated:%s..%s", startString, endString)
}

func floor(t time.Time) time.Time {
	if t.Before(github.FoundingYear) {
		return github.FoundingYear
	}
	return t
}
