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
package reporter

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

type gerritClient interface {
	SetReview(instance, id, revision, message string, labels map[string]string) error
}

// Client is a gerrit reporter client
type Client struct {
	gc     gerritClient
	lister pjlister.ProwJobLister
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

	// Only report when all jobs of the same type on the same revision finished
	selector := labels.Set{
		client.GerritRevision: pj.ObjectMeta.Labels[client.GerritRevision],
		kube.ProwJobTypeLabel: pj.ObjectMeta.Labels[kube.ProwJobTypeLabel],
	}

	if pj.ObjectMeta.Labels[client.GerritReportLabel] == "" {
		// Shouldn't happen, adapter should already have defaulted to Code-Review
		logrus.Errorf("Gerrit report label not set for job %s", pj.Spec.Job)
	} else {
		selector[client.GerritReportLabel] = pj.ObjectMeta.Labels[client.GerritReportLabel]
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

	selector := labels.Set{
		clientGerritRevision: pj.ObjectMeta.Labels[clientGerritRevision],
		pjTypeLabel:          pj.ObjectMeta.Labels[pjTypeLabel],
	}
	if pj.ObjectMeta.Labels[gerritReportLabel] == "" {
		// Shouldn't happen, adapter should already have defaulted to Code-Review
		logger.Errorf("Gerrit report label not set for job %s", pj.Spec.Job)
	} else {
		selector[gerritReportLabel] = pj.ObjectMeta.Labels[gerritReportLabel]
	}

	// list all prowjobs in the patchset matching pj's type (pre- or post-submit)

	pjsOnRevisionWithSameLabel, err := c.lister.List(selector.AsSelector())
	if err != nil {
		logger.WithError(err).Errorf("Cannot list prowjob with selector %v", selector)
		return nil, err
	}

	// generate an aggregated report:
	success := 0
	message := ""
	var toReportJobs []*v1.ProwJob
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

		checkOrX := "❌"
		if pjOnRevisionWithSameLabel.Status.State == v1.SuccessState {
			checkOrX = "✔️"
			success++
		}

		message += fmt.Sprintf("\n\n%s %s %s - %s", checkOrX, pjOnRevisionWithSameLabel.Spec.Job, strings.ToUpper(string(pjOnRevisionWithSameLabel.Status.State)), pjOnRevisionWithSameLabel.Status.URL)
	}
	total := len(toReportJobs)
	if total <= 0 {
		// Shouldn't happen but return if does
		logger.Warn("Tried to report empty or aborted jobs.")
		return nil, nil
	}

	message = fmt.Sprintf("%d out of %d jobs passed!%s", success, total, message)

	// report back
	gerritID := pj.ObjectMeta.Annotations[clientGerritID]
	gerritInstance := pj.ObjectMeta.Annotations[clientGerritInstance]
	gerritRevision := pj.ObjectMeta.Labels[clientGerritRevision]
	reportLabel := client.CodeReview
	if val, ok := pj.ObjectMeta.Labels[client.GerritReportLabel]; ok {
		reportLabel = val
	}

	vote := client.LBTM
	if success == total {
		vote = client.LGTM
	}
	reviewLabels := map[string]string{reportLabel: vote}

	logger.Infof("Reporting to instance %s on id %s with message %s", gerritInstance, gerritID, message)
	if err := c.gc.SetReview(gerritInstance, gerritID, gerritRevision, message, reviewLabels); err != nil {
		logger.WithError(err).Errorf("fail to set review with %s label on change ID %s", reportLabel, gerritID)

		// possibly don't have label permissions, try without labels
		message = fmt.Sprintf("[NOTICE]: Prow Bot cannot access %s label!\n%s", reportLabel, message)
		if err := c.gc.SetReview(gerritInstance, gerritID, gerritRevision, message, nil); err != nil {
			logger.WithError(err).Errorf("fail to set plain review on change ID %s", gerritID)
			return nil, err
		}
	}
	logger.Infof("Review Complete, reported jobs: %v", toReportJobs)

	return toReportJobs, nil
}
