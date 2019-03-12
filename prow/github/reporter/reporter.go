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
package reporter

import (
	"k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github/report"
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
}

// NewReporter returns a reporter client
func NewReporter(gc report.GitHubClient, cfg config.Getter, reportAgent v1.ProwJobAgent) *Client {
	return &Client{
		gc:          gc,
		config:      cfg,
		reportAgent: reportAgent,
	}
}

// GetName returns the name of the reporter
func (c *Client) GetName() string {
	return GitHubReporterName
}

// ShouldReport returns if this prowjob should be reported by the github reporter
func (c *Client) ShouldReport(pj *v1.ProwJob) bool {

	if !pj.Spec.Report {
		// Respect report field
		return false
	}

	if pj.Spec.Type != v1.PresubmitJob && pj.Spec.Type != v1.PostsubmitJob {
		// Report presubmit and postsubmit github jobs for github reporter
		return false
	}

	if c.reportAgent != "" && pj.Spec.Agent != c.reportAgent {
		// Only report for specified agent
		return false
	}

	return true
}

// Report will report via reportlib
func (c *Client) Report(pj *v1.ProwJob) ([]*v1.ProwJob, error) {
	// TODO(krzyzacy): ditch ReportTemplate, and we can drop reference to config.Getter
	return []*v1.ProwJob{pj}, report.Report(c.gc, c.config().Plank.ReportTemplate, *pj, c.config().GitHubReporter.JobTypesToReport)
}
