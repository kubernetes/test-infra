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
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/andygrunwald/go-gerrit"
	"github.com/sirupsen/logrus"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/gerrit/client"
	"k8s.io/test-infra/prow/kube"
)

const (
	cross      = "❌"
	tick       = "✔️"
	hourglass  = "⏳"
	prohibited = "🚫"

	defaultProwHeader = "Prow Status:"
	jobReportFormat   = "%s %s %s - %s\n"

	// lgtm means all presubmits passed, but need someone else to approve before merge (looks good to me).
	lgtm = "+1"
	// lbtm means some presubmits failed, perfer not merge (looks bad to me).
	lbtm = "-1"
	// lztm is the minimum score for a postsubmit.
	lztm = "0"
	// codeReview is the default gerrit code review label
	codeReview = client.CodeReview
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
	gc     gerritClient
	lister ctrlruntimeclient.Reader
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
func NewReporter(cookiefilePath string, projects map[string][]string, lister ctrlruntimeclient.Reader) (*Client, error) {
	gc, err := client.NewClient(projects)
	if err != nil {
		return nil, err
	}
	gc.Authenticate(cookiefilePath, "")
	return &Client{
		gc:     gc,
		lister: lister,
	}, nil
}

// GetName returns the name of the reporter
func (c *Client) GetName() string {
	return "gerrit-reporter"
}

// ShouldReport returns if this prowjob should be reported by the gerrit reporter
func (c *Client) ShouldReport(ctx context.Context, log *logrus.Entry, pj *v1.ProwJob) bool {
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
		if err := c.lister.List(ctx, &pjs, ctrlruntimeclient.MatchingLabels(selector)); err != nil {
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
	return allPJsAgreeToReport([]string{client.GerritRevision, kube.ProwJobTypeLabel, client.GerritReportLabel}, func(pj *v1.ProwJob) bool {
		if pj.Status.State == v1.TriggeredState || pj.Status.State == v1.PendingState {
			// other jobs with same label are still running on this revision, skip report
			log.Info("Other jobs with same label are still running on this revision")
			return false
		}
		return true
	}) && allPJsAgreeToReport([]string{kube.OrgLabel, kube.RepoLabel, kube.PullLabel}, func(pj *v1.ProwJob) bool {
		// Newer patchset exists, skip report
		return patchsetNumFromPJ(pj) <= patchsetNum
	})
}

// Report will send the current prowjob status as a gerrit review
func (c *Client) Report(ctx context.Context, logger *logrus.Entry, pj *v1.ProwJob) ([]*v1.ProwJob, *reconcile.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	clientGerritRevision := client.GerritRevision
	clientGerritID := client.GerritID
	clientGerritInstance := client.GerritInstance
	pjTypeLabel := kube.ProwJobTypeLabel
	gerritReportLabel := client.GerritReportLabel

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

		var pjsOnRevisionWithSameLabel v1.ProwJobList
		if err := c.lister.List(ctx, &pjsOnRevisionWithSameLabel, ctrlruntimeclient.MatchingLabels(selector)); err != nil {
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
			if pjOnRevisionWithSameLabel.Status.State == v1.AbortedState {
				continue
			}
			toReportJobs = append(toReportJobs, pjOnRevisionWithSameLabel)
		}
	}
	report := GenerateReport(toReportJobs)
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
		logger.Warn("Tried to report empty or aborted jobs.")
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
		logger.WithError(err).Errorf("fail to set review with label %q on change ID %s", reportLabel, gerritID)

		if reportLabel == "" {
			return nil, nil, err
		}
		// Retry without voting on a label
		message := fmt.Sprintf("[NOTICE]: Prow Bot cannot access %s label!\n%s", reportLabel, message)
		if err := c.gc.SetReview(gerritInstance, gerritID, gerritRevision, message, nil); err != nil {
			logger.WithError(err).Errorf("fail to set plain review on change ID %s", gerritID)
			return nil, nil, err
		}
	}

	logger.Infof("Review Complete, reported jobs: %v", toReportJobs)
	return toReportJobs, nil, nil
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

func (j *Job) serialize() string {
	return fmt.Sprintf(jobReportFormat, j.Icon, j.Name, strings.ToUpper(string(j.State)), j.URL)
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

func GenerateReport(pjs []*v1.ProwJob) JobReport {
	report := JobReport{Total: len(pjs)}
	for _, pj := range pjs {
		job := jobFromPJ(pj)
		report.Jobs = append(report.Jobs, job)
		if pj.Status.State == v1.SuccessState {
			report.Success++
		}

		report.Message += job.serialize()
		report.Message += "\n"

	}
	report.Header = defaultProwHeader
	report.Header += fmt.Sprintf(" %d out of %d pjs passed!\n", report.Success, report.Total)
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
		if contents[i] == "" {
			continue
		}
		var j Job
		if err := deserialize(contents[i], &j); err != nil {
			logrus.Warnf("Could not deserialize %s to a job: %v", contents[i], err)
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
