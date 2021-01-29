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
	"context"
	"fmt"
	"io/ioutil"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	githubql "github.com/shurcooL/githubv4"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/tide/blockers"
)

const (
	statusContext string = "tide"
	statusInPool         = "In merge pool."
	// statusNotInPool is a format string used when a PR is not in a tide pool.
	// The '%s' field is populated with the reason why the PR is not in a
	// tide pool or the empty string if the reason is unknown. See requirementDiff.
	statusNotInPool = "Not mergeable.%s"

	maxStatusDescriptionLength = 140
)

type storedState struct {
	// LatestPR is the update time of the most recent result
	LatestPR metav1.Time
	// PreviousQuery is the query most recently used for results
	PreviousQuery string
}

type statusController struct {
	pjClient ctrlruntimeclient.Client
	logger   *logrus.Entry
	config   config.Getter
	ghc      githubClient
	gc       git.ClientFactory

	mergeChecker *mergeChecker

	// newPoolPending is a size 1 chan that signals that the main Tide loop has
	// updated the 'poolPRs' field with a freshly updated pool.
	newPoolPending chan bool
	// shutDown is used to signal to the main controller that the statusController
	// has completed processing after newPoolPending is closed.
	shutDown chan bool

	// lastSyncStart is used to ensure that the status update period is at least
	// the minimum status update period.
	lastSyncStart time.Time

	sync.Mutex
	poolPRs          map[string]PullRequest
	requiredContexts map[string][]string
	blocks           blockers.Blockers
	baseSHAs         map[string]string

	storedState
	opener io.Opener
	path   string
}

func (sc *statusController) shutdown() {
	close(sc.newPoolPending)
	<-sc.shutDown
}

// requirementDiff calculates the diff between a PR and a TideQuery.
// This diff is defined with a string that describes some subset of the
// differences and an integer counting the total number of differences.
// The diff count should always reflect the scale of the differences between
// the current state of the PR and the query, but the message returned need not
// attempt to convey all of that information if some differences are more severe.
// For instance, we need to convey that a PR is open against a forbidden branch
// more than we need to detail which status contexts are failed against the PR.
// To this end, some differences are given a higher diff weight than others.
// Note: an empty diff can be returned if the reason that the PR does not match
// the TideQuery is unknown. This can happen if this function's logic
// does not match GitHub's and does not indicate that the PR matches the query.
func requirementDiff(pr *PullRequest, q *config.TideQuery, cc contextChecker) (string, int) {
	const maxLabelChars = 50
	var desc string
	var diff int
	// Drops labels if needed to fit the description text area, but keep at least 1.
	truncate := func(labels []string) []string {
		i := 1
		chars := len(labels[0])
		for ; i < len(labels); i++ {
			if chars+len(labels[i]) > maxLabelChars {
				break
			}
			chars += len(labels[i]) + 2 // ", "
		}
		return labels[:i]
	}

	// Weight incorrect branches with very high diff so that we select the query
	// for the correct branch.
	targetBranchDenied := false
	for _, excludedBranch := range q.ExcludedBranches {
		if string(pr.BaseRef.Name) == excludedBranch {
			targetBranchDenied = true
			break
		}
	}
	// if no allowlist is configured, the target is OK by default
	targetBranchAllowed := len(q.IncludedBranches) == 0
	for _, includedBranch := range q.IncludedBranches {
		if string(pr.BaseRef.Name) == includedBranch {
			targetBranchAllowed = true
			break
		}
	}
	if targetBranchDenied || !targetBranchAllowed {
		diff += 1000
		if desc == "" {
			desc = fmt.Sprintf(" Merging to branch %s is forbidden.", pr.BaseRef.Name)
		}
	}

	qAuthor := github.NormLogin(q.Author)
	prAuthor := github.NormLogin(string(pr.Author.Login))

	// Weight incorrect author with very high diff so that we select the query
	// for the correct author.
	if qAuthor != "" && prAuthor != qAuthor {
		diff += 1000
		if desc == "" {
			desc = fmt.Sprintf(" Must be by author %s.", qAuthor)
		}
	}

	// Weight incorrect milestone with relatively high diff so that we select the
	// query for the correct milestone (but choose favor query for correct branch).
	if q.Milestone != "" && (pr.Milestone == nil || string(pr.Milestone.Title) != q.Milestone) {
		diff += 100
		if desc == "" {
			desc = fmt.Sprintf(" Must be in milestone %s.", q.Milestone)
		}
	}

	// Weight incorrect labels and statues with low (normal) diff values.
	var missingLabels []string
	for _, l1 := range q.Labels {
		var found bool
		for _, l2 := range pr.Labels.Nodes {
			if string(l2.Name) == l1 {
				found = true
				break
			}
		}
		if !found {
			missingLabels = append(missingLabels, l1)
		}
	}
	diff += len(missingLabels)
	if desc == "" && len(missingLabels) > 0 {
		sort.Strings(missingLabels)
		trunced := truncate(missingLabels)
		if len(trunced) == 1 {
			desc = fmt.Sprintf(" Needs %s label.", trunced[0])
		} else {
			desc = fmt.Sprintf(" Needs %s labels.", strings.Join(trunced, ", "))
		}
	}

	var presentLabels []string
	for _, l1 := range q.MissingLabels {
		for _, l2 := range pr.Labels.Nodes {
			if string(l2.Name) == l1 {
				presentLabels = append(presentLabels, l1)
				break
			}
		}
	}
	diff += len(presentLabels)
	if desc == "" && len(presentLabels) > 0 {
		sort.Strings(presentLabels)
		trunced := truncate(presentLabels)
		if len(trunced) == 1 {
			desc = fmt.Sprintf(" Should not have %s label.", trunced[0])
		} else {
			desc = fmt.Sprintf(" Should not have %s labels.", strings.Join(trunced, ", "))
		}
	}

	// fixing label issues takes precedence over status contexts
	var contexts []string
	log := logrus.WithFields(pr.logFields())
	for _, commit := range pr.Commits.Nodes {
		if commit.Commit.OID == pr.HeadRefOID {
			for _, ctx := range unsuccessfulContexts(append(commit.Commit.Status.Contexts, checkRunNodesToContexts(log, commit.Commit.StatusCheckRollup.Contexts.Nodes)...), cc, log) {
				contexts = append(contexts, string(ctx.Context))
			}
		}
	}
	diff += len(contexts)
	if desc == "" && len(contexts) > 0 {
		sort.Strings(contexts)
		trunced := truncate(contexts)
		if len(trunced) == 1 {
			desc = fmt.Sprintf(" Job %s has not succeeded.", trunced[0])
		} else {
			desc = fmt.Sprintf(" Jobs %s have not succeeded.", strings.Join(trunced, ", "))
		}
	}

	// TODO(cjwagner): List reviews (states:[APPROVED], first: 1) as part of open
	// PR query.

	return desc, diff
}

