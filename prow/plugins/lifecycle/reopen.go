/*
Copyright 2017 The Kubernetes Authors.

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

package lifecycle

import (
	"fmt"
	"regexp"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

var reopenRe = regexp.MustCompile(`(?mi)^/reopen\s*$`)

func init() {
	plugins.RegisterGenericCommentHandler("reopen", deprecatedHandleReopenComment, reopenHelp)
}

func reopenHelp(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	// The Config field is omitted because this plugin is not configurable.
	return &pluginhelp.PluginHelp{
			Description: "Deprecated! Please use the lifecycle plugin instead of reopen.",
		},
		nil
}

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
	ReopenIssue(owner, repo string, number int) error
	ReopenPR(owner, repo string, number int) error
}

func deprecatedHandleReopenComment(pc plugins.PluginClient, e github.GenericCommentEvent) error {
	warn := true
	return handleReopen(pc.GitHubClient, pc.Logger, &e, warn)
}

func handleReopen(gc githubClient, log *logrus.Entry, e *github.GenericCommentEvent, warn bool) error {
	// Only consider closed issues and new comments.
	if e.IssueState != "closed" || e.Action != github.GenericCommentActionCreated {
		return nil
	}

	if !reopenRe.MatchString(e.Body) {
		return nil
	}

	org := e.Repo.Owner.Login
	repo := e.Repo.Name
	number := e.Number
	commentAuthor := e.User.Login

	// Allow assignees and authors to re-open issues.
	isAssignee := false
	for _, assignee := range e.Assignees {
		if commentAuthor == assignee.Login {
			isAssignee = true
			break
		}
	}
	if e.IssueAuthor.Login != commentAuthor && !isAssignee {
		resp := "you can't re-open an issue/PR unless you authored it or you are assigned to it."
		log.Infof("Commenting \"%s\".", resp)
		return gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, resp))
	}

	if warn {
		if err := deprecate(gc, "reopen", org, repo, number, e); err != nil {
			return err
		}
	}

	if e.IsPR {
		log.Infof("/reopen PR")
		err := gc.ReopenPR(org, repo, number)
		if err != nil {
			if scbc, ok := err.(github.StateCannotBeChanged); ok {
				resp := fmt.Sprintf("failed to re-open PR: %v", scbc)
				return gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, resp))
			}
		}
		return err
	}

	log.Infof("/reopen issue")
	err := gc.ReopenIssue(org, repo, number)
	if err != nil {
		if scbc, ok := err.(github.StateCannotBeChanged); ok {
			resp := fmt.Sprintf("failed to re-open Issue: %v", scbc)
			return gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, resp))
		}
	}
	return err
}
