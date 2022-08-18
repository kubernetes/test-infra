/*
Copyright 2018 The Kubernetes Authors.

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

// Package reporter implements a reporter interface for github
// TODO(krzyzacy): move logic from report.go here
package github

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/crier/reporters/criercommonlib"
	"k8s.io/test-infra/prow/github/report"
	"k8s.io/test-infra/prow/kube"
)

const (
	// GitHubReporterName is the name for github reporter
	GitHubReporterName = "github-reporter"
)

// Client is a github reporter client
type Client struct {
	gc          report.GitHubClient
	config      config.Getter
	reportAgent v1.ProwJobAgent
	prLocks     *criercommonlib.ShardedLock
	lister      ctrlruntimeclient.Reader
}

// NewReporter returns a reporter client
func NewReporter(gc report.GitHubClient, cfg config.Getter, reportAgent v1.ProwJobAgent, lister ctrlruntimeclient.Reader) *Client {
	c := &Client{
		gc:          gc,
		config:      cfg,
		reportAgent: reportAgent,
		prLocks:     criercommonlib.NewShardedLock(),
		lister:      lister,
	}
	c.prLocks.RunCleanup()
	return c
}

// GetName returns the name of the reporter
func (c *Client) GetName() string {
	return GitHubReporterName
}

// ShouldReport returns if this prowjob should be reported by the github reporter
func (c *Client) ShouldReport(_ context.Context, _ *logrus.Entry, pj *v1.ProwJob) bool {
	if !pj.Spec.Report {
		return false
	}

	switch {
	case pj.Labels[kube.GerritReportLabel] != "":
		return false // TODO(fejta): opt-in to github reporting
	case pj.Spec.Type != v1.PresubmitJob && pj.Spec.Type != v1.PostsubmitJob:
		return false // Report presubmit and postsubmit github jobs for github reporter
	case c.reportAgent != "" && pj.Spec.Agent != c.reportAgent:
		return false // Only report for specified agent
	}

	return true
}

// Report will report via reportlib
func (c *Client) Report(ctx context.Context, log *logrus.Entry, pj *v1.ProwJob) ([]*v1.ProwJob, *reconcile.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// TODO(krzyzacy): ditch ReportTemplate, and we can drop reference to config.Getter
	err := report.ReportStatusContext(ctx, c.gc, *pj, c.config().GitHubReporter)
	if err != nil {
		if strings.Contains(err.Error(), "This SHA and context has reached the maximum number of statuses") {
			// This is completely unrecoverable, so just swallow the error to make sure we wont retry, even when crier gets restarted.
			log.WithError(err).Debug("Encountered an error, skipping retries")
			err = nil
		} else if strings.Contains(err.Error(), "\"message\":\"Not Found\"") || strings.Contains(err.Error(), "\"message\":\"No commit found for SHA:") {
			// "message":"Not Found" error occurs when someone force push, which is not a crier error
			log.WithError(err).Debug("Could not find PR commit, skipping retries")
			err = nil
		}
		// Always return when there is any error reporting status context.
		return []*v1.ProwJob{pj}, nil, err
	}

	// The github comment create/update/delete done for presubmits
	// needs pr-level locking to avoid racing when reporting multiple
	// jobs in parallel.
	if pj.Spec.Type == v1.PresubmitJob {
		key, err := lockKeyForPJ(pj)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get lockkey for job: %w", err)
		}
		lock, err := c.prLocks.GetLock(ctx, *key)
		if err != nil {
			return nil, nil, err
		}
		if err := lock.Acquire(ctx, 1); err != nil {
			return nil, nil, err
		}
		defer lock.Release(1)
	}

	// Check if this org or repo has opted out of failure report comments.
	// This check has to be here and not in ShouldReport as we always need to report
	// the status context, just potentially not creating a comment.
	refs := pj.Spec.Refs
	fullRepo := fmt.Sprintf("%s/%s", refs.Org, refs.Repo)
	for _, ident := range c.config().GitHubReporter.NoCommentRepos {
		if refs.Org == ident || fullRepo == ident {
			return []*v1.ProwJob{pj}, nil, nil
		}
	}
	// Check if this org or repo has opted out of failure report comments
	toReport := []v1.ProwJob{*pj}
	var mustCreateComment bool
	for _, ident := range c.config().GitHubReporter.SummaryCommentRepos {
		if pj.Spec.Refs.Org == ident || fullRepo == ident {
			mustCreateComment = true
			toReport, err = pjsToReport(ctx, log, c.lister, pj)
			if err != nil {
				return []*v1.ProwJob{pj}, nil, err
			}
		}
	}
	err = report.ReportComment(ctx, c.gc, c.config().Plank.ReportTemplateForRepo(pj.Spec.Refs), toReport, c.config().GitHubReporter, mustCreateComment)

	return []*v1.ProwJob{pj}, nil, err
}

func pjsToReport(ctx context.Context, log *logrus.Entry, lister ctrlruntimeclient.Reader, pj *v1.ProwJob) ([]v1.ProwJob, error) {
	if len(pj.Spec.Refs.Pulls) != 1 {
		return nil, nil
	}
	// find all prowjobs from this PR
	selector := map[string]string{}
	for _, l := range []string{kube.OrgLabel, kube.RepoLabel, kube.PullLabel} {
		selector[l] = pj.ObjectMeta.Labels[l]
	}
	var pjs v1.ProwJobList
	if err := lister.List(ctx, &pjs, ctrlruntimeclient.MatchingLabels(selector)); err != nil {
		return nil, fmt.Errorf("Cannot list prowjob with selector %v", selector)
	}

	latestBatch := make(map[string]v1.ProwJob)
	for _, pjob := range pjs.Items {
		if !pjob.Complete() { // Any job still running should prevent from comments
			return nil, nil
		}
		if !pjob.Spec.Report { // Filtering out non-reporting jobs
			continue
		}
		// Now you have convinced me that you are the same job from my revision,
		// continue convince me that you are the last one of your kind
		if existing, ok := latestBatch[pjob.Spec.Job]; !ok {
			latestBatch[pjob.Spec.Job] = pjob
		} else if pjob.CreationTimestamp.After(existing.CreationTimestamp.Time) {
			latestBatch[pjob.Spec.Job] = pjob
		}
	}

	var toReport []v1.ProwJob
	for _, pjob := range latestBatch {
		toReport = append(toReport, pjob)
	}

	return toReport, nil
}

func lockKeyForPJ(pj *v1.ProwJob) (*criercommonlib.SimplePull, error) {
	if pj.Spec.Type != v1.PresubmitJob {
		return nil, fmt.Errorf("can only get lock key for presubmit jobs, was %q", pj.Spec.Type)
	}
	if pj.Spec.Refs == nil {
		return nil, errors.New("pj.Spec.Refs is nil")
	}
	if n := len(pj.Spec.Refs.Pulls); n != 1 {
		return nil, fmt.Errorf("prowjob doesn't have one but %d pulls", n)
	}
	return criercommonlib.NewSimplePull(pj.Spec.Refs.Org, pj.Spec.Refs.Repo, pj.Spec.Refs.Pulls[0].Number), nil
}
