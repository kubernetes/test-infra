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

// Package reporter implements a reporter interface for gerrit
package gerrit

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/andygrunwald/go-gerrit"
	"github.com/sirupsen/logrus"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/crier/reporters/criercommonlib"
	"k8s.io/test-infra/prow/gerrit/client"
	"k8s.io/test-infra/prow/kube"
)

const (
	cross      = "❌"
	tick       = "✔️"
	hourglass  = "⏳"
	prohibited = "🚫"

	defaultProwHeader         = "Prow Status:"
	jobReportFormat           = "%s %s %s - %s\n"
	jobReportFormatWithoutURL = "%s %s %s\n"
	errorLinePrefix           = "NOTE FROM PROW"

	// lgtm means all presubmits passed, but need someone else to approve before merge (looks good to me).
	lgtm = "+1"
	// lbtm means some presubmits failed, perfer not merge (looks bad to me).
	lbtm = "-1"
	// lztm is the minimum score for a postsubmit.
	lztm = "0"
	// codeReview is the default gerrit code review label
	codeReview = client.CodeReview
	// maxCommentSizeLimit is from
	// http://gerrit-documentation.storage.googleapis.com/Documentation/3.2.0/config-gerrit.html#change.commentSizeLimit,
	// if a comment is 16000 chars it's almost not readable any way, let's not
	// use all of the space, picking 80% as a heuristic number here
	maxCommentSizeLimit = 14400
)

var (
	stateIcon = map[v1.ProwJobState]string{
		v1.PendingState:   hourglass,
		v1.TriggeredState: hourglass,
		v1.SuccessState:   tick,
		v1.FailureState:   cross,
		v1.AbortedState:   prohibited,
	}
)

type gerritClient interface {
	SetReview(instance, id, revision, message string, labels map[string]string) error
	GetChange(instance, id string) (*gerrit.ChangeInfo, error)
}

// Client is a gerrit reporter client
type Client struct {
	gc          gerritClient
	pjclientset ctrlruntimeclient.Client
	prLocks     *criercommonlib.ShardedLock
}

// Job is the view of a prowjob scoped for a report
type Job struct {
	Name  string
	State v1.ProwJobState
	Icon  string
	URL   string
}

// JobReport is the structured job report format
type JobReport struct {
	Jobs    []Job
	Success int
	Total   int
	Message string
	Header  string
}

// NewReporter returns a reporter client
func NewReporter(cfg config.Getter, cookiefilePath string, projects map[string][]string, pjclientset ctrlruntimeclient.Client) (*Client, error) {
	gc, err := client.NewClient(projects)
	if err != nil {
		return nil, err
	}
	// applyGlobalConfig reads gerrit configurations from global gerrit config,
	// it will completely override previously configured gerrit hosts and projects.
	// it will also by the way authenticate gerrit
	applyGlobalConfig(cfg, gc, cookiefilePath)

	// Authenticate creates a goroutine for rotating token secrets when called the first
	// time, afterwards it only authenticate once.
	// applyGlobalConfig calls authenticate only when global gerrit config presents,
	// call it here is required for cases where gerrit repos are defined as command
	// line arg(which is going to be deprecated).
	gc.Authenticate(cookiefilePath, "")

	c := &Client{
		gc:          gc,
		pjclientset: pjclientset,
		prLocks:     criercommonlib.NewShardedLock(),
	}

	c.prLocks.RunCleanup()
	return c, nil
}

func applyGlobalConfig(cfg config.Getter, gerritClient *client.Client, cookiefilePath string) {
	applyGlobalConfigOnce(cfg, gerritClient, cookiefilePath)

	go func() {
		for {
			applyGlobalConfigOnce(cfg, gerritClient, cookiefilePath)
			// No need to spin constantly, give it a break. It's ok that config change has one second delay.
			time.Sleep(time.Second)
		}
	}()
}

