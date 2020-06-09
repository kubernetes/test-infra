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
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"

	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	pjlister "k8s.io/test-infra/prow/client/listers/prowjobs/v1"
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
}

// Client is a gerrit reporter client
type Client struct {
	gc     gerritClient
	lister pjlister.ProwJobLister
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
func NewReporter(cookiefilePath string, projects map[string][]string, lister pjlister.ProwJobLister) (*Client, error) {
	gc, err := client.NewClient(projects)
	if err != nil {
		return nil, err
	}
	gc.Start(cookiefilePath)
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
func (c *Client) ShouldReport(pj *v1.ProwJob) bool {

	if pj.Status.State == v1.TriggeredState || pj.Status.State == v1.PendingState {
		// not done yet
		logrus.WithField("prowjob", pj.ObjectMeta.Name).Info("PJ not finished")
		return false
	}

	if pj.Status.State == v1.AbortedState {
		// aborted (new patchset)
		logrus.WithField("prowjob", pj.ObjectMeta.Name).Info("PJ aborted")
		return false
	}

	// has gerrit metadata (scheduled by gerrit adapter)
	if pj.ObjectMeta.Annotations[client.GerritID] == "" ||
		pj.ObjectMeta.Annotations[client.GerritInstance] == "" ||
		pj.ObjectMeta.Labels[client.GerritRevision] == "" {
		logrus.WithField("prowjob", pj.ObjectMeta.Name).Info("Not a gerrit job")
		return false
	}

	// Don't wait for report aggregation if not voting on any label
	if pj.ObjectMeta.Labels[client.GerritReportLabel] == "" {
		return true
	}

	// Only report when all jobs of the same type on the same revision finished
	selector := labels.Set{
		client.GerritRevision:    pj.ObjectMeta.Labels[client.GerritRevision],
		kube.ProwJobTypeLabel:    pj.ObjectMeta.Labels[kube.ProwJobTypeLabel],
		client.GerritReportLabel: pj.ObjectMeta.Labels[client.GerritReportLabel],
	}

	pjs, err := c.lister.List(selector.AsSelector())
	if err != nil {
		logrus.WithError(err).Errorf("Cannot list prowjob with selector %v", selector)
		return false
	}

	for _, pjob := range pjs {
		if pjob.Status.State == v1.TriggeredState || pjob.Status.State == v1.PendingState {
			// other jobs with same label are still running on this revision, skip report
			logrus.WithField("prowjob", pjob.ObjectMeta.Name).Info("Other jobs with same label are still running on this revision")
			return false
		}
	}

	return true
}

// Report will send the current prowjob status as a gerrit review
func (c *Client) Report(pj *v1.ProwJob) ([]*v1.ProwJob, error) {

	logger := logrus.WithField("prowjob", pj)

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

		selector := labels.Set{
			clientGerritRevision: pj.ObjectMeta.Labels[clientGerritRevision],
			pjTypeLabel:          pj.ObjectMeta.Labels[pjTypeLabel],
			gerritReportLabel:    pj.ObjectMeta.Labels[gerritReportLabel],
		}

		pjsOnRevisionWithSameLabel, err := c.lister.List(selector.AsSelector())
		if err != nil {
			logger.WithError(err).Errorf("Cannot list prowjob with selector %v", selector)
			return nil, err
		}

		mostRecentJob := map[string]*v1.ProwJob{}
		for _, pjOnRevisionWithSameLabel := range pjsOnRevisionWithSameLabel {
			job, ok := mostRecentJob[pjOnRevisionWithSameLabel.Spec.Job]
			if !ok || job.CreationTimestamp.Time.Before(pjOnRevisionWithSameLabel.CreationTimestamp.Time) {
				mostRecentJob[pjOnRevisionWithSameLabel.Spec.Job] = pjOnRevisionWithSameLabel
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
		return nil, nil
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
			vote = lbtm
		default:
			vote = lztm
		}
		reviewLabels = map[string]string{reportLabel: vote}
	}

	logger.Infof("Reporting to instance %s on id %s with message %s", gerritInstance, gerritID, message)
	if err := c.gc.SetReview(gerritInstance, gerritID, gerritRevision, message, reviewLabels); err != nil {
		logger.WithError(err).Errorf("fail to set review with label %q on change ID %s", reportLabel, gerritID)

		if reportLabel == "" {
			return nil, err
		}
		// Retry without voting on a label
		message := fmt.Sprintf("[NOTICE]: Prow Bot cannot access %s label!\n%s", reportLabel, message)
		if err := c.gc.SetReview(gerritInstance, gerritID, gerritRevision, message, nil); err != nil {
			logger.WithError(err).Errorf("fail to set plain review on change ID %s", gerritID)
			return nil, err
		}
	}

	logger.Infof("Review Complete, reported jobs: %v", toReportJobs)
	return toReportJobs, nil
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
