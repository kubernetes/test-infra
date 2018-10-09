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
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/gerrit/client"
)

type gerritClient interface {
	SetReview(instance, id, revision, message string) error
}

// Client is a gerrit reporter client
type Client struct {
	gc gerritClient
}

// NewReporter returns a reporter client
func NewReporter(cookiefilePath string, projects map[string][]string) (*Client, error) {
	gc, err := gerrit.NewClient(projects)
	if err != nil {
		return nil, err
	}
	gc.Start(cookiefilePath)
	return &Client{
		gc: gc,
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
	return pj.ObjectMeta.Annotations["gerrit-id"] != "" &&
		pj.ObjectMeta.Annotations["gerrit-instance"] != "" &&
		pj.ObjectMeta.Labels["gerrit-revision"] != ""
}

// Report will send the current prowjob status as a gerrit review
func (c *Client) Report(pj *v1.ProwJob) error {
	// TODO(krzyzacy): we should also do an aggregate report, as golang does in their repo
	// see https://go-review.googlesource.com/c/go/+/132155
	// also add ability to set code-review labels
	// ref: https://github.com/kubernetes/test-infra/issues/9433

	if pj.Spec.Refs == nil {
		return errors.New("no pj.Spec.Refs, not a presubmit job (should not happen?!)")
	}

	message := fmt.Sprintf("Job %s finished with %s\n Gubernator URL: %s", pj.Spec.Job, pj.Status.State, pj.Status.URL)

	// report back
	gerritID := pj.ObjectMeta.Annotations["gerrit-id"]
	gerritInstance := pj.ObjectMeta.Annotations["gerrit-instance"]
	gerritRevision := pj.ObjectMeta.Labels["gerrit-revision"]

	logrus.Infof("Reporting job %s to instance %s on id %s with message %s", pj.Spec.Job, gerritInstance, gerritID, message)
	if err := c.gc.SetReview(gerritInstance, gerritID, gerritRevision, message); err != nil {
		logrus.Warnf("fail to set review")
		return err
	}
	logrus.Infof("Review Complete")

	return nil
}
