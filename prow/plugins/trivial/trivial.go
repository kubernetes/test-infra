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

package trivial

import (
	"fmt"
	"net/url"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const (
	pluginName = "trivial"
	// TODO: Change to Local
	trivialURL = "https://i.kym-cdn.com/entries/icons/original/000/028/021/work.jpg"
)

// TODO: Assign Trivial Tag
type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
}

func init() {
	plugins.RegisterGenericCommentHandler(pluginName, handleGenericComment, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The trivial plugin marks a PR as trivial in response to the `/trivial` command.",
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/trivial",
		Description: "Tags a PR as trivial",
		Featured:    false,
		WhoCanUse:   "Anyone",
		Examples:    []string{"/trivial"},
	})
	return pluginHelp, nil
}

func handleGenericComment(pc plugins.Agent, e github.GenericCommentEvent) error {
	return handle(
		pc.GitHubClient,
		pc.Logger,
		&e,
	)
}

func handle(gc githubClient, log *logrus.Entry, e *github.GenericCommentEvent) error {
	// Only consider new comments.
	if e.Action != github.GenericCommentActionCreated {
		return nil
	}

	org := e.Repo.Owner.Login
	repo := e.Repo.Name
	number := e.Number

	img, err := url.Parse(trivialURL)
	if err != nil {
		return fmt.Errorf("invalid image url %s: %v", trivialURL, err)
	}

	return gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, fmt.Sprintf("![trivial image](%s)", img)))
}
