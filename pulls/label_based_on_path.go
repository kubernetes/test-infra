/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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

package pulls

import (
	"fmt"
	"strings"

	github_util "k8s.io/contrib/github"
	"k8s.io/contrib/mungegithub/config"

	"github.com/golang/glog"
	"github.com/google/go-github/github"
	"github.com/spf13/cobra"
)

var (
	_        = fmt.Print
	labelMap = map[string]string{
		"docs/proposals":         "kind/design",
		"pkg/api/register.go":    "kind/new-api",
		"pkg/expapi/register.go": "kind/new-api",
		"pkg/api/types.go":       "kind/api-change",
		"pkg/expapi/types.go":    "kind/api-change",
	}
)

type PathLabelMunger struct{}

func init() {
	RegisterMungerOrDie(PathLabelMunger{})
}

func (PathLabelMunger) Name() string { return "path-label" }

func (PathLabelMunger) AddFlags(cmd *cobra.Command) {}

func (PathLabelMunger) MungePullRequest(config *config.MungeConfig, pr *github.PullRequest, issue *github.Issue, commits []github.RepositoryCommit, events []github.IssueEvent) {
	glog.V(8).Infof("Checking out PR %d\n", *pr.Number)
	needsLabels := []string{}
	for _, c := range commits {
		for _, f := range c.Files {
			for prefix, label := range labelMap {
				if strings.HasPrefix(*f.Filename, prefix) && !github_util.HasLabel(issue.Labels, label) {
					needsLabels = append(needsLabels, label)
				}
			}
		}
	}

	if len(needsLabels) != 0 {
		config.AddLabels(*pr.Number, needsLabels)
	}
}
