/*
Copyright 2022 The Kubernetes Authors.

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

package testfreeze

import (
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/testfreeze/checker"
)

const (
	PluginName                  = "testfreeze"
	defaultKubernetesBranch     = "master"
	defaultKubernetesRepoAndOrg = "kubernetes"
	templateString              = `Please note that we're already in [Test Freeze](https://github.com/kubernetes/sig-release/blob/master/releases/release_phases.md#test-freeze) for the ` + "`{{ .Branch }}`" + ` branch. This means every merged PR will be automatically fast-forwarded via the periodic [ci-fast-forward](https://testgrid.k8s.io/sig-release-releng-blocking#git-repo-kubernetes-fast-forward) job to the release branch of the upcoming {{ .Tag }} release.

Fast forwards are scheduled to happen every 6 hours, whereas the most recent run was: {{ .LastFastForward }}.
`
)

func init() {
	plugins.RegisterPullRequestHandler(PluginName, handlePullRequestEvent, helpProvider)
}

func helpProvider(*plugins.Configuration, []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	return &pluginhelp.PluginHelp{
		Description: fmt.Sprintf(
			"The %s plugin adds additional documentation about cherry-picks during the Test Freeze period.",
			PluginName,
		),
	}, nil
}

func handlePullRequestEvent(p plugins.Agent, e github.PullRequestEvent) error {
	h := newHandler()
	log := p.Logger
	if err := h.handle(
		log,
		p.GitHubClient,
		e.Action,
		e.Number,
		e.Repo.Owner.Login,
		e.Repo.Name,
		e.PullRequest.Base.Ref,
	); err != nil {
		log.WithError(err).Error("skipping")
	}
	return nil
}

type handler struct {
	verifier verifier
}

func newHandler() *handler {
	return &handler{
		verifier: &defaultVerifier{},
	}
}

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate . verifier
type verifier interface {
	CheckInTestFreeze(*logrus.Entry) (*checker.Result, error)
	CreateComment(plugins.PluginGitHubClient, string, string, int, string) error
}

type defaultVerifier struct{}

func (*defaultVerifier) CheckInTestFreeze(log *logrus.Entry) (*checker.Result, error) {
	return checker.New(log).InTestFreeze()
}

func (*defaultVerifier) CreateComment(
	client plugins.PluginGitHubClient,
	org, repo string,
	number int,
	comment string,
) error {
	return client.CreateComment(org, repo, number, comment)
}

func (h *handler) handle(
	log *logrus.Entry,
	client plugins.PluginGitHubClient,
	action github.PullRequestEventAction,
	number int,
	org, repo, branch string,
) error {
	funcStart := time.Now()
	defer func() {
		log.WithField("duration", time.Since(funcStart).String()).
			Debug("Completed handlePullRequest")
	}()

	if action != github.PullRequestActionOpened &&
		action != github.PullRequestActionReopened {
		log.Debugf("Skipping pull request action %s", action)
		return nil
	}

	if org != defaultKubernetesRepoAndOrg ||
		repo != defaultKubernetesRepoAndOrg ||
		branch != defaultKubernetesBranch {
		log.Debug("Skipping non k/k master branch PR")
		return nil
	}

	result, err := h.verifier.CheckInTestFreeze(log)
	if err != nil {
		return fmt.Errorf("get test freeze result: %w", err)
	}

	if !result.InTestFreeze {
		log.Debugf("Not in test freeze, skipping")
		return nil
	}

	comment := &strings.Builder{}
	tpl, err := template.New(PluginName).Parse(templateString)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}
	if err := tpl.Execute(comment, result); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}

	if err := h.verifier.CreateComment(
		client, org, repo, number, comment.String(),
	); err != nil {
		return fmt.Errorf("create comment on %s/%s#%d: %q: %w", org, repo, number, comment, err)
	}

	return nil
}
