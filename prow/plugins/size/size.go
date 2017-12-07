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

// Package size contains a Prow plugin which counts the number of lines changed
// in a pull request, buckets this number into a few size classes (S, L, XL, etc),
// and finally labels the pull request with this size.
package size

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/genfiles"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const pluginName = "size"

func init() {
	plugins.RegisterPullRequestHandler(pluginName, handlePullRequest, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	// Only the Description field is specified because this plugin is not triggered with commands and is not configurable.
	return &pluginhelp.PluginHelp{
			Description: `The size plugin manages the 'size/*' labels, maintaining the appropriate label on each pull request as it is updated. Generated files identified by the config file '.generated_files' at the repo root are ignored. Labels are applied based on the total number of lines of changes (additions and deletions):<ul>
<li>size/XS:	0-9</li>
<li>size/S:	10-29</li>
<li>size/M:	30-99</li>
<li>size/L	100-499</li>
<li>size/XL:	500-999</li>
<li>size/XXL:	1000+</li>
</ul>`,
		},
		nil
}

func handlePullRequest(pc plugins.PluginClient, pe github.PullRequestEvent) error {
	return handlePR(pc.GitHubClient, pc.Logger, pe)
}

// Strict subset of *github.Client methods.
type githubClient interface {
	AddLabel(owner, repo string, number int, label string) error
	RemoveLabel(owner, repo string, number int, label string) error
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
	GetFile(org, repo, filepath, commit string) ([]byte, error)
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
}

func handlePR(gc githubClient, le *logrus.Entry, pe github.PullRequestEvent) error {
	if !isPRChanged(pe) {
		return nil
	}

	var (
		owner = pe.PullRequest.Base.Repo.Owner.Login
		repo  = pe.PullRequest.Base.Repo.Name
		num   = pe.PullRequest.Number
		sha   = pe.PullRequest.Base.SHA
	)

	g, err := genfiles.NewGroup(gc, owner, repo, sha)
	if err != nil {
		switch err.(type) {
		case *genfiles.ParseError:
			// Continue on parse errors, but warn that something is wrong.
			le.Warnf("error while parsing .generated_files: %v", err)
		default:
			return err
		}
	}

	changes, err := gc.GetPullRequestChanges(owner, repo, num)
	if err != nil {
		return fmt.Errorf("can not get PR changes for size plugin: %v", err)
	}

	var count int
	for _, change := range changes {
		if g.Match(change.Filename) {
			continue
		}

		count += change.Additions + change.Deletions
	}

	labels, err := gc.GetIssueLabels(owner, repo, num)
	if err != nil {
		le.Warnf("while retrieving labels, error: %v", err)
	}

	newLabel := bucket(count).label()
	var hasLabel bool

	for _, label := range labels {
		if label.Name == newLabel {
			hasLabel = true
			continue
		}

		if strings.HasPrefix(label.Name, labelPrefix) {
			if err := gc.RemoveLabel(owner, repo, num, label.Name); err != nil {
				le.Warnf("error while removing label %q: %v", label.Name, err)
			}
		}
	}

	if hasLabel {
		return nil
	}

	if err := gc.AddLabel(owner, repo, num, newLabel); err != nil {
		return fmt.Errorf("error adding label to %s/%s PR #%d: %v", owner, repo, num, err)
	}

	return nil
}

// One of a set of discrete buckets.
type size int

const (
	sizeXS size = iota
	sizeS
	sizeM
	sizeL
	sizeXL
	sizeXXL
)

const (
	labelPrefix = "size/"

	labelXS     = "size/XS"
	labelS      = "size/S"
	labelM      = "size/M"
	labelL      = "size/L"
	labelXL     = "size/XL"
	labelXXL    = "size/XXL"
	labelUnkown = "size/?"
)

func (s size) label() string {
	switch s {
	case sizeXS:
		return labelXS
	case sizeS:
		return labelS
	case sizeM:
		return labelM
	case sizeL:
		return labelL
	case sizeXL:
		return labelXL
	case sizeXXL:
		return labelXXL
	}

	return labelUnkown
}

func bucket(lineCount int) size {
	if lineCount < 10 {
		return sizeXS
	} else if lineCount < 30 {
		return sizeS
	} else if lineCount < 100 {
		return sizeM
	} else if lineCount < 500 {
		return sizeL
	} else if lineCount < 1000 {
		return sizeXL
	}

	return sizeXXL
}

// These are the only actions indicating the code diffs may have changed.
func isPRChanged(pe github.PullRequestEvent) bool {
	switch pe.Action {
	case github.PullRequestActionOpened:
		return true
	case github.PullRequestActionReopened:
		return true
	case github.PullRequestActionSynchronize:
		return true
	case github.PullRequestActionEdited:
		return true
	default:
		return false
	}
}
