/*
Copyright 2019 The Kubernetes Authors.

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
package reporter

import (
	"fmt"

	"cloud.google.com/go/storage"
	"k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/custom-reporter/guest-test-infra/versionutil"
	"k8s.io/test-infra/prow/github"
)

const (
	// GuestTestInfraReporter is the name for this report
	GuestTestInfraReporter = "guest-test-infra-reporter"
)

// Client is a guest-test-infra reporter client
type Client struct {
	gc          github.Client
	gcs         storage.Client
	reportAgent v1.ProwJobAgent
}

// NewReporter returns a reporter client
func NewReporter(gc github.Client, gcs storage.Client, reportAgent v1.ProwJobAgent) *Client {
	return &Client{
		gc:          gc,
		gcs:         gcs,
		reportAgent: reportAgent,
	}
}

// GetName returns the name of the reporter
func (c *Client) GetName() string {
	return GuestTestInfraReporter
}

// ShouldReport returns if this prowjob should be reported by the github reporter
func (c *Client) ShouldReport(pj *v1.ProwJob) bool {
	if !pj.Spec.Report {
		// Respect report field
		return false
	}

	if pj.Spec.Type != v1.PostsubmitJob {
		// Report only for postsubmit jobs
		return false
	}

	if pj.Status.State != v1.SuccessState {
		// only run after successful completion of prowjob
		return false
	}

	return true
}

// Report will report via reportlib
func (c *Client) Report(pj *v1.ProwJob) ([]*v1.ProwJob, error) {
	org := pj.Spec.Refs.Org
	repo := pj.Spec.Refs.Repo
	baseSHA := pj.Spec.Refs.BaseSHA
	tags, err := c.gc.ListTag(org, repo)

	if err != nil {
		return []*v1.ProwJob{pj}, fmt.Errorf("Error while fetching github tags: %+v\n", err)
	}
	var tagNames []string
	for _, tag := range *tags {
		tagNames = append(tagNames, tag.Name)
	}

	latestVersion, err := versionutil.GetLatestVersionTag(tagNames)
	if err != nil {
		return []*v1.ProwJob{pj}, fmt.Errorf("Error fetching latest version: %+v", err)
	}

	newVersion := latestVersion.IncrementVersion()

	user, err := c.gc.BotUser()
	if err != nil {
		return []*v1.ProwJob{pj}, fmt.Errorf("Error fetching user details: %+v", err)
	}
	createTagRequest := github.TagRequest{
		Name:    newVersion.String(),
		Message: "",
		Object:  baseSHA,
		Type:    github.TagTypeCommit,
		Tagger:  *user,
	}
	ret, err := c.gc.CreateTag(org, repo, createTagRequest)
	if err != nil {
		return []*v1.ProwJob{pj}, fmt.Errorf("Error creating tag: %+v", err)
	}
	resp, err := c.gc.CreateRef(org, repo, fmt.Sprintf("refs/tags/%s", ret.Tag), ret.Object.SHA)
	if err != nil {
		return []*v1.ProwJob{pj}, fmt.Errorf("Error creating ref: %+v\n", err)
	}
	fmt.Printf("Ref: %+v\n", resp)
	fmt.Print("tag created....\n")
	return []*v1.ProwJob{pj}, nil
}
