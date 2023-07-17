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
	"regexp"
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

	defaultProwHeader               = "Prow Status:"
	jobReportFormat                 = "%s [%s](%s) %s\n"
	jobReportFormatUrlNotFound      = "%s %s (URL_NOT_FOUND) %s\n"
	jobReportFormatWithoutURL       = "%s %s %s\n"
	jobReportFormatLegacyRegex      = `^(\S+) (\S+) (\S+) - (\S+)$`
	jobReportFormatRegex            = `^(\S+) \[(\S+)\]\((\S+)\) (\S+)$`
	jobReportFormatUrlNotFoundRegex = `^(\S+) (\S+) \(URL_NOT_FOUND\) (\S+)$`
	jobReportFormatWithoutURLRegex  = `^(\S+) (\S+) (\S+)$`
	errorLinePrefix                 = "NOTE FROM PROW"
	// jobReportHeader expects 4 args. {defaultProwHeader}, {jobs-passed},
	// {jobs-total}, {additional-text(optional)}.
	jobReportHeader = "%s %d out of %d pjs passed! 👉 Comment `/retest` to rerun only failed tests (if any), or `/test all` to rerun all tests.%s\n"

	// lgtm means all presubmits passed, but need someone else to approve before merge (looks good to me).
	lgtm = "+1"
	// lbtm means some presubmits failed, perfer not merge (looks bad to me).
	lbtm = "-1"
	// lztm is the minimum score for a postsubmit.
	lztm = "0"
	// codeReview is the default gerrit code review label
	codeReview = client.CodeReview
	// maxCommentSizeLimit is from
	// http://gerrit-documentation.storage.googleapis.com/Documentation/3.2.0/config-gerrit.html#change.commentSizeLimit, where it says:
	//
	//    Maximum allowed size in characters of a regular (non-robot) comment.
	//    Comments which exceed this size will be rejected. Size computation is
	//    approximate and may be off by roughly 1%. Common unit suffixes of 'k',
	//    'm', or 'g' are supported. The value must be positive.
	//
	//    The default limit is 16kiB.
	//
	// 16KiB = 16*1024 bytes. Note that the size computation is stated as
	// **approximate** and can be off by about 1%. To be safe, we use 15*1024 or
	// 93.75% of the default 16KiB limit. This value is lower than the limit by
	// 6.25% to be 6x below the ~1% margin of error described by the Gerrit
	// docs.
	//
	// Even assuming that the docs have their units wrong (maybe they actually
	// mean 16KB = 16000, not 16KiB), the new value of (15*1024)/16000 = 0.96,
	// or to be 4% less than the theoretical maximum, which is still a
	// conservative figure.
	maxCommentSizeLimit = 15 * 1024
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
	GetChange(instance, id string, additionalFields ...string) (*gerrit.ChangeInfo, error)
	ChangeExist(instance, id string) (bool, error)
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
func NewReporter(orgRepoConfigGetter func() *config.GerritOrgRepoConfigs, cookiefilePath string, pjclientset ctrlruntimeclient.Client, maxQPS, maxBurst int) (*Client, error) {
	// Initialize an empty client, the orgs/repos will be filled in by
	// ApplyGlobalConfig later.
	gc, err := client.NewClient(nil, maxQPS, maxBurst)
	if err != nil {
		return nil, err
	}
	// applyGlobalConfig reads gerrit configurations from global gerrit config,
	// it will completely override previously configured gerrit hosts and projects.
	// it will also by the way authenticate gerrit
	gc.ApplyGlobalConfig(orgRepoConfigGetter, nil, cookiefilePath, "", func() {})

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
	if pj.ObjectMeta.Annotations[kube.GerritID] == "" ||
		pj.ObjectMeta.Annotations[kube.GerritInstance] == "" ||
		pj.ObjectMeta.Labels[kube.GerritRevision] == "" {
		log.Info("Not a gerrit job")
		return false
	}

	// Don't wait for report aggregation if not voting on any label
	if pj.ObjectMeta.Labels[kube.GerritReportLabel] == "" {
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
		log := log.WithFields(logrus.Fields{"label": kube.GerritPatchset, "job": pj.Name})
		ps, ok := pj.ObjectMeta.Labels[kube.GerritPatchset]
		if !ok {
			// This label exists only in jobs that are created by Gerrit. For jobs that are
			// created by Pubsub it's entirely up to the users.
			log.Debug("Label not found in prowjob.")
			return -1
		}
		intPs, err := strconv.Atoi(ps)
		if err != nil {
			log.Debug("Found non integer label value in prowjob.")
			return -1
		}
		return intPs
	}

	// Get patchset number from current pj.
	patchsetNum := patchsetNumFromPJ(pj)

	// Check all other prowjobs to see whether they agree or not
	return allPJsAgreeToReport([]string{kube.GerritRevision, kube.ProwJobTypeLabel, kube.GerritReportLabel}, func(otherPj *v1.ProwJob) bool {
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

	clientGerritRevision := kube.GerritRevision
	clientGerritID := kube.GerritID
	clientGerritInstance := kube.GerritInstance
	pjTypeLabel := kube.ProwJobTypeLabel
	gerritReportLabel := kube.GerritReportLabel

	var pjsOnRevisionWithSameLabel v1.ProwJobList
	var pjsToUpdateState []v1.ProwJob
	var toReportJobs []*v1.ProwJob
	if pj.ObjectMeta.Labels[gerritReportLabel] == "" && pj.Status.State != v1.AbortedState {
		toReportJobs = append(toReportJobs, pj)
		pjsToUpdateState = []v1.ProwJob{*pj}
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
			pjsToUpdateState = append(pjsToUpdateState, pjOnRevisionWithSameLabel)
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
	logger = logger.WithFields(logrus.Fields{
		"instance": gerritInstance,
		"id":       gerritID,
	})
	var reportLabel string
	if val, ok := pj.ObjectMeta.Labels[kube.GerritReportLabel]; ok {
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
	var change *gerrit.ChangeInfo
	var err error
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

			change, err = c.gc.GetChange(gerritInstance, gerritID)
			if err != nil {
				exist, existErr := c.gc.ChangeExist(gerritInstance, gerritID)
				if existErr == nil && !exist {
					// PR was deleted, no reason to report or retry
					logger.WithError(err).Info("Change doesn't exist any more, skip reporting.")
					return nil, nil, nil
				}
				logger.WithError(err).Warn("Unable to get change")
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

		// It could be that the commit is deleted by the time we want to report.
		// Swollow the error if this is the case.
		exist, existErr := c.gc.ChangeExist(gerritInstance, gerritID)
		if existErr == nil {
			if !exist {
				// PR was deleted, no reason to report or retry
				logger.WithError(err).Info("Change doesn't exist any more, skip reporting.")
				return nil, nil, nil
			}
			if change == nil {
				var debugErr error
				change, debugErr = c.gc.GetChange(gerritInstance, gerritID)
				if debugErr != nil {
					logger.WithError(debugErr).WithField("gerrit_id", gerritID).Info("Getting change failed. This is trying to help determine why SetReview failed.")
				}
			}
		} else {
			// Checking change exist error is not as useful as the error from
			// SetReview, log it on debug level
			logger.WithError(existErr).Debug("Failed checking existence of change.")
		}
		if change != nil {
			// keys of `Revisions` are the revision strings, see
			// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#change-info
			if _, ok := change.Revisions[gerritRevision]; !ok {
				logger.WithFields(logrus.Fields{"gerrit_id": gerritID, "revision": gerritRevision}).Info("The revision to be commented is missing, swallow error.")
				// still want the rest of the function continue, so that all
				// jobs for this revision are marked reported.
				err = nil
			}
		}

		if err != nil {
			if reportLabel == "" {
				return nil, nil, err
			}
			// Retry without voting on a label
			message := fmt.Sprintf("[NOTICE]: Prow Bot cannot access %s label!\n%s", reportLabel, message)
			if err := c.gc.SetReview(gerritInstance, gerritID, gerritRevision, message, nil); err != nil {
				return nil, nil, err
			}
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
		"all-jobs-count": len(pjsToUpdateState),
	}).Info("Reported job(s), now will update pj(s).")
	// All latest jobs for this label were already reported, none of the jobs
	// for this label are worthy reporting any more. Mark all of them as
	// reported to avoid corner cases where an older job finished later, and the
	// newer prowjobs CRD was somehow missing from the cluster.
	for _, pjob := range pjsToUpdateState {
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

// jobFromPJ extracts the minimum job information for the given ProwJob, to be
// used by GenerateReport to create a textual report of it. It will be
// serialized to a single line of text, with or without the URL depending on how
// much room we have left against maxCommentSizeLimit. The reason why it is
// serialized as a single line of text is because ParseReport uses newlines as a
// token delimiter.
func jobFromPJ(pj *v1.ProwJob) Job {
	return Job{Name: pj.Spec.Job, State: pj.Status.State, Icon: statusIcon(pj.Status.State), URL: pj.Status.URL}
}

func (j *Job) serializeWithoutURL() string {
	return fmt.Sprintf(jobReportFormatWithoutURL, j.Icon, j.Name, strings.ToUpper(string(j.State)))
}

func (j *Job) serialize() string {

	// It may be that the URL is empty, so we have to take care not to link it
	// as such if we're doing Markdown-flavored URLs. This can happen if the job
	// has not been scheduled due to some other failure.
	if j.URL == "" {
		return fmt.Sprintf(jobReportFormatUrlNotFound, j.Icon, j.Name, strings.ToUpper(string(j.State)))
	}

	return fmt.Sprintf(jobReportFormat, j.Icon, j.Name, j.URL, strings.ToUpper(string(j.State)))
}

func deserialize(s string, j *Job) error {
	var state string
	var formats = []struct {
		regex  string
		tokens []*string
	}{
		// Legacy format. This is to cover the case where we're still trying to
		// parse legacy style comments during the transition to the new style
		// (just in case).
		//
		// TODO(listx): It should be safe to delete this legacy format checker
		// after we migrate all Prow instances over to the version of crier's
		// gerrit reporter (this file) that uses the Markdown-flavored links.
		// There is no hurry to delete this code because having it here is
		// harmless, other than incurring negligible CPU cycles.
		{jobReportFormatLegacyRegex,
			[]*string{&j.Icon, &j.Name, &state, &j.URL}},

		// New format with Markdown syntax for the URL.
		{jobReportFormatRegex,
			[]*string{&j.Icon, &j.Name, &j.URL, &state}},

		// New format, but where the URL was not found.
		{jobReportFormatUrlNotFoundRegex,
			[]*string{&j.Icon, &j.Name, &j.URL, &state}},

		// Job without URL (because GenerateReport() decided that adding a URL would be too much).
		{jobReportFormatWithoutURLRegex,
			[]*string{&j.Icon, &j.Name, &state}},
	}

	for _, format := range formats {

		re := regexp.MustCompile(format.regex)
		if !re.MatchString(s) {
			continue
		}

		// We drop the first token because it is the
		// entire string itself.
		matchedTokens := re.FindStringSubmatch(s)[1:]

		// Even though the regexes are exact matches "^...$", we still check the
		// number of tokens found just to be sure.
		if len(matchedTokens) != len(format.tokens) {
			return fmt.Errorf("tokens: got %d, want %d", len(format.tokens), len(matchedTokens))
		}

		for i := range format.tokens {
			*format.tokens[i] = matchedTokens[i]
		}

		state = strings.ToLower(state)
		validProwJobState := false
		for _, pjState := range v1.GetAllProwJobStates() {
			if v1.ProwJobState(state) == pjState {
				validProwJobState = true
				break
			}
		}
		if !validProwJobState {
			return fmt.Errorf("invalid prow job state %q", state)
		}
		j.State = v1.ProwJobState(state)

		return nil
	}

	return fmt.Errorf("Could not deserialize %q to a job", s)
}

func headerMessageLine(success, total int, additionalText string) string {
	return fmt.Sprintf(jobReportHeader, defaultProwHeader, success, total, additionalText)
}

func isHeaderMessageLine(s string) bool {
	return strings.HasPrefix(s, defaultProwHeader)
}

func errorMessageLine(s string) string {
	return fmt.Sprintf("[%s: %s]", errorLinePrefix, s)
}

func isErrorMessageLine(s string) bool {
	return strings.HasPrefix(s, fmt.Sprintf("[%s: ", errorLinePrefix))
}

// GenerateReport generates a JobReport based on pjs passed in. As URLs are very
// long string, including them in the report could easily make the report exceed
// the maxCommentSizeLimit of 14400 characters.  Unfortunately we need info for
// all prowjobs for /retest to work, which is by far the most reliable way of
// retrieving prow jobs results (this is because prowjob custom resources are
// garbage-collected by sinker after max_pod_age, which normally is 48 hours).
// So to ensure that all prow jobs results are displayed, URLs for some of the
// jobs are omitted from this report to keep it under 14400 characters.
//
// Note that even if we drop all URLs, it may be that we're forced to drop jobs
// names entirely if there are just too many jobs. So there is actually no
// guarantee that we'll always report all job names (although this is rare in
// practice).
//
// customCommentSizeLimit is used by unit tests that actually test that we
// perform job serialization with or without URLs (without this, our unit tests
// would have to be very large to hit the default maxCommentSizeLimit to trigger
// the "don't print URLs" behavior).
func GenerateReport(pjs []*v1.ProwJob, customCommentSizeLimit int) JobReport {
	// A JobReport has 2 string parts: (1) the "Header" that summarizes the
	// report, and (2) a list of links to each job result (URL) (the "Message").
	// We take care to make sure that the overall Header + Message falls under
	// the commentSizeLimit, which is the maxCommentSizeLimit by default (this
	// limit is parameterized so that we can test different size limits in unit
	// tests).

	// By default, use the maximum comment size limit const.
	commentSizeLimit := maxCommentSizeLimit
	if customCommentSizeLimit > 0 {
		commentSizeLimit = customCommentSizeLimit
	}

	// Construct JobReport.
	var additionalText string
	report := JobReport{Total: len(pjs)}
	for _, pj := range pjs {
		job := jobFromPJ(pj)
		report.Jobs = append(report.Jobs, job)
		if pj.Status.State == v1.SuccessState {
			report.Success++
		}
		if val, ok := pj.Labels[kube.CreatedByTideLabel]; ok && val == "true" {
			additionalText = " (Not a duplicated report. Some of the jobs below were triggered by Tide)"
		}
	}
	numJobs := len(report.Jobs)

	report.prioritizeFailedJobs()

	// Construct our comment that we want to send off to Gerrit. It is composed
	// of the Header + Message.

	// Construct report.Header portion.
	report.Header = headerMessageLine(report.Success, report.Total, additionalText)
	commentSize := len(report.Header)

	// Construct report.Messages portion. We need to construct the long list of
	// job result messages, delimited by a newline, where each message
	// corresponds to a single job result.  These messages are concatenated
	// together into report.Message.

	// First just serialize without the URL. Afterwards, if we have room, we can
	// start adding URLs as much as possible (failed jobs first). If we do not
	// have room, simply truncate from the end of the list until we fall under
	// the comment limit. This second scenario is highly unlikely, but is still
	// something to consider (and tell the user about).
	jobLines := []string{}
	for _, job := range report.Jobs {
		line := job.serializeWithoutURL()
		jobLines = append(jobLines, line)
		commentSize += len(line)
	}

	// Initially we skip displaying URLs for all jobs. Then depending on where
	// we stand with our overall commentSize, we can try to either build it up
	// (add URL links), or truncate it down (remove jobs from the end).
	//
	// For truncation, note that we truncate from the end, so that we prioritize
	// reporting the names of the failed jobs (if any), which are at the front
	// of the list.
	skippedURLsFormat := "Skipped displaying URLs for %d/%d jobs due to reaching gerrit comment size limit"
	errorLine := errorMessageLine(fmt.Sprintf(skippedURLsFormat, numJobs, numJobs))
	commentSize += len(errorLine)
	if commentSize < commentSizeLimit {
		skipped := numJobs
		for i, job := range report.Jobs {
			lineWithURL := job.serialize()

			lineSizeWithoutURL := len(jobLines[i])
			lineSizeWithURL := len(lineWithURL)

			delta := lineSizeWithURL - lineSizeWithoutURL

			proposedErrorLine := errorMessageLine(fmt.Sprintf(skippedURLsFormat, skipped-1, numJobs))

			// It could be that the new error line is smaller than the existing
			// one, because e.g. `skipped` goes down from 100 to 99 (1 character
			// less), or that we don't need the errorLine at all because there
			// would be 0 skipped.
			if skipped-1 == 0 {
				proposedErrorLine = ""
			}
			delta -= (len(errorLine) - len(proposedErrorLine))

			// Only replace the current line if the new commentSize would still
			// be under the commentSizeLimit. Otherwise, break early because the
			// commentSize is too big already.
			if commentSize+delta < commentSizeLimit {
				jobLines[i] = lineWithURL
				commentSize += delta
				errorLine = proposedErrorLine
				skipped--
			} else {
				break
			}
		}

		report.Message += strings.Join(jobLines, "")

		if skipped > 0 {
			report.Message += errorLine
		}

	} else {
		// Drop existing errorLine (skip displaying URLs) because it no longer
		// applies (we're skipping jobs entirely now, not just skipping the
		// display of URLs).
		commentSize -= len(errorLine)
		errorLine = ""
		skipped := 0
		skippedJobsFormat := "Skipped displaying %d/%d jobs due to reaching gerrit comment size limit (too many jobs)"

		last := numJobs - 1
		for i := range report.Jobs {
			j := last - i

			// Truncate (delete) a job line.
			commentSize -= len(jobLines[i])
			jobLines[j] = ""
			skipped++

			// Construct new  errorLine to account for the truncation.
			errorLine = errorMessageLine(fmt.Sprintf(skippedJobsFormat, skipped, numJobs))

			// Break early if we've truncated enough to be under the
			// commentSizeLimit.
			if commentSize+len(errorLine) < commentSizeLimit {
				break
			}
		}

		report.Message += strings.Join(jobLines, "")
		report.Message += errorLine
	}

	return report
}

// prioritizeFailedJobs sorts jobs so that the report will start with the failed
// jobs first. This also makes it so that the failed jobs get priority in terms
// of getting linked to the job URL.
func (report *JobReport) prioritizeFailedJobs() {
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
		// We don't care about other states, so keep original order.
		return true
	})
}

// ParseReport creates a jobReport from a string, nil if cannot parse
func ParseReport(message string) *JobReport {
	contents := strings.Split(message, "\n")
	start := 0
	isReport := false
	for start < len(contents) {
		if isHeaderMessageLine(contents[start]) {
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
			logrus.Warn(err)
			continue
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
