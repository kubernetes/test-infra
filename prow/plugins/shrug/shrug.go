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

package shrug

import (
	"fmt"
	"regexp"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

const pluginName = "shrug"

var (
	shrugLabel = "¯\\_(ツ)_/¯"
	shrugRe    = regexp.MustCompile(`(?mi)^/shrug\s*$`)
	unshrugRe  = regexp.MustCompile(`(?mi)^/unshrug\s*$`)
)

type event struct {
	org           string
	repo          string
	number        int
	prAuthor      string
	commentAuthor string
	body          string
	assignees     []github.User
	hasLabel      func(label string) (bool, error)
	htmlurl       string
}

func init() {
	plugins.RegisterGenericCommentHandler(pluginName, handleGenericComment, nil)
}

type githubClient interface {
	AddLabel(owner, repo string, number int, label string) error
	CreateComment(owner, repo string, number int, comment string) error
	RemoveLabel(owner, repo string, number int, label string) error
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
}

func handleGenericComment(pc plugins.PluginClient, e github.GenericCommentEvent) error {
	return handle(pc.GitHubClient, pc.Logger, &e)
}

func handle(gc githubClient, log *logrus.Entry, e *github.GenericCommentEvent) error {
	if e.Action != github.GenericCommentActionCreated {
		return nil
	}

	wantShrug := false
	if shrugRe.MatchString(e.Body) {
		wantShrug = true
	} else if unshrugRe.MatchString(e.Body) {
		wantShrug = false
	} else {
		return nil
	}

	org := e.Repo.Owner.Login
	repo := e.Repo.Name

	// Only add the label if it doesn't have it yet.
	hasShrug := false
	labels, err := gc.GetIssueLabels(org, repo, e.Number)
	if err != nil {
		log.WithError(err).Errorf("Failed to get the labels on %s/%s#%d.", org, repo, e.Number)
	}
	for _, candidate := range labels {
		if candidate.Name == shrugLabel {
			hasShrug = true
			break
		}
	}
	if hasShrug && !wantShrug {
		log.Info("Removing Shrug label.")
		resp := "¯\\\\\\_(ツ)\\_/¯"
		log.Infof("Commenting with \"%s\".", resp)
		if err := gc.CreateComment(org, repo, e.Number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, resp)); err != nil {
			return fmt.Errorf("failed to comment on %s/%s#%d: %v", org, repo, e.Number, err)
		}
		return gc.RemoveLabel(org, repo, e.Number, shrugLabel)
	} else if !hasShrug && wantShrug {
		log.Info("Adding Shrug label.")
		return gc.AddLabel(org, repo, e.Number, shrugLabel)
	}
	return nil
}
