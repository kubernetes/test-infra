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
	"sigs.k8s.io/yaml"

	"k8s.io/test-infra/pkg/io"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/tide/blockers"
)

const (
	statusContext string = "tide"
	statusInPool         = "In merge pool."
	// statusNotInPool is a format string used when a PR is not in a tide pool.
	// The '%s' field is populated with the reason why the PR is not in a
	// tide pool or the empty string if the reason is unknown. See requirementDiff.
	statusNotInPool = "Not mergeable.%s"
)

type storedState struct {
	// LatestPR is the update time of the most recent result
	LatestPR metav1.Time
	// PreviousQuery is the query most recently used for results
	PreviousQuery string
}

type statusController struct {
	logger *logrus.Entry
	config config.Getter
	ghc    githubClient

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
	poolPRs map[string]PullRequest
	blocks  blockers.Blockers

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
	targetBranchBlacklisted := false
	for _, excludedBranch := range q.ExcludedBranches {
		if string(pr.BaseRef.Name) == excludedBranch {
			targetBranchBlacklisted = true
			break
		}
	}
	// if no whitelist is configured, the target is OK by default
	targetBranchWhitelisted := len(q.IncludedBranches) == 0
	for _, includedBranch := range q.IncludedBranches {
		if string(pr.BaseRef.Name) == includedBranch {
			targetBranchWhitelisted = true
			break
		}
	}
	if targetBranchBlacklisted || !targetBranchWhitelisted {
		diff += 1000
		if desc == "" {
			desc = fmt.Sprintf(" Merging to branch %s is forbidden.", pr.BaseRef.Name)
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
	for _, commit := range pr.Commits.Nodes {
		if commit.Commit.OID == pr.HeadRefOID {
			for _, ctx := range unsuccessfulContexts(commit.Commit.Status.Contexts, cc, logrus.New().WithFields(pr.logFields())) {
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
func expectedStatus(queryMap *config.QueryMap, pr *PullRequest, pool map[string]PullRequest, cc contextChecker, blocks blockers.Blockers) (string, string) {
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
			return github.StatusError, fmt.Sprintf(statusNotInPool, fmt.Sprintf(" Merging is blocked by issue%s %s.", s, strings.Join(numbers, ", ")))
		}
		minDiffCount := -1
		var minDiff string
		for _, q := range queryMap.ForRepo(string(pr.Repository.Owner.Login), string(pr.Repository.Name)) {
			diff, diffCount := requirementDiff(pr, &q, cc)
			if minDiffCount == -1 || diffCount < minDiffCount {
				minDiffCount = diffCount
				minDiff = diff
			}
		}
		return github.StatusPending, fmt.Sprintf(statusNotInPool, minDiff)
	}
	return github.StatusSuccess, statusInPool
}

// targetURL determines the URL used for more details in the status
// context on GitHub. If no PR dashboard is configured, we will use
// the administrative Prow overview.
func targetURL(c config.Getter, pr *PullRequest, log *logrus.Entry) string {
	var link string
	if tideURL := c().Tide.TargetURL; tideURL != "" {
		link = tideURL
	} else if baseURL := c().Tide.PRStatusBaseURL; baseURL != "" {
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

func (sc *statusController) setStatuses(all []PullRequest, pool map[string]PullRequest, blocks blockers.Blockers) {
	// queryMap caches which queries match a repo.
	// Make a new one each sync loop as queries will change.
	queryMap := sc.config().Tide.Queries.QueryMap()
	processed := sets.NewString()

	process := func(pr *PullRequest) {
		processed.Insert(prKey(pr))
		log := sc.logger.WithFields(pr.logFields())
		contexts, err := headContexts(log, sc.ghc, pr)
		if err != nil {
			log.WithError(err).Error("Getting head commit status contexts, skipping...")
			return
		}
		cr, err := sc.config().GetTideContextPolicy(
			string(pr.Repository.Owner.Login),
			string(pr.Repository.Name),
			string(pr.BaseRef.Name))
		if err != nil {
			log.WithError(err).Error("setting up context register")
			return
		}

		wantState, wantDesc := expectedStatus(queryMap, pr, pool, cr, blocks)
		var actualState githubql.StatusState
		var actualDesc string
		for _, ctx := range contexts {
			if string(ctx.Context) == statusContext {
				actualState = ctx.State
				actualDesc = string(ctx.Description)
			}
		}
		if wantState != strings.ToLower(string(actualState)) || wantDesc != actualDesc {
			if err := sc.ghc.CreateStatus(
				string(pr.Repository.Owner.Login),
				string(pr.Repository.Name),
				string(pr.HeadRefOID),
				github.Status{
					Context:     statusContext,
					State:       wantState,
					Description: wantDesc,
					TargetURL:   targetURL(sc.config, pr, log),
				}); err != nil {
				log.WithError(err).Errorf(
					"Failed to set status context from %q to %q.",
					string(actualState),
					wantState,
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
			sc.Unlock()
			sc.sync(pool, blocks)
			return
		case more := <-sc.newPoolPending:
			if !more {
				return
			}
		}
	}
}

func (sc *statusController) sync(pool map[string]PullRequest, blocks blockers.Blockers) {
	sc.lastSyncStart = time.Now()
	defer func() {
		duration := time.Since(sc.lastSyncStart)
		sc.logger.WithField("duration", duration.String()).Info("Statuses synced.")
		tideMetrics.statusUpdateDuration.Set(duration.Seconds())
	}()

	sc.setStatuses(sc.search(), pool, blocks)
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

	prs, err := search(sc.ghc.Query, sc.logger, query, sc.LatestPR.Time, now)
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

func openPRsQuery(orgs, repos []string, orgExceptions map[string]sets.String) string {
	return "is:pr state:open sort:updated-asc " + orgRepoQueryString(orgs, repos, orgExceptions)
}
