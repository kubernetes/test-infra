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

func handleStatusEvent(pc plugins.PluginClient, se github.StatusEvent) error {
	return handle(pc.GitHubClient, pc.Logger, se)
}

// 1. Check that the status event received from the webhook is for the CNCF-CLA.
// 2. Use the github search API to search for the PRs which match the commit hash corresponding to the status event.
// 3. For each issue that matches, check that the PR's HEAD commit hash against the commit hash for which the status
//    was received. This is because we only care about the status associated with the last (latest) commit in a PR.
// 4. Set the corresponding CLA label if needed.
func handle(gc gitHubClient, log *logrus.Entry, se github.StatusEvent) error {
	if se.State == "" || se.Context == "" {
		return fmt.Errorf("invalid status event delivered with empty state/context")
	}

	if se.Context != claContextName {
		// Not the CNCF CLA context, do not process this.
		return nil
	}

	if se.State == github.StatusPending {
		// do nothing and wait for state to be updated.
		return nil
	}

	org := se.Repo.Owner.Login
	repo := se.Repo.Name
	issues, err := gc.FindIssues(fmt.Sprintf("%s repo:%s/%s type:pr state:open", se.SHA, org, repo))
	if err != nil {
		return err
	}

	for _, issue := range issues {
		hasCncfYes := issue.HasLabel(claYesLabel)
		hasCncfNo := issue.HasLabel(claNoLabel)
		if hasCncfYes && se.State == github.StatusSuccess {
			// Nothing to update.
			continue
		}

		if hasCncfNo && (se.State == github.StatusFailure || se.State == github.StatusError) {
			// Nothing to update.
			continue
		}

		pr, err := gc.GetPullRequest(org, repo, issue.Number)
		if err != nil {
			log.WithError(err).Warningf("Unable to fetch PR-%d from %s/%s.", issue.Number, org, repo)
			continue
		}

		// Check if this is the latest commit in the PR.
		if pr.Head.SHA != se.SHA {
			continue
		}

		number := pr.Number
		if se.State == github.StatusSuccess {
			if hasCncfNo {
				gc.RemoveLabel(org, repo, number, claNoLabel)
			}
			gc.AddLabel(org, repo, number, claYesLabel)
			continue
		}

		// If we end up here, the status is a failure/error.
		// TODO(foxish): add a comment which explains what happened and how to rectify it.
		if hasCncfYes {
			gc.RemoveLabel(org, repo, number, claYesLabel)
		}
		gc.AddLabel(org, repo, number, claNoLabel)
	}
	return nil
}
