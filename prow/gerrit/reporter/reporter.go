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
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/apis/prowjobs/v1"
)

// Client holds a gerrit client
type Client struct {
	// TODO(krzyzacy): fill this up
	// gc gerrit.Client
}

// NewReporter returns a reporter client
func NewReporter() *Client {
	return &Client{}
}

// Report will send the current prowjob status as a gerrit review
func (c *Client) Report(pj *v1.ProwJob) error {
	logrus.Infof("Reporting %s to gerrit, state %s", pj.Spec.Job, pj.Status.State)
	// TODO(krzyzacy): we should also do an aggregate report, as golang does in their repo
	// see https://go-review.googlesource.com/c/go/+/132155
	return nil
}
