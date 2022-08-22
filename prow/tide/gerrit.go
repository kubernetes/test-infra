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
	"errors"
	"fmt"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/gerrit/client"
	"k8s.io/test-infra/prow/git/types"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/tide/blockers"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/andygrunwald/go-gerrit"
	githubql "github.com/shurcooL/githubv4"
	"github.com/sirupsen/logrus"
)

const (
	// ref:
	// https://gerrit-review.googlesource.com/Documentation/user-search.html#_search_operators.
	// Also good to know: `(repo:repo-A OR repo:repo-B)`
	gerritDefaultQueryParam = "status:open -is:wip is:submittable is:mergeable"
)

type gerritClient interface {
	QueryChangesForProject(instance, project string, lastUpdate time.Time, rateLimit int, addtionalFilters []string) ([]gerrit.ChangeInfo, error)
	GetBranchRevision(instance, project, branch string) (string, error)
}

// Enforcing interface implementation check at compile time
var _ provider = (*GerritProvider)(nil)

// GerritProvider implements provider, used by tide Controller for
// interacting directly with Gerrit.
//
// Tide Controller should only use GerritProvider for communicating with Gerrit.
type GerritProvider struct {
	cfg config.Getter
	gc  gerritClient

	pjclientset ctrlruntimeclient.Client

	cookiefilePath    string
	tokenPathOverride string

	*mergeChecker
	logger *logrus.Entry
}

func newGerritProvider(
	logger *logrus.Entry,
	cfg config.Getter,
	pjclientset ctrlruntimeclient.Client,
	mergeChecker *mergeChecker,
	cookiefilePath string,
	tokenPathOverride string,
) *GerritProvider {
	gerritClient, err := client.NewClient(nil)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating gerrit client.")
	}
	orgRepoConfigGetter := func() *config.GerritOrgRepoConfigs {
		return &cfg().Tide.Gerrit.Queries
	}
	gerritClient.ApplyGlobalConfig(orgRepoConfigGetter, nil, cookiefilePath, tokenPathOverride, nil)

	return &GerritProvider{
		logger:            logger,
		cfg:               cfg,
		pjclientset:       pjclientset,
		gc:                gerritClient,
		cookiefilePath:    cookiefilePath,
		tokenPathOverride: tokenPathOverride,
		mergeChecker:      mergeChecker,
	}
}

// Query returns all PRs from configured gerrit org/repos.
func (p *GerritProvider) Query() (map[string]CodeReviewCommon, error) {
	// lastUpdate is used by gerrit adapter for achieving incremental query. In
	// tide case we want to get everything so use default time.Time, which
	// should be 1970,1,1.
	var lastUpdate time.Time

	res := make(map[string]CodeReviewCommon)
	// This is querying serially, which would safely guard against quota issues.
	// TODO(chaodai): parallize this to boot the performance.
	for instance, projs := range p.cfg().Tide.Gerrit.Queries.AllRepos() {
		for projName := range projs {
			changes, err := p.gc.QueryChangesForProject(instance, projName, lastUpdate, p.cfg().Gerrit.RateLimit, []string{gerritDefaultQueryParam})
			if err != nil {
				p.logger.WithFields(logrus.Fields{"instance": instance, "project": projName}).WithError(err).Error("Querying gerrit project for changes.")
				continue
			}
			for _, pr := range changes {
				crc := CodeReviewCommonFromGerrit(&pr, instance)
				res[prKey(crc)] = *crc
			}
		}
	}

	return res, nil
}

func (p *GerritProvider) blockers() (blockers.Blockers, error) {
	// This is not supported yet, so return an empty blocker for now.
	return blockers.Blockers{}, nil
}

func (p *GerritProvider) isAllowedToMerge(crc *CodeReviewCommon) (string, error) {
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
	var pjs v1.ProwJobList
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

func (p *GerritProvider) mergePRs(sp subpool, prs []CodeReviewCommon, dontUpdateStatus *threadSafePRSet) error {
	p.logger.Info("The merge function hasn't been implemented yet, just logging for now.")
	return nil
}

// GetTideContextPolicy gets context policy defined by users + requirements from
// prow jobs.
func (p *GerritProvider) GetTideContextPolicy(gitClient git.ClientFactory, org, repo, branch string, baseSHAGetter config.RefGetter, crc *CodeReviewCommon) (contextChecker, error) {
	pr := crc.Gerrit
	if pr == nil {
		return nil, errors.New("programmer error: crc.Gerrit cannot be nil for GerritProvider")
	}

	required := sets.NewString()
	requiredIfPresent := sets.NewString()
	optional := sets.NewString()

	headSHAGetter := func() (string, error) {
		return crc.HeadRefOID, nil
	}
	presubmits, err := p.cfg().GetPresubmits(gitClient, org+"/"+repo, baseSHAGetter, headSHAGetter)
	if err != nil {
		return nil, fmt.Errorf("failed to get presubmits: %w", err)
	}

	requireLabels := sets.NewString()
	for l, info := range pr.Labels {
		if !info.Optional {
			requireLabels.Insert(l)
		}
	}

	// generate required and optional entries for Prow Jobs
	for _, pj := range presubmits {
		if !pj.CouldRun(branch) {
			continue
		}

		var isJobRequired bool
		if val, ok := pj.Labels[kube.GerritReportLabel]; ok && requireLabels.Has(val) {
			isJobRequired = true
		}

		if isJobRequired {
			if pj.TriggersConditionally() {
				// jobs that trigger conditionally are required if present.
				requiredIfPresent.Insert(pj.Context)
			} else {
				// jobs that produce required contexts and will
				// always run should be required at all times
				required.Insert(pj.Context)
			}
		} else {
			optional.Insert(pj.Context)
		}
	}

	t := &config.TideContextPolicy{
		RequiredContexts:          required.List(),
		RequiredIfPresentContexts: requiredIfPresent.List(),
		OptionalContexts:          optional.List(),
	}
	if err := t.Validate(); err != nil {
		return t, err
	}
	return t, nil
}

func (p *GerritProvider) prMergeMethod(crc *CodeReviewCommon) (types.PullRequestMergeType, error) {
	var res types.PullRequestMergeType
	pr := crc.Gerrit
	if pr == nil {
		return res, errors.New("programmer error: crc.Gerrit cannot be nil for GerritProvider")
	}

	// Translate merge methods to types that git could understand. The merge
	// methods for gerrit are documented at
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

	return res, nil
}
