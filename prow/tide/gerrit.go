/*
Copyright 2022 The Kubernetes Authors.

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
	"strconv"
	"sync"
	"time"

	configflagutil "k8s.io/test-infra/prow/flagutil/config"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	gerritadaptor "k8s.io/test-infra/prow/gerrit/adapter"
	"k8s.io/test-infra/prow/gerrit/client"
	"k8s.io/test-infra/prow/git/types"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/moonraker"
	"k8s.io/test-infra/prow/tide/blockers"
	"k8s.io/test-infra/prow/tide/history"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/andygrunwald/go-gerrit"
	githubql "github.com/shurcooL/githubv4"
	"github.com/sirupsen/logrus"
)

const (
	// tideEnablementLabel is the Gerrit label that has to be voted for enabling
	// Tide. By default a PR is not considered by Tide unless the author of the
	// PR toggled this label.
	tideEnablementLabel = "Prow-Auto-Submit"
	// ref:
	// https://gerrit-review.googlesource.com/Documentation/user-search.html#_search_operators.
	// Also good to know: `(repo:repo-A OR repo:repo-B)`
	gerritDefaultQueryParam = "status:open+-is:wip+is:submittable"
)

func gerritQueryParam(optInByDefault bool) string {
	// Whenever a the `Prow-Auto-Submit` label is voted with -1 by anyone, the
	// PR has to be excluded from Tide.
	enablementLabelQueryParam := "+-label:" + tideEnablementLabel + "=-1"
	// By default require `Prow-Auto-Submit` label.
	// If the repo enabled optInByDefault, `Prow-Auto-Submit` is no longer
	// required. But users can still temporarily opting out of merge automation
	// by voting -1 on this label.
	if !optInByDefault {
		// We want `-label:Prow-Auto-Submit=-1 label:Prow-Auto-Submit`
		enablementLabelQueryParam += "+label:" + tideEnablementLabel
	}
	return gerritDefaultQueryParam + enablementLabelQueryParam
}

// gerritContextChecker implements contextChecker, it's a permissive no-op
// implementation for Gerrit only, as context checking only applies to GitHub.
type gerritContextChecker struct{}

// IsOptional tells whether a context is optional.
func (gcc *gerritContextChecker) IsOptional(string) bool {
	return true
}

// MissingRequiredContexts tells if required contexts are missing from the list of contexts provided.
func (gcc *gerritContextChecker) MissingRequiredContexts([]string) []string {
	return nil
}

type gerritClient interface {
	QueryChangesForProject(instance, project string, lastUpdate time.Time, rateLimit int, additionalFilters ...string) ([]gerrit.ChangeInfo, error)
	GetChange(instance, id string, additionalFields ...string) (*gerrit.ChangeInfo, error)
	GetBranchRevision(instance, project, branch string) (string, error)
	SubmitChange(instance, id string, wait bool) (*gerrit.ChangeInfo, error)
	SetReview(instance, id, revision, message string, _ map[string]string) error
}

// NewController makes a Controller out of the given clients.
func NewGerritController(
	mgr manager,
	cfgAgent *config.Agent,
	gc git.ClientFactory,
	maxRecordsPerPool int,
	opener io.Opener,
	historyURI,
	statusURI string,
	logger *logrus.Entry,
	configOptions configflagutil.ConfigOptions,
	cookieFilePath string,
	maxQPS, maxBurst int,
) (*Controller, error) {
	if logger == nil {
		logger = logrus.NewEntry(logrus.StandardLogger())
	}
	hist, err := history.New(maxRecordsPerPool, opener, historyURI)
	if err != nil {
		return nil, fmt.Errorf("error initializing history client from %q: %w", historyURI, err)
	}

	ctx := context.Background()
	// Shared fields
	statusUpdate := &statusUpdate{
		dontUpdateStatus: &threadSafePRSet{},
		newPoolPending:   make(chan bool),
	}

	var ircg config.InRepoConfigGetter
	if configOptions.MoonrakerAddress != "" {
		moonrakerClient, err := moonraker.NewClient(configOptions.MoonrakerAddress, cfgAgent)
		if err != nil {
			logrus.WithError(err).Fatal("Error getting Moonraker client.")
		}
		ircg = moonrakerClient
	} else {
		var err error
		ircg, err = config.NewInRepoConfigCache(configOptions.InRepoConfigCacheSize, cfgAgent, gc)
		if err != nil {
			return nil, fmt.Errorf("failed creating inrepoconfig cache: %v", err)
		}
	}

	provider := newGerritProvider(logger, cfgAgent.Config, mgr.GetClient(), ircg, cookieFilePath, "", maxQPS, maxBurst)
	syncCtrl, err := newSyncController(ctx, logger, mgr, provider, cfgAgent.Config, gc, hist, false, statusUpdate)
	if err != nil {
		return nil, err
	}
	return &Controller{syncCtrl: syncCtrl}, nil
}

// Enforcing interface implementation check at compile time
var _ provider = (*GerritProvider)(nil)

// GerritProvider implements provider, used by Tide Controller for
// interacting directly with Gerrit.
//
// Tide Controller should only use GerritProvider for communicating with Gerrit.
type GerritProvider struct {
	cfg         config.Getter
	gc          gerritClient
	pjclientset ctrlruntimeclient.Client

	cookiefilePath     string
	inRepoConfigGetter config.InRepoConfigGetter
	tokenPathOverride  string

	logger *logrus.Entry
}

func newGerritProvider(
	logger *logrus.Entry,
	cfg config.Getter,
	pjclientset ctrlruntimeclient.Client,
	ircg config.InRepoConfigGetter,
	cookiefilePath string,
	tokenPathOverride string,
	maxQPS, maxBurst int,
) *GerritProvider {
	gerritClient, err := client.NewClient(nil, maxQPS, maxBurst)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating gerrit client.")
	}
	orgRepoConfigGetter := func() *config.GerritOrgRepoConfigs {
		return &cfg().Tide.Gerrit.Queries
	}
	gerritClient.ApplyGlobalConfig(orgRepoConfigGetter, nil, cookiefilePath, tokenPathOverride, nil)

	return &GerritProvider{
		logger:             logger,
		cfg:                cfg,
		pjclientset:        pjclientset,
		gc:                 gerritClient,
		inRepoConfigGetter: ircg,
		cookiefilePath:     cookiefilePath,
		tokenPathOverride:  tokenPathOverride,
	}
}

// Query returns all PRs from configured Gerrit org/repos.
func (p *GerritProvider) Query() (map[string]CodeReviewCommon, error) {
	// lastUpdate is used by Gerrit adapter for achieving incremental query. In
	// Tide case we want to get everything so use default time.Time, which
	// should be 1970,1,1.
	var lastUpdate time.Time

	var wg sync.WaitGroup
	errChan := make(chan error)
	type changesFromProject struct {
		instance string
		project  string
		changes  []gerrit.ChangeInfo
	}
	resChan := make(chan changesFromProject)
	for instance, projs := range p.cfg().Tide.Gerrit.Queries.AllRepos() {
		instance, projs := instance, projs
		for projName, projFilter := range projs {
			wg.Add(1)
			var optInByDefault bool
			if projFilter != nil {
				optInByDefault = projFilter.OptInByDefault
			}
			go func(projName string, optInByDefault bool) {
				changes, err := p.gc.QueryChangesForProject(instance, projName, lastUpdate, p.cfg().Gerrit.RateLimit, gerritQueryParam(optInByDefault))
				if err != nil {
					p.logger.WithFields(logrus.Fields{"instance": instance, "project": projName}).WithError(err).Warn("Querying gerrit project for changes.")
					errChan <- fmt.Errorf("failed querying project '%s' from instance '%s': %v", projName, instance, err)
					return
				}
				resChan <- changesFromProject{instance: instance, project: projName, changes: changes}
			}(projName, optInByDefault)
		}
	}

	var combinedErrs []error
	res := make(map[string]CodeReviewCommon)
	go func() {
		for {
			select {
			case err := <-errChan:
				combinedErrs = append(combinedErrs, err)
				wg.Done()
			case changes := <-resChan:
				for _, pr := range changes.changes {
					crc := CodeReviewCommonFromGerrit(&pr, changes.instance)
					res[prKey(crc)] = *crc
				}
				wg.Done()
			}
		}
	}()

	wg.Wait()

	// Let's not return error unless all queries failed.
	if len(combinedErrs) > 0 && len(res) == 0 {
		return nil, utilerrors.NewAggregate(combinedErrs)
	}
	return res, nil
}

func (p *GerritProvider) blockers() (blockers.Blockers, error) {
	// This is not supported yet, so return an empty blocker for now.
	return blockers.Blockers{}, nil
}

func (p *GerritProvider) isAllowedToMerge(crc *CodeReviewCommon) (string, error) {
	// gci.Mergeable is only set if this feature is enabled on the Gerrit Host.
	// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-info
	if crc.Mergeable == string(githubql.MergeableStateConflicting) {
		return "PR has a merge conflict.", nil
	}
	return "", nil
}

// GetRef gets the latest revision from org/repo/branch.
func (p *GerritProvider) GetRef(org, repo, ref string) (string, error) {
	return p.gc.GetBranchRevision(org, repo, ref)
}

// headContexts gets the status contexts for the commit with OID ==
// pr.HeadRefOID
//
// Assuming all submission requirements are already met as the PRs queried are
// already submittable. So the focus here is to ensure that all prowjobs were
// tested against latest baseSHA.
// Prow parses baseSHA from the `Description` field of a context, will make sure
// that all Prow jobs that vote to required labels are represented here.
func (p *GerritProvider) headContexts(crc *CodeReviewCommon) ([]Context, error) {
	var res []Context

	selector := map[string]string{
		kube.GerritRevision:   crc.HeadRefOID,
		kube.ProwJobTypeLabel: string(prowapi.PresubmitJob),
		kube.OrgLabel:         crc.Org,
		kube.RepoLabel:        crc.Repo,
		kube.PullLabel:        strconv.Itoa(crc.Number),
	}
	var pjs prowapi.ProwJobList
	if err := p.pjclientset.List(context.Background(), &pjs, ctrlruntimeclient.MatchingLabels(selector)); err != nil {
		return nil, fmt.Errorf("Cannot list prowjob with selector %v", selector)
	}

	// keep track of latest prowjobs only
	latestPjs := make(map[string]*prowapi.ProwJob)
	for _, pj := range pjs.Items {
		pj := pj
		if exist, ok := latestPjs[pj.Spec.Context]; ok && exist.CreationTimestamp.After(pj.CreationTimestamp.Time) {
			continue
		}
		latestPjs[pj.Spec.Context] = &pj
	}

	for _, pj := range latestPjs {
		res = append(res, Context{
			Context:     githubql.String(pj.Spec.Context),
			Description: githubql.String(config.ContextDescriptionWithBaseSha(pj.Status.Description, pj.Spec.Refs.BaseSHA)),
			State:       githubql.StatusState(pj.Status.State),
		})
	}

	return res, nil
}

func (p *GerritProvider) mergePRs(sp subpool, prs []CodeReviewCommon, _ *threadSafePRSet) ([]CodeReviewCommon, error) {
	logger := p.logger.WithFields(logrus.Fields{"repo": sp.repo, "org": sp.org, "branch": sp.branch, "prs": len(prs)})
	logger.Info("Merging subpool.")

	isBatch := len(prs) > 1

	var merged []CodeReviewCommon
	var errs []error
	for _, pr := range prs {
		logger := logger.WithField("id", pr.Gerrit.ID)
		logger.Info("Submitting change.")
		_, err := p.gc.SubmitChange(sp.org, pr.Gerrit.ID, true)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed submitting change '%s' from org '%s': %v", sp.org, pr.Gerrit.ID, err))
		} else {
			merged = append(merged, pr)
		}
		// Comment on the PR if it's a batch.
		// In case of flaky tests, Tide triggered prowjobs for highest priority
		// PR might fail even when batch prowjobs passed. And in this case Crier
		// would report this failure on the PR before Tide merges the PR, this
		// might cause confusing to users so comment on the PR explaining that
		// the merge was based on batch testing.
		if isBatch && err != nil {
			msg := fmt.Sprintf("The Tide batch containing current change passed all required prowjobs, so this submission was performed by Tide. See %s/tide-history for record", p.cfg().Gerrit.DeckURL)
			if err := p.gc.SetReview(sp.org, pr.Gerrit.ID, pr.Gerrit.CurrentRevision, msg, nil); err != nil {
				logger.WithError(err).Warn("Failed commenting after batch submission.")
			}
		}
	}
	return merged, utilerrors.NewAggregate(errs)
}

// GetTideContextPolicy returns an empty config.TideContextPolicy struct.
//
// These information are only for determining whether a PR is ready for merge or
// not, this in Gerrit is handled by Gerrit query filters, so this is not useful
// for Gerrit.
func (p *GerritProvider) GetTideContextPolicy(org, repo, branch string, baseSHAGetter config.RefGetter, crc *CodeReviewCommon) (contextChecker, error) {
	return &gerritContextChecker{}, nil
}

func (p *GerritProvider) prMergeMethod(crc *CodeReviewCommon) *types.PullRequestMergeType {
	var res types.PullRequestMergeType
	pr := crc.Gerrit
	if pr == nil {
		return nil
	}

	// Translate merge methods to types that Git could understand. The merge
	// methods for Gerrit are documented at
	// https://gerrit-review.googlesource.com/Documentation/config-gerrit.html#repository.
	// Git can only understand MergeIfNecessary, MergeMerge, MergeRebase, MergeSquash.
	switch pr.SubmitType {
	case "MERGE_IF_NECESSARY":
		res = types.MergeIfNecessary
	case "FAST_FORWARD_ONLY":
		res = types.MergeMerge
	case "REBASE_IF_NECESSARY":
		res = types.MergeRebase
	case "REBASE_ALWAYS":
		res = types.MergeRebase
	case "MERGE_ALWAYS":
		res = types.MergeMerge
	default:
		res = types.MergeMerge
	}

	return &res
}

// GetPresubmits gets presubmit jobs for a PR.
//
// (TODO:chaodaiG): deduplicate this with GitHub, which means inrepoconfig
// processing all use cache client.
func (p *GerritProvider) GetPresubmits(identifier, baseBranch string, baseSHAGetter config.RefGetter, headSHAGetters ...config.RefGetter) ([]config.Presubmit, error) {
	// If InRepoConfigCache is provided, then it means that we want to fetch
	// from an inrepoconfig.
	if p.inRepoConfigGetter != nil {
		return p.inRepoConfigGetter.GetPresubmits(identifier, baseBranch, baseSHAGetter, headSHAGetters...)
	}
	// Get presubmits from Config alone.
	return p.cfg().GetPresubmitsStatic(identifier), nil
}

func (p *GerritProvider) GetChangedFiles(org, repo string, number int) ([]string, error) {
	// "CURRENT_FILES" lists all changed files from current revision, which is
	// what we want, "CURRENT_REVISION" is required for "CURRENT_FILES".
	// according to
	// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#list-changes.
	change, err := p.gc.GetChange(org, strconv.Itoa(number), "CURRENT_FILES", "CURRENT_REVISION")
	if err != nil {
		return nil, fmt.Errorf("failed get change: %v", err)
	}
	return client.ChangedFilesProvider(change)()
}

func (p *GerritProvider) refsForJob(sp subpool, prs []CodeReviewCommon) (prowapi.Refs, error) {
	var changes []client.ChangeInfo
	for _, pr := range prs {
		changes = append(changes, *pr.Gerrit)
	}
	return gerritadaptor.CreateRefs(sp.org, sp.repo, sp.branch, sp.sha, changes...)
}

func (p *GerritProvider) labelsAndAnnotations(instance string, jobLabels, jobAnnotations map[string]string, prs ...CodeReviewCommon) (labels, annotations map[string]string) {
	var changes []client.ChangeInfo
	for _, pr := range prs {
		changes = append(changes, *pr.Gerrit)
	}
	labels, annotations = gerritadaptor.LabelsAndAnnotations(instance, jobLabels, jobAnnotations, changes...)
	return
}

func (p *GerritProvider) jobIsRequiredByTide(ps *config.Presubmit, crc *CodeReviewCommon) bool {
	if ps.RunBeforeMerge {
		return true
	}

	requireLabels := sets.New[string]()
	for l, info := range crc.Gerrit.Labels {
		if !info.Optional {
			requireLabels.Insert(l)
		}
	}

	val, ok := ps.Labels[kube.GerritReportLabel]
	if !ok {
		return false
	}
	return requireLabels.Has(val)
}
