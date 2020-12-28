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

package updater

import (
	"fmt"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
)

// Indicates whether maintainers can modify a pull request in fork.
const (
	AllowMods   = true
	PreventMods = false
)

type updateClient interface {
	UpdatePullRequest(org, repo string, number int, title, body *string, open *bool, branch *string, canModify *bool) error
	BotUser() (*github.UserData, error)
	FindIssues(query, sort string, asc bool) ([]github.Issue, error)
}

type ensureClient interface {
	updateClient
	AddLabel(org, repo string, number int, label string) error
	CreatePullRequest(org, repo, title, body, head, base string, canModify bool) (int, error)
	GetIssue(org, repo string, number int) (*github.Issue, error)
}

func UpdatePR(org, repo, title, body, headBranch string, gc updateClient) (*int, error) {
	logrus.Info("Looking for a PR to reuse...")
	me, err := gc.BotUser()
	if err != nil {
		return nil, fmt.Errorf("bot name: %v", err)
	}

	issues, err := gc.FindIssues("is:open is:pr archived:false repo:"+org+"/"+repo+" author:"+me.Login+" head:"+headBranch, "updated", false)
	if err != nil {
		return nil, fmt.Errorf("find issues: %v", err)
	} else if len(issues) == 0 {
		logrus.Info("No reusable issues found")
		return nil, nil
	}
	n := issues[0].Number
	logrus.Infof("Found %d", n)
	var ignoreOpen *bool
	var ignoreBranch *string
	var ignoreModify *bool
	if err := gc.UpdatePullRequest(org, repo, n, &title, &body, ignoreOpen, ignoreBranch, ignoreModify); err != nil {
		return nil, fmt.Errorf("update %d: %v", n, err)
	}

	return &n, nil
}

func EnsurePR(org, repo, title, body, source, branch, headBranch string, allowMods bool, gc ensureClient) (*int, error) {
	return EnsurePRWithLabels(org, repo, title, body, source, branch, headBranch, allowMods, gc, nil)
}

func EnsurePRWithLabels(org, repo, title, body, source, baseBranch, headBranch string, allowMods bool, gc ensureClient, labels []string) (*int, error) {
	n, err := UpdatePR(org, repo, title, body, headBranch, gc)
	if err != nil {
		return nil, fmt.Errorf("update error: %v", err)
	}
	if n == nil {
		pr, err := gc.CreatePullRequest(org, repo, title, body, source, baseBranch, allowMods)
		if err != nil {
			return nil, fmt.Errorf("create error: %v", err)
		}
		n = &pr
	}

	if len(labels) == 0 {
		return n, nil
	}

	issue, err := gc.GetIssue(org, repo, *n)
	if err != nil {
		return n, fmt.Errorf("failed to get PR: %v", err)
	}

	for _, label := range labels {
		if issue.HasLabel(label) {
			continue
		}

		if err := gc.AddLabel(org, repo, *n, label); err != nil {
			return n, fmt.Errorf("failed to add label %q: %v", label, err)
		}
		logrus.WithField("label", label).Info("Added label")
	}
	return n, nil
}