// Returns expected status state and description.
// If a PR is not mergeable, we have to select a TideQuery to compare it against
// in order to generate a diff for the status description. We choose the query
// for the repo that the PR is closest to meeting (as determined by the number
// of unmet/violated requirements).
func (sc *statusController) expectedStatus(log *logrus.Entry, queryMap *config.QueryMap, pr *PullRequest, pool map[string]PullRequest, ccg contextCheckerGetter, blocks blockers.Blockers, baseSHA string) (string, string, error) {
	repo := config.OrgRepo{Org: string(pr.Repository.Owner.Login), Repo: string(pr.Repository.Name)}

	if reason, err := sc.mergeChecker.isAllowed(pr); err != nil {
		return "", "", fmt.Errorf("error checking if merge is allowed: %v", err)
	} else if reason != "" {
		return github.StatusError, fmt.Sprintf(statusNotInPool, " "+reason), nil
	}

	cc, err := ccg()
	if err != nil {
		return "", "", fmt.Errorf("failed to set up context register: %v", err)
	}

	if _, ok := pool[prKey(pr)]; !ok {
		// if the branch is blocked forget checking for a diff
		blockingIssues := blocks.GetApplicable(string(pr.Repository.Owner.Login), string(pr.Repository.Name), string(pr.BaseRef.Name))
		var numbers []string
		for _, issue := range blockingIssues {
			numbers = append(numbers, strconv.Itoa(issue.Number))
		}
		if len(numbers) > 0 {
			var s string
			if len(numbers) > 1 {
				s = "s"
			}
			return github.StatusError, fmt.Sprintf(statusNotInPool, fmt.Sprintf(" Merging is blocked by issue%s %s.", s, strings.Join(numbers, ", "))), nil
		}
		minDiffCount := -1
		var minDiff string
		for _, q := range queryMap.ForRepo(repo) {
			diff, diffCount := requirementDiff(pr, &q, cc)
			if minDiffCount == -1 || diffCount < minDiffCount {
				minDiffCount = diffCount
				minDiff = diff
			}
		}
		return github.StatusPending, fmt.Sprintf(statusNotInPool, minDiff), nil
	}

	indexKey := indexKeyPassingJobs(repo, baseSHA, string(pr.HeadRefOID))
	passingUpToDatePJs := &prowapi.ProwJobList{}
	if err := sc.pjClient.List(context.Background(), passingUpToDatePJs, ctrlruntimeclient.MatchingFields{indexNamePassingJobs: indexKey}); err != nil {
		// Just log the error and return success, as the PR is in the merge pool
		log.WithError(err).Error("Failed to list ProwJobs.")
		return github.StatusSuccess, statusInPool, nil
	}

	var passingUpToDateContexts []string
	for _, pj := range passingUpToDatePJs.Items {
		passingUpToDateContexts = append(passingUpToDateContexts, pj.Spec.Context)
	}
	if diff := cc.MissingRequiredContexts(passingUpToDateContexts); len(diff) > 0 {
		return github.StatePending, retestingStatus(diff), nil
	}
	return github.StatusSuccess, statusInPool, nil
}

