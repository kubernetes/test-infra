/*
Copyright 2018 The Kubernetes Authors.

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

// Package override supports the /override context command.
package override

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const pluginName = "override"

var (
	overrideRe = regexp.MustCompile(`(?mi)^/override( (.+?)\s*)?$`)
)

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
	CreateStatus(org, repo, ref string, s github.Status) error
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	GetRef(org, repo, ref string) (string, error)
	HasPermission(org, repo, user string, role ...string) (bool, error)
	ListStatuses(org, repo, ref string) ([]github.Status, error)
}

type prowJobClient interface {
	Create(pj *prowapi.ProwJob) (*prowapi.ProwJob, error)
}

type overrideClient interface {
	githubClient
	prowJobClient
	presubmitForContext(org, repo, context string) *config.Presubmit
}

type client struct {
	gc            githubClient
	jc            config.JobConfig
	prowJobClient prowJobClient
}

func (c client) CreateComment(owner, repo string, number int, comment string) error {
	return c.gc.CreateComment(owner, repo, number, comment)
}
func (c client) CreateStatus(org, repo, ref string, s github.Status) error {
	return c.gc.CreateStatus(org, repo, ref, s)
}

func (c client) GetRef(org, repo, ref string) (string, error) {
	return c.gc.GetRef(org, repo, ref)
}

func (c client) GetPullRequest(org, repo string, number int) (*github.PullRequest, error) {
	return c.gc.GetPullRequest(org, repo, number)
}
func (c client) ListStatuses(org, repo, ref string) ([]github.Status, error) {
	return c.gc.ListStatuses(org, repo, ref)
}
func (c client) HasPermission(org, repo, user string, role ...string) (bool, error) {
	return c.gc.HasPermission(org, repo, user, role...)
}

func (c client) Create(pj *prowapi.ProwJob) (*prowapi.ProwJob, error) {
	return c.prowJobClient.Create(pj)
}

func (c client) presubmitForContext(org, repo, context string) *config.Presubmit {
	for _, p := range c.jc.AllPresubmits([]string{org + "/" + repo}) {
		if p.Context == context {
			return &p
		}
	}
	return nil
}

func init() {
	plugins.RegisterGenericCommentHandler(pluginName, handleGenericComment, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The override plugin allows repo admins to force a github status context to pass",
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/override [context]",
		Description: "Forces a github status context to green (one per line).",
		Featured:    false,
		WhoCanUse:   "Repo administrators",
		Examples:    []string{"/override pull-repo-whatever", "/override ci/circleci", "/override deleted-job"},
	})
	return pluginHelp, nil
}

func handleGenericComment(pc plugins.Agent, e github.GenericCommentEvent) error {
	c := client{
		gc:            pc.GitHubClient,
		jc:            pc.Config.JobConfig,
		prowJobClient: pc.ProwJobClient,
	}
	return handle(c, pc.Logger, &e)
}

func authorized(gc githubClient, log *logrus.Entry, org, repo, user string) bool {
	ok, err := gc.HasPermission(org, repo, user, github.RoleAdmin)
	if err != nil {
		log.WithError(err).Warnf("cannot determine whether %s is an admin of %s/%s", user, org, repo)
		return false
	}
	return ok
}

func description(user string) string {
	return fmt.Sprintf("Overridden by %s", user)
}

func formatList(list []string) string {
	var lines []string
	for _, item := range list {
		lines = append(lines, fmt.Sprintf(" - `%s`", item))
	}
	return strings.Join(lines, "\n")
}

func handle(oc overrideClient, log *logrus.Entry, e *github.GenericCommentEvent) error {

	if !e.IsPR || e.IssueState != "open" || e.Action != github.GenericCommentActionCreated {
		return nil
	}

	mat := overrideRe.FindAllStringSubmatch(e.Body, -1)
	if len(mat) == 0 {
		return nil // no /override commands given in the comment
	}

	org := e.Repo.Owner.Login
	repo := e.Repo.Name
	number := e.Number
	user := e.User.Login

	overrides := sets.NewString()
	for _, m := range mat {
		if m[1] == "" {
			resp := "/override requires a failed status context to operate on, but none was given"
			log.Debug(resp)
			return oc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, user, resp))
		}
		overrides.Insert(m[2])
	}

	if !authorized(oc, log, org, repo, user) {
		resp := fmt.Sprintf("%s unauthorized: /override is restricted to repo administrators", user)
		log.Debug(resp)
		return oc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, user, resp))
	}

	pr, err := oc.GetPullRequest(org, repo, number)
	if err != nil {
		resp := fmt.Sprintf("Cannot get PR #%d in %s/%s", number, org, repo)
		log.WithError(err).Warn(resp)
		return oc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, user, resp))
	}

	sha := pr.Head.SHA
	statuses, err := oc.ListStatuses(org, repo, sha)
	if err != nil {
		resp := fmt.Sprintf("Cannot get commit statuses for PR #%d in %s/%s", number, org, repo)
		log.WithError(err).Warn(resp)
		return oc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, user, resp))
	}

	contexts := sets.NewString()
	for _, status := range statuses {
		if status.State == github.StatusSuccess {
			continue
		}
		contexts.Insert(status.Context)
	}
	if unknown := overrides.Difference(contexts); unknown.Len() > 0 {
		resp := fmt.Sprintf(`/override requires a failed status context to operate on.
The following unknown contexts were given:
%s

Only the following contexts were expected:
%s`, formatList(unknown.List()), formatList(contexts.List()))
		log.Debug(resp)
		return oc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, user, resp))
	}

	done := sets.String{}

	defer func() {
		if len(done) == 0 {
			return
		}
		msg := fmt.Sprintf("Overrode contexts on behalf of %s: %s", user, strings.Join(done.List(), ", "))
		log.Info(msg)
		oc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, user, msg))
	}()

	for _, status := range statuses {
		if status.State == github.StatusSuccess || !overrides.Has(status.Context) {
			continue
		}
		// First create the overridden prow result if necessary
		if pre := oc.presubmitForContext(org, repo, status.Context); pre != nil {
			baseSHA, err := oc.GetRef(org, repo, "heads/"+pr.Base.Ref)
			if err != nil {
				resp := fmt.Sprintf("Cannot get base ref of PR")
				log.WithError(err).Warn(resp)
				return oc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, user, resp))
			}

			pj := pjutil.NewPresubmit(*pr, baseSHA, *pre, e.GUID)
			now := metav1.Now()
			pj.Status = prowapi.ProwJobStatus{
				StartTime:      now,
				CompletionTime: &now,
				State:          prowapi.SuccessState,
				Description:    description(user),
				URL:            e.HTMLURL,
			}
			log.WithFields(pjutil.ProwJobFields(&pj)).Info("Creating a new prowjob.")
			if _, err := oc.Create(&pj); err != nil {
				resp := fmt.Sprintf("Failed to create override job for %s", status.Context)
				log.WithError(err).Warn(resp)
				return oc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, user, resp))
			}
		}
		status.State = github.StatusSuccess
		status.Description = description(user)
		if err := oc.CreateStatus(org, repo, sha, status); err != nil {
			resp := fmt.Sprintf("Cannot update PR status for context %s", status.Context)
			log.WithError(err).Warn(resp)
			return oc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, user, resp))
		}
		done.Insert(status.Context)
	}
	return nil
}
