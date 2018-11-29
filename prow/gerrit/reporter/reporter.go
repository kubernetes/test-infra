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

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"

	"k8s.io/test-infra/prow/apis/prowjobs/v1"
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
		return false
	}

	// has gerrit metadata (scheduled by gerrit adapter)
	if pj.ObjectMeta.Annotations[client.GerritID] == "" ||
		pj.ObjectMeta.Annotations[client.GerritInstance] == "" ||
		pj.ObjectMeta.Labels[client.GerritRevision] == "" {
		return false
	}

	// Only report when all jobs of the same type on the same revision finished
	selector := labels.Set{
		client.GerritRevision: pj.ObjectMeta.Labels[client.GerritRevision],
		kube.ProwJobTypeLabel: pj.ObjectMeta.Labels[kube.ProwJobTypeLabel],
	}
	pjs, err := c.lister.List(selector.AsSelector())
	if err != nil {
		logrus.WithError(err).Errorf("Cannot list prowjob with selector %v", selector)
		return false
	}

	for _, pj := range pjs {
		if pj.Status.State == v1.TriggeredState || pj.Status.State == v1.PendingState {
			// other jobs are still running on this revision, skip report
			return false
		}
	}

	return true
}

// Report will send the current prowjob status as a gerrit review
func (c *Client) Report(pj *v1.ProwJob) error {
	// If you are hitting here, which means the entire patchset has been finished :-)

	clientGerritRevision := client.GerritRevision
	clientGerritID := client.GerritID
	clientGerritInstance := client.GerritInstance
	pjTypeLabel := kube.ProwJobTypeLabel

	// list all prowjobs in the patchset matching pj's type (pre- or post-submit)
	selector := labels.Set{
		clientGerritRevision: pj.ObjectMeta.Labels[clientGerritRevision],
		pjTypeLabel:          pj.ObjectMeta.Labels[pjTypeLabel],
	}
	pjsOnRevision, err := c.lister.List(selector.AsSelector())
	if err != nil {
		logrus.WithError(err).Errorf("Cannot list prowjob with selector %v", selector)
		return err
	}

	// generate an aggregated report:
	total := len(pjsOnRevision)
	success := 0
	message := ""

	for _, pjOnRevision := range pjsOnRevision {
		if pjOnRevision.Status.PrevReportStates[c.GetName()] == pjOnRevision.Status.State {
			logrus.Infof("Revision %s has been reported already", pj.ObjectMeta.Labels[clientGerritRevision])
			return nil
		}

		if pjOnRevision.Status.State == v1.SuccessState {
			success++
		}

		message = fmt.Sprintf("%s\nJob %s finished with %s -- URL: %s", message, pjOnRevision.Spec.Job, pjOnRevision.Status.State, pjOnRevision.Status.URL)
	}

	message = fmt.Sprintf("%d out of %d jobs passed!\n%s", success, total, message)

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
	labels := map[string]string{reportLabel: vote}

	logrus.Infof("Reporting to instance %s on id %s with message %s", gerritInstance, gerritID, message)
	if err := c.gc.SetReview(gerritInstance, gerritID, gerritRevision, message, labels); err != nil {
		logrus.WithError(err).Errorf("fail to set review with %s label on change ID %s", reportLabel, gerritID)

		// possibly don't have label permissions, try without labels
		message = fmt.Sprintf("[NOTICE]: Prow Bot cannot access %s label!\n%s", reportLabel, message)
		if err := c.gc.SetReview(gerritInstance, gerritID, gerritRevision, message, nil); err != nil {
			logrus.WithError(err).Errorf("fail to set plain review on change ID %s", gerritID)
			return err
		}
	}
	logrus.Infof("Review Complete")

	return nil
}