func retestingStatus(retested []string) string {
	sort.Strings(retested)
	all := fmt.Sprintf(statusNotInPool, fmt.Sprintf(" Retesting: %s", strings.Join(retested, " ")))
	if len(all) > maxStatusDescriptionLength {
		s := ""
		if len(retested) > 1 {
			s = "s"
		}
		return fmt.Sprintf(statusNotInPool, fmt.Sprintf(" Retesting %d job%s.", len(retested), s))
	}
	return all
}

// targetURL determines the URL used for more details in the status
// context on GitHub. If no PR dashboard is configured, we will use
// the administrative Prow overview.
func targetURL(c *config.Config, pr *PullRequest, log *logrus.Entry) string {
	var link string
	if tideURL := c.Tide.TargetURL; tideURL != "" {
		link = tideURL
	} else if baseURL := c.Tide.GetPRStatusBaseURL(config.OrgRepo{Org: string(pr.Repository.Owner.Login), Repo: string(pr.Repository.Name)}); baseURL != "" {
		parseURL, err := url.Parse(baseURL)
		if err != nil {
			log.WithError(err).Error("Failed to parse PR status base URL")
		} else {
			prQuery := fmt.Sprintf("is:pr repo:%s author:%s head:%s", pr.Repository.NameWithOwner, pr.Author.Login, pr.HeadRefName)
			values := parseURL.Query()
			values.Set("query", prQuery)
			parseURL.RawQuery = values.Encode()
			link = parseURL.String()
		}
	}
	return link
}

func (sc *statusController) setStatuses(all []PullRequest, pool map[string]PullRequest, blocks blockers.Blockers, baseSHAs map[string]string, requiredContexts map[string][]string) {
	c := sc.config()
	// queryMap caches which queries match a repo.
	// Make a new one each sync loop as queries will change.
	queryMap := c.Tide.Queries.QueryMap()
	processed := sets.NewString()

	process := func(pr *PullRequest) {
		processed.Insert(prKey(pr))
		log := sc.logger.WithFields(pr.logFields())
		contexts, err := headContexts(log, sc.ghc, pr)
		if err != nil {
			log.WithError(err).Error("Getting head commit status contexts, skipping...")
			return
		}

		org := string(pr.Repository.Owner.Login)
		repo := string(pr.Repository.Name)
		branch := string(pr.BaseRef.Name)
		headSHA := string(pr.HeadRefOID)
		// baseSHA is an empty string for any PR that doesn't have a corresponding merge pool
		baseSHA := baseSHAs[poolKey(org, repo, branch)]
		baseSHAGetter := newBaseSHAGetter(baseSHAs, sc.ghc, org, repo, branch)

		cr := contextCheckerGetterFactory(c, sc.gc, org, repo, branch, baseSHAGetter, headSHA, requiredContexts[prKey(pr)])

		wantState, wantDesc, err := sc.expectedStatus(log, queryMap, pr, pool, cr, blocks, baseSHA)
		if err != nil {
			log.WithError(err).Error("getting expected status")
			return
		}
		var actualState githubql.StatusState
		var actualDesc string
		for _, ctx := range contexts {
			if string(ctx.Context) == statusContext {
				actualState = ctx.State
				actualDesc = string(ctx.Description)
			}
		}
		if len(wantDesc) > maxStatusDescriptionLength {
			original := wantDesc
			wantDesc = fmt.Sprintf("%s...", wantDesc[0:(maxStatusDescriptionLength-3)])
			log.WithField("original-desc", original).Warn("GitHub status description needed to be truncated to fit GH API limit")
		}
		actualState = githubql.StatusState(strings.ToLower(string(actualState)))
		if wantState != string(actualState) || wantDesc != actualDesc {
			if err := sc.ghc.CreateStatus(
				org,
				repo,
				headSHA,
				github.Status{
					Context:     statusContext,
					State:       wantState,
					Description: wantDesc,
					TargetURL:   targetURL(c, pr, log),
				}); err != nil {
				log.WithError(err).Errorf(
					"Failed to set status context from %q to %q and description from %q to %q",
					actualState,
					wantState,
					actualDesc,
					wantDesc,
				)
			}
		}
	}

	for _, pr := range all {
		process(&pr)
	}
	// The list of all open PRs may not contain a PR if it was merged before we
	// listed all open PRs. To prevent a new PR that starts in the pool and
	// immediately merges from missing a tide status context we need to ensure that
	// every PR in the pool is processed even if it doesn't appear in all.
	//
	// Note: We could still fail to update a status context if the statusController
	// falls behind the main Tide sync loop by multiple loops (if we are lapped).
	// This would be unlikely to occur, could only occur if the status update sync
	// period is longer than the main sync period, and would only result in a
	// missing tide status context on a successfully merged PR.
	for key, poolPR := range pool {
		if !processed.Has(key) {
			process(&poolPR)
		}
	}
}