func applyGlobalConfigOnce(cfg config.Getter, gerritClient *client.Client, cookiefilePath string) {
	orgReposConfig := cfg().Gerrit.OrgReposConfig
	if orgReposConfig == nil {
		return
	}
	// Updates clients based on global gerrit config.
	gerritClient.UpdateClients(orgReposConfig.AllRepos())
	// Authenticate creates a goroutine for rotating token secrets when called the first
	// time, afterwards it only authenticate once.
	// Newly added orgs/repos are only authenticated by the goroutine when token secret is
	// rotated, which is up to 1 hour after config change. Explicitly call Authenticate
	// here to get them authenticated immediately.
	gerritClient.Authenticate(cookiefilePath, "")
}

// GetName returns the name of the reporter
func (c *Client) GetName() string {
	return "gerrit-reporter"
}

// ShouldReport returns if this prowjob should be reported by the gerrit reporter
func (c *Client) ShouldReport(ctx context.Context, log *logrus.Entry, pj *v1.ProwJob) bool {
	if !pj.Spec.Report {
		return false
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if pj.Status.State == v1.TriggeredState || pj.Status.State == v1.PendingState {
		// not done yet
		log.Info("PJ not finished")
		return false
	}

	if pj.Status.State == v1.AbortedState {
		// aborted (new patchset)
		log.Info("PJ aborted")
		return false
	}

	// has gerrit metadata (scheduled by gerrit adapter)
	if pj.ObjectMeta.Annotations[client.GerritID] == "" ||
		pj.ObjectMeta.Annotations[client.GerritInstance] == "" ||
		pj.ObjectMeta.Labels[client.GerritRevision] == "" {
		log.Info("Not a gerrit job")
		return false
	}

	// Don't wait for report aggregation if not voting on any label
	if pj.ObjectMeta.Labels[client.GerritReportLabel] == "" {
		return true
	}

	// allPJsAgreeToReport is a helper function that queries all prowjobs based
	// on provided labels and run each one through singlePJAgreeToReport,
	// returns false if any of the prowjob doesn't agree.
	allPJsAgreeToReport := func(labels []string, singlePJAgreeToReport func(pj *v1.ProwJob) bool) bool {
		selector := map[string]string{}
		for _, l := range labels {
			selector[l] = pj.ObjectMeta.Labels[l]
		}

		var pjs v1.ProwJobList
		if err := c.pjclientset.List(ctx, &pjs, ctrlruntimeclient.MatchingLabels(selector)); err != nil {
			log.WithError(err).Errorf("Cannot list prowjob with selector %v", selector)
			return false
		}

		for _, pjob := range pjs.Items {
			if !singlePJAgreeToReport(&pjob) {
				return false
			}
		}

		return true
	}

	// patchsetNumFromPJ converts value of "prow.k8s.io/gerrit-patchset" to
	// integer, the value is used for evaluating whether a newer patchset for
	// current CR was already established. It may accidentally omit reporting if
	// current prowjob doesn't have this label or has an invalid value, this
	// will be reflected as warning message in prow.
	patchsetNumFromPJ := func(pj *v1.ProwJob) int {
		ps, ok := pj.ObjectMeta.Labels[client.GerritPatchset]
		if !ok {
			log.Warnf("Label %s not found in prowjob %s", client.GerritPatchset, pj.Name)
			return -1
		}
		intPs, err := strconv.Atoi(ps)
		if err != nil {
			log.Warnf("Found non integer label for %s: %s in prowjob %s", client.GerritPatchset, ps, pj.Name)
			return -1
		}
		return intPs
	}

	// Get patchset number from current pj.
	patchsetNum := patchsetNumFromPJ(pj)

	// Check all other prowjobs to see whether they agree or not
	return allPJsAgreeToReport([]string{client.GerritRevision, kube.ProwJobTypeLabel, client.GerritReportLabel}, func(otherPj *v1.ProwJob) bool {
		if otherPj.Status.State == v1.TriggeredState || otherPj.Status.State == v1.PendingState {
			// other jobs with same label are still running on this revision, skip report
			log.Info("Other jobs with same label are still running on this revision")
			return false
		}
		return true
	}) && allPJsAgreeToReport([]string{kube.OrgLabel, kube.RepoLabel, kube.PullLabel}, func(otherPj *v1.ProwJob) bool {
		// This job has duplicate(s) and there are newer one(s)
		if otherPj.Spec.Job == pj.Spec.Job && otherPj.CreationTimestamp.After(pj.CreationTimestamp.Time) {
			return false
		}
		// Newer patchset exists, skip report
		return patchsetNumFromPJ(otherPj) <= patchsetNum
	})
}

// Report will send the current prowjob status as a gerrit review
func (c *Client) Report(ctx context.Context, logger *logrus.Entry, pj *v1.ProwJob) ([]*v1.ProwJob, *reconcile.Result, error) {
	logger = logger.WithFields(logrus.Fields{"job": pj.Spec.Job, "name": pj.Name})

	// Gerrit reporter hasn't learned how to deduplicate itself from report yet,
	// will need to block here. Unfortunately need to check after this section
	// to ensure that the job was not already marked reported by other threads
	// TODO(chaodaiG): postsubmit job technically doesn't know which PR it's
	// from, currently it's associated with a PR in gerrit in a weird way, which
	// needs to be fixed in
	// https://github.com/kubernetes/test-infra/issues/22653, remove the
	// PostsubmitJob check once it's fixed
	if pj.Spec.Type == v1.PresubmitJob || pj.Spec.Type == v1.PostsubmitJob {
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

		// In the case where several prow jobs from the same PR are finished one
		// after another, by the time the lock is acquired, this job might have
		// already been reported by another worker, refetch this pj to make sure
		// that no duplicate report is produced
		pjObjKey := ctrlruntimeclient.ObjectKeyFromObject(pj)
		if err := c.pjclientset.Get(ctx, pjObjKey, pj); err != nil {
			if apierrors.IsNotFound(err) {
				// Job could be GC'ed or deleted for other reasons, not to
				// report, this is not a prow error and should not be retried
				logger.Debug("object no longer exist")
				return nil, nil, nil
			}

			return nil, nil, fmt.Errorf("failed to get prowjob %s: %w", pjObjKey.String(), err)
		}
		if pj.Status.PrevReportStates[c.GetName()] == pj.Status.State {
			logger.Info("Already reported by other threads.")
			return nil, nil, nil
		}
	}

	newCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	clientGerritRevision := client.GerritRevision
	clientGerritID := client.GerritID
	clientGerritInstance := client.GerritInstance
	pjTypeLabel := kube.ProwJobTypeLabel
	gerritReportLabel := client.GerritReportLabel

	var pjsOnRevisionWithSameLabel v1.ProwJobList
	var toReportJobs []*v1.ProwJob
	if pj.ObjectMeta.Labels[gerritReportLabel] == "" && pj.Status.State != v1.AbortedState {
		toReportJobs = append(toReportJobs, pj)
	} else { // generate an aggregated report

		// list all prowjobs in the patchset matching pj's type (pre- or post-submit)
		selector := map[string]string{
			clientGerritRevision: pj.ObjectMeta.Labels[clientGerritRevision],
			pjTypeLabel:          pj.ObjectMeta.Labels[pjTypeLabel],
			gerritReportLabel:    pj.ObjectMeta.Labels[gerritReportLabel],
		}

		if err := c.pjclientset.List(newCtx, &pjsOnRevisionWithSameLabel, ctrlruntimeclient.MatchingLabels(selector)); err != nil {
			logger.WithError(err).WithField("selector", selector).Errorf("Cannot list prowjob with selector")
			return nil, nil, err
		}

		mostRecentJob := map[string]*v1.ProwJob{}
		for idx, pjOnRevisionWithSameLabel := range pjsOnRevisionWithSameLabel.Items {
			job, ok := mostRecentJob[pjOnRevisionWithSameLabel.Spec.Job]
			if !ok || job.CreationTimestamp.Time.Before(pjOnRevisionWithSameLabel.CreationTimestamp.Time) {
				mostRecentJob[pjOnRevisionWithSameLabel.Spec.Job] = &pjsOnRevisionWithSameLabel.Items[idx]
			}
		}
		for _, pjOnRevisionWithSameLabel := range mostRecentJob {
			toReportJobs = append(toReportJobs, pjOnRevisionWithSameLabel)
		}
	}
	report := GenerateReport(toReportJobs, 0)
	message := report.Header + report.Message
	// report back
	gerritID := pj.ObjectMeta.Annotations[clientGerritID]
	gerritInstance := pj.ObjectMeta.Annotations[clientGerritInstance]
	gerritRevision := pj.ObjectMeta.Labels[clientGerritRevision]
	var reportLabel string
	if val, ok := pj.ObjectMeta.Labels[client.GerritReportLabel]; ok {
		reportLabel = val
	} else {
		reportLabel = codeReview
	}

	if report.Total <= 0 {
		// Shouldn't happen but return if does
		logger.Warn("Tried to report empty jobs.")
		return nil, nil, nil
	}
	var reviewLabels map[string]string
	if reportLabel != "" {
		var vote string
		// Can only vote below zero before merge
		// TODO(fejta): cannot vote below previous vote after merge
		switch {
		case report.Success == report.Total:
			vote = lgtm
		case pj.Spec.Type == v1.PresubmitJob:
			//https://gerrit-documentation.storage.googleapis.com/Documentation/3.1.4/config-labels.html#label_allowPostSubmit
			// If presubmit and failure vote -1...
			vote = lbtm

			change, err := c.gc.GetChange(gerritInstance, gerritID)
			//TODO(mpherman): In cases where the change was deleted we do not want warn nor report
			if err != nil {
				logger.WithError(err).Warnf("Unable to get change from instance %s with id %s", gerritInstance, gerritID)
			} else if change.Status == client.Merged {
				// Unless change is already merged. Merged changes should not be voted <0
				vote = lztm
			}
		default:
			vote = lztm
		}
		reviewLabels = map[string]string{reportLabel: vote}
	}

	logger.Infof("Reporting to instance %s on id %s with message %s", gerritInstance, gerritID, message)
	if err := c.gc.SetReview(gerritInstance, gerritID, gerritRevision, message, reviewLabels); err != nil {
		logger.WithError(err).WithField("gerrit_id", gerritID).WithField("label", reportLabel).Info("Failed to set review.")

		if reportLabel == "" {
			return nil, nil, err
		}
		// Retry without voting on a label
		message := fmt.Sprintf("[NOTICE]: Prow Bot cannot access %s label!\n%s", reportLabel, message)
		if err := c.gc.SetReview(gerritInstance, gerritID, gerritRevision, message, nil); err != nil {
			logger.WithError(err).WithField("gerrit_id", gerritID).Errorf("Failed to set plain review on change ID.")
			return nil, nil, err
		}
	}

	logger.Infof("Review Complete, reported jobs: %s", jobNames(toReportJobs))

	// If return here, the shardedLock will be released, and other threads that
	// are from the same PR will still not understand that it's already
	// reported, as the change of previous report state happens only after the
	// returning of current function from the caller.
	// Ideally the previous report state should be changed here.
	// This operation takes a long time when there are a lot of jobs
	// in the batch, so we are creating a new context.
	loopCtx, loopCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer loopCancel()
	logger.WithFields(logrus.Fields{
		"job-count":      len(toReportJobs),
		"all-jobs-count": len(pjsOnRevisionWithSameLabel.Items),
	}).Info("Reported job(s), now will update pj(s).")
	var err error
	// All latest jobs for this label were already reported, none of the jobs
	// for this label are worthy reporting any more. Mark all of them as
	// reported to avoid corner cases where an older job finished later, and the
	// newer prowjobs CRD was somehow missing from the cluster.
	for _, pjob := range pjsOnRevisionWithSameLabel.Items {
		if pjob.Status.State == v1.AbortedState || pjob.Status.PrevReportStates[c.GetName()] == pjob.Status.State {
			continue
		}
		if err = criercommonlib.UpdateReportStateWithRetries(loopCtx, &pjob, logger, c.pjclientset, c.GetName()); err != nil {
			logger.WithError(err).Error("Failed to update report state on prowjob")
		}
	}

	// Let caller know that we are done with this job.
	return nil, nil, err
}

func jobNames(jobs []*v1.ProwJob) []string {
	names := make([]string, len(jobs))
	for i, job := range jobs {
		names[i] = fmt.Sprintf("%s, %s", job.Spec.Job, job.Name)
	}
	return names
}

func statusIcon(state v1.ProwJobState) string {
	icon, ok := stateIcon[state]
	if !ok {
		return prohibited
	}
	return icon
}

func jobFromPJ(pj *v1.ProwJob) Job {
	return Job{Name: pj.Spec.Job, State: pj.Status.State, Icon: statusIcon(pj.Status.State), URL: pj.Status.URL}
}

func (j *Job) serializeWithoutURL() string {
	return fmt.Sprintf(jobReportFormatWithoutURL, j.Icon, j.Name, strings.ToUpper(string(j.State)))
}

func (j *Job) serialize() string {
	return fmt.Sprintf(jobReportFormat, j.Icon, j.Name, strings.ToUpper(string(j.State)), j.URL)
}

func deserializeWithoutURL(s string, j *Job) error {
	var state string
	n, err := fmt.Sscanf(s, jobReportFormatWithoutURL, &j.Icon, &j.Name, &state)
	if err != nil {
		return err
	}
	j.State = v1.ProwJobState(strings.ToLower(state))
	const want = 3
	if n != want {
		return fmt.Errorf("scan: got %d, want %d", n, want)
	}
	return nil
}

func deserialize(s string, j *Job) error {
	var state string
	n, err := fmt.Sscanf(s, jobReportFormat, &j.Icon, &j.Name, &state, &j.URL)
	if err != nil {
		return err
	}
	j.State = v1.ProwJobState(strings.ToLower(state))
	const want = 4
	if n != want {
		return fmt.Errorf("scan: got %d, want %d", n, want)
	}
	return nil
}

func errorMessageLine(s string) string {
	return fmt.Sprintf("[%s: %s]", errorLinePrefix, s)
}

func isErrorMessageLine(s string) bool {
	return strings.HasPrefix(s, fmt.Sprintf("[%s: ", errorLinePrefix))
}

// GenerateReport generates report header and message based on pjs passed in. As
// URLs are very long string, includes them in the report would easily make the
// report exceed the size limit of 14400.
// Unfortunately we need all prowjobs info for /retest to work, which is by far
// the most reliable way of retrieving prow jobs results, as prow jobs
// custom resources are GCed by sinker after max_pod_age, which normally is 48
// hours. So to ensure that all prow jobs results are displayed, URLs for some
// of the jobs are omitted from this report.
// commentSizeLimit is used to make sure that the report generated won't exceed
func GenerateReport(pjs []*v1.ProwJob, commentSizeLimit int) JobReport {
	if commentSizeLimit == 0 {
		commentSizeLimit = maxCommentSizeLimit
	}
	report := JobReport{Total: len(pjs)}
	for _, pj := range pjs {
		job := jobFromPJ(pj)
		report.Jobs = append(report.Jobs, job)
		if pj.Status.State == v1.SuccessState {
			report.Success++
		}
	}

	fullHeader := func(header, reTestMessage string) string {
		return fmt.Sprintf("%s%s\n", header, reTestMessage)
	}

	report.Header = fmt.Sprintf("%s %d out of %d pjs passed!", defaultProwHeader, report.Success, report.Total)
	var reTestMessage string
	if report.Success < report.Total {
		reTestMessage = " Comment '/retest' to rerun all failed tests"
	}

	// Sort first so that failed jobs always on top
	sort.Slice(report.Jobs, func(i, j int) bool {
		for _, state := range []v1.ProwJobState{
			v1.FailureState,
			v1.ErrorState,
			v1.AbortedState,
		} {
			if report.Jobs[i].State == state {
				return true
			}
			if report.Jobs[j].State == state {
				return false
			}
		}
		// We don't care the orders of the following states, so keep original order
		return true
	})

	remainingSize := commentSizeLimit - len(fullHeader(report.Header, reTestMessage))
	linesWithURLs := make([]string, len(report.Jobs))
	linesWithoutURLs := make([]string, len(report.Jobs))
	for i, job := range report.Jobs {
		linesWithURLs[i] = job.serialize()
		remainingSize -= len(linesWithURLs[i])
	}

	// cutoff is the index where if it's the line contains URL it will exceed the
	// size limit.
	cutoff := len(report.Jobs)
	for cutoff > 0 && remainingSize < 0 {
		cutoff--
		linesWithoutURLs[cutoff] = report.Jobs[cutoff].serializeWithoutURL()
		// remainingSize >= 0 after this condition means next line is not good
		// to include URL
		remainingSize += len(linesWithURLs[cutoff]) - len(linesWithoutURLs[cutoff])
	}

	// This shouldn't happen unless there are too many prow jobs(e.g. > 300) and
	// each job name is super long(e.g. > 50)
	if remainingSize < 0 {
		report.Header = fullHeader(report.Header, " Comment '/test all' to rerun all tests")
		report.Message = errorMessageLine("Prow failed to report all jobs, are there excessive amount of prow jobs?")
		return report
	}

	// Now that we know a cutoff between long and short lines, assemble them
	for i := range report.Jobs {
		if i < cutoff {
			report.Message += linesWithURLs[i]
		} else {
			report.Message += linesWithoutURLs[i]
		}
	}

	if cutoff < len(report.Jobs) {
		// Note that this makes the comment longer, but since the size limit of
		// 14400 is conservative, we should be fine
		report.Message += errorMessageLine(fmt.Sprintf("Skipped displaying URLs for %d/%d jobs due to reaching gerrit comment size limit", len(report.Jobs)-cutoff, len(report.Jobs)))
	}

	report.Header = fullHeader(report.Header, reTestMessage)
	return report
}

// ParseReport creates a jobReport from a string, nil if cannot parse
func ParseReport(message string) *JobReport {
	contents := strings.Split(message, "\n")
	start := 0
	isReport := false
	for start < len(contents) {
		if strings.HasPrefix(contents[start], defaultProwHeader) {
			isReport = true
			break
		}
		start++
	}
	if !isReport {
		return nil
	}
	var report JobReport
	report.Header = contents[start] + "\n"
	for i := start + 1; i < len(contents); i++ {
		if contents[i] == "" || isErrorMessageLine(contents[i]) {
			continue
		}
		var j Job
		if err := deserialize(contents[i], &j); err != nil {
			// Will also need to support reports without URL
			if err = deserializeWithoutURL(contents[i], &j); err != nil {
				logrus.Warnf("Could not deserialize %s to a job: %v", contents[i], err)
				continue
			}
		}
		report.Total++
		if j.State == v1.SuccessState {
			report.Success++
		}
		report.Jobs = append(report.Jobs, j)
	}
	report.Message = strings.TrimPrefix(message, report.Header+"\n")
	return &report
}

// String implements Stringer for JobReport
func (r JobReport) String() string {
	return fmt.Sprintf("%s\n%s", r.Header, r.Message)
}

func lockKeyForPJ(pj *v1.ProwJob) (*criercommonlib.SimplePull, error) {
	// TODO(chaodaiG): remove postsubmit once
	// https://github.com/kubernetes/test-infra/issues/22653 is fixed
	if pj.Spec.Type != v1.PresubmitJob && pj.Spec.Type != v1.PostsubmitJob {
		return nil, fmt.Errorf("can only get lock key for presubmit and postsubmit jobs, was %q", pj.Spec.Type)
	}
	if pj.Spec.Refs == nil {
		return nil, errors.New("pj.Spec.Refs is nil")
	}
	if n := len(pj.Spec.Refs.Pulls); n != 1 {
		return nil, fmt.Errorf("prowjob doesn't have one but %d pulls", n)
	}
	return criercommonlib.NewSimplePull(pj.Spec.Refs.Org, pj.Spec.Refs.Repo, pj.Spec.Refs.Pulls[0].Number), nil
}
