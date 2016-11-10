/*
Copyright 2016 The Kubernetes Authors.

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

package cla

import (
	"fmt"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/util"
)

const (
	pluginName     = "cla"
	claContextName = "cla/linuxfoundation"
	claYesLabel    = "cncf-cla: yes"
	claNoLabel     = "cncf-cla: no"
)

func init() {
	plugins.RegisterStatusEventHandler(pluginName, handleStatusEvent)
}

type gitHubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
	AddLabel(owner, repo string, number int, label string) error
	RemoveLabel(owner, repo string, number int, label string) error
	GetPullRequest(owner, repo string, number int) (*github.PullRequest, error)
	FindIssues(query string) ([]github.Issue, error)
}

func handleStatusEvent(pa *plugins.PluginAgent, se github.StatusEvent) error {
	return handle(pa.GitHubClient, pa.Logger, se)
}

func handle(gc gitHubClient, log *logrus.Entry, se github.StatusEvent) error {
	if se.State == nil || se.Context == nil {
		return fmt.Errorf("Invalid status event delivered with empty state/context")
	}

	if *se.Context != claContextName {
		return nil
	}

	if *se.State == github.StatusPending {
		// do nothing and wait for state to be updated.
		return nil
	}

	org := se.Repo.Owner.Login
	repo := se.Repo.Name
	issues, err := gc.FindIssues(fmt.Sprintf("%s&repo=%s/%s&type=pr", *se.SHA, org, repo))
	if err != nil {
		return err
	}

	for _, issue := range issues {
		hasCncfYes := util.IssueHasLabel(issue, claYesLabel)
		hasCncfNo := util.IssueHasLabel(issue, claNoLabel)
		if hasCncfYes && *se.State == github.StatusSuccess {
			// nothing to update.
			continue
		}

		if hasCncfNo && (*se.State == github.StatusFailure || *se.State == github.StatusError) {
			// nothing to update.
			continue
		}

		pr, err := gc.GetPullRequest(org, repo, issue.Number)
		if err != nil {
			log.Warningf("Unable to fetch PR-%d from %s/%s.", issue.Number, org, repo)
			continue
		}

		if pr.Head.SHA != *se.SHA {
			continue
		}

		number := pr.Number
		if *se.State == github.StatusSuccess {
			if hasCncfNo {
				gc.RemoveLabel(org, repo, number, claNoLabel)
			}
			gc.AddLabel(org, repo, number, claYesLabel)
		} else {
			if hasCncfYes {
				gc.RemoveLabel(org, repo, number, claYesLabel)
			}
			gc.AddLabel(org, repo, number, claNoLabel)
		}
	}
	return nil
}