func (sc *statusController) load() {
	if sc.path == "" {
		sc.logger.Debug("No stored state configured")
		return
	}
	entry := sc.logger.WithField("path", sc.path)
	reader, err := sc.opener.Reader(context.Background(), sc.path)
	if err != nil {
		entry.WithError(err).Warn("Cannot open stored state")
		return
	}
	defer io.LogClose(reader)

	buf, err := ioutil.ReadAll(reader)
	if err != nil {
		entry.WithError(err).Warn("Cannot read stored state")
		return
	}

	var stored storedState
	if err := yaml.Unmarshal(buf, &stored); err != nil {
		entry.WithError(err).Warn("Cannot unmarshal stored state")
		return
	}
	sc.storedState = stored
}

func (sc *statusController) save(ticker *time.Ticker) {
	for range ticker.C {
		if sc.path == "" {
			return
		}
		entry := sc.logger.WithField("path", sc.path)
		current := sc.storedState
		buf, err := yaml.Marshal(current)
		if err != nil {
			entry.WithError(err).Warn("Cannot marshal state")
			continue
		}
		writer, err := sc.opener.Writer(context.Background(), sc.path)
		if err != nil {
			entry.WithError(err).Warn("Cannot open state writer")
			continue
		}
		if _, err = writer.Write(buf); err != nil {
			entry.WithError(err).Warn("Cannot write state")
			io.LogClose(writer)
			continue
		}
		if err := writer.Close(); err != nil {
			entry.WithError(err).Warn("Failed to close written state")
		}
		entry.Debug("Saved status state")
	}
}

func (sc *statusController) run() {
	sc.load()
	ticks := time.NewTicker(time.Hour)
	defer ticks.Stop()
	go sc.save(ticks)
	for {
		// wait for a new pool
		if !<-sc.newPoolPending {
			// chan was closed
			break
		}
		sc.waitSync()
	}
	close(sc.shutDown)
}

// waitSync waits until the minimum status update period has elapsed then syncs,
// returning the sync start time.
// If newPoolPending is closed while waiting (indicating a shutdown request)
// this function returns immediately without syncing.
func (sc *statusController) waitSync() {
	// wait for the min sync period time to elapse if needed.
	wait := time.After(time.Until(sc.lastSyncStart.Add(sc.config().Tide.StatusUpdatePeriod.Duration)))
	for {
		select {
		case <-wait:
			sc.Lock()
			pool := sc.poolPRs
			blocks := sc.blocks
			baseSHAs := sc.baseSHAs
			if baseSHAs == nil {
				baseSHAs = map[string]string{}
			}
			requiredContexts := sc.requiredContexts
			sc.Unlock()
			sc.sync(pool, blocks, baseSHAs, requiredContexts)
			return
		case more := <-sc.newPoolPending:
			if !more {
				return
			}
		}
	}
}

func (sc *statusController) sync(pool map[string]PullRequest, blocks blockers.Blockers, baseSHAs map[string]string, requiredContexts map[string][]string) {
	sc.lastSyncStart = time.Now()
	defer func() {
		duration := time.Since(sc.lastSyncStart)
		sc.logger.WithField("duration", duration.String()).Info("Statuses synced.")
		tideMetrics.statusUpdateDuration.Set(duration.Seconds())
		tideMetrics.syncHeartbeat.WithLabelValues("status-update").Inc()
	}()

	sc.setStatuses(sc.search(), pool, blocks, baseSHAs, requiredContexts)
}

