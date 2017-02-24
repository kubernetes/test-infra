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

package label

import (
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"

	"fmt"
	"regexp"
	"strings"
)

const pluginName = "label"

var (
	allowedPrefixes = []string{"area", "priority"}
	regexBuilder    = `(?m)^/%s[\t\s]+(?:\S*(?:[^\n]+))*`
	labelFailed     = "The following labels: %v could not be added to the issue or PR"
)

func init() {
	plugins.RegisterIssueCommentHandler(pluginName, handleIssueComment)
}

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
	IsMember(org, user string) (bool, error)
	AddLabel(owner, repo string, number int, label string) error
	RemoveLabel(owner, repo string, number int, label string) error
}

func handleIssueComment(pc plugins.PluginClient, ic github.IssueCommentEvent) error {
	return handle(pc.GitHubClient, ic)
}

func handle(gc githubClient, ic github.IssueCommentEvent) error {
	commenter := ic.Comment.User.Login
	owner := ic.Repo.Owner.Name
	repo := ic.Repo.Name
	number := ic.Issue.Number

	member, err := gc.IsMember(repo, commenter)
	if err != nil {
		return fmt.Errorf("IsMember failed: %v", err)
	}
	if !member {
		return gc.CreateComment(owner, repo, number, plugins.FormatResponse(ic.Comment, "Only kubernetes org members can add labels."))
	}

	var failures []string
	for _, pref := range allowedPrefixes {
		labelRegex := regexp.MustCompile(fmt.Sprintf(regexBuilder, pref))
		match := labelRegex.FindStringSubmatch(ic.Comment.Body)
		if match == nil || len(match) == 0 {
			continue
		}
		for _, newLabel := range strings.Split(match[0], " ")[1:] {
			newLabel = strings.TrimSpace(newLabel)
			if ic.Issue.HasLabel(newLabel) {
				continue
			}
			if strings.HasPrefix(newLabel, pref) {
				if err := gc.AddLabel(owner, repo, number, newLabel); err != nil {
					failures = append(failures, newLabel)
				}
			} else {
				failures = append(failures, newLabel)
			}
		}
	}

	if len(failures) > 0 {
		gc.CreateComment(owner, repo, number, fmt.Sprintf(labelFailed, strings.Join(failures, ", "), number))
	}

	return nil
}