func (sc *statusController) search() []PullRequest {
	queries := sc.config().Tide.Queries
	if len(queries) == 0 {
		return nil
	}

	orgExceptions, repos := queries.OrgExceptionsAndRepos()
	orgs := sets.StringKeySet(orgExceptions)
	query := openPRsQuery(orgs.List(), repos.List(), orgExceptions)
	now := time.Now()
	log := sc.logger.WithField("query", query)
	if query != sc.PreviousQuery {
		// Query changed and/or tide restarted, recompute everything
		log.WithField("previously", sc.PreviousQuery).Info("Query changed, resetting start time to zero")
		sc.LatestPR = metav1.Time{}
		sc.PreviousQuery = query
	}

	// TODO @alvaroaleman: Add github apps support
	prs, err := search(sc.ghc.QueryWithGitHubAppsSupport, sc.logger, query, sc.LatestPR.Time, now, "")
	log.WithField("duration", time.Since(now).String()).Debugf("Found %d open PRs.", len(prs))
	if err != nil {
		log := log.WithError(err)
		if len(prs) == 0 {
			log.Error("Search failed")
			return nil
		}
		log.Warn("Search partially completed")
	}
	if len(prs) == 0 {
		log.WithField("latestPR", sc.LatestPR).Debug("no new results")
		return nil
	}

	latest := prs[len(prs)-1].UpdatedAt.Time
	if latest.IsZero() {
		log.WithField("latestPR", sc.LatestPR).Debug("latest PR has zero time")
		return prs
	}
	sc.LatestPR.Time = latest.Add(-30 * time.Second)
	log.WithField("latestPR", sc.LatestPR).Debug("Advanced start time")
	return prs
}

// newBaseSHAGetter is a refGetter that will look up the baseSHA from GitHub if necessary
// and if it did so, store in in the baseSHA map
func newBaseSHAGetter(baseSHAs map[string]string, ghc githubClient, org, repo, branch string) config.RefGetter {
	return func() (string, error) {
		if sha, exists := baseSHAs[poolKey(org, repo, branch)]; exists {
			return sha, nil
		}
		baseSHA, err := ghc.GetRef(org, repo, "heads/"+branch)
		if err != nil {
			return "", err
		}
		baseSHAs[poolKey(org, repo, branch)] = baseSHA
		return baseSHAs[poolKey(org, repo, branch)], nil
	}
}

func openPRsQuery(orgs, repos []string, orgExceptions map[string]sets.String) string {
	return "is:pr state:open sort:updated-asc " + orgRepoQueryString(orgs, repos, orgExceptions)
}

const indexNamePassingJobs = "tide-passing-jobs"

func indexKeyPassingJobs(repo config.OrgRepo, baseSHA, headSHA string) string {
	return fmt.Sprintf("%s@%s+%s", repo, baseSHA, headSHA)
}

func indexFuncPassingJobs(obj ctrlruntimeclient.Object) []string {
	pj := obj.(*prowapi.ProwJob)
	// We do not care about jobs other than presubmit and batch
	if pj.Spec.Type != prowapi.PresubmitJob && pj.Spec.Type != prowapi.BatchJob {
		return nil
	}
	if pj.Status.State != prowapi.SuccessState {
		return nil
	}
	if pj.Spec.Refs == nil {
		return nil
	}

	var result []string
	for _, pull := range pj.Spec.Refs.Pulls {
		result = append(result, indexKeyPassingJobs(config.OrgRepo{Org: pj.Spec.Refs.Org, Repo: pj.Spec.Refs.Repo}, pj.Spec.Refs.BaseSHA, pull.SHA))
	}
	return result
}

type contextCheckerGetter = func() (contextChecker, error)

func contextCheckerGetterFactory(cfg *config.Config, gc git.ClientFactory, org, repo, branch string, baseSHAGetter config.RefGetter, headSHA string, requiredContexts []string) contextCheckerGetter {
	return func() (contextChecker, error) {
		contextPolicy, err := cfg.GetTideContextPolicy(gc, org, repo, branch, baseSHAGetter, headSHA)
		if err != nil {
			return nil, err
		}
		contextPolicy.RequiredContexts = requiredContexts
		return contextPolicy, nil
	}
}
