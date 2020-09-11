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
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/repoowners"
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
	ListTeams(org string) ([]github.Team, error)
	ListTeamMembers(id int, role string) ([]github.TeamMember, error)
}

type prowJobClient interface {
	Create(context.Context, *prowapi.ProwJob, metav1.CreateOptions) (*prowapi.ProwJob, error)
}

type ownersClient interface {
	LoadRepoOwners(org, repo, base string) (repoowners.RepoOwner, error)
}

type overrideClient interface {
	githubClient
	prowJobClient
	ownersClient
	presubmits(org, repo string, baseSHAGetter config.RefGetter, headSHA string) ([]config.Presubmit, error)
}

type client struct {
	ghc           githubClient
	gc            git.ClientFactory
	config        *config.Config
	ownersClient  ownersClient
	prowJobClient prowJobClient
}

func (c client) CreateComment(owner, repo string, number int, comment string) error {
	return c.ghc.CreateComment(owner, repo, number, comment)
}
func (c client) CreateStatus(org, repo, ref string, s github.Status) error {
	return c.ghc.CreateStatus(org, repo, ref, s)
}

func (c client) GetRef(org, repo, ref string) (string, error) {
	return c.ghc.GetRef(org, repo, ref)
}

func (c client) GetPullRequest(org, repo string, number int) (*github.PullRequest, error) {
	return c.ghc.GetPullRequest(org, repo, number)
}
func (c client) ListStatuses(org, repo, ref string) ([]github.Status, error) {
	return c.ghc.ListStatuses(org, repo, ref)
}
func (c client) HasPermission(org, repo, user string, role ...string) (bool, error) {
	return c.ghc.HasPermission(org, repo, user, role...)
}
func (c client) ListTeams(org string) ([]github.Team, error) {
	return c.ghc.ListTeams(org)
}
func (c client) ListTeamMembers(id int, role string) ([]github.TeamMember, error) {
	return c.ghc.ListTeamMembers(id, role)
}

func (c client) Create(ctx context.Context, pj *prowapi.ProwJob, o metav1.CreateOptions) (*prowapi.ProwJob, error) {
	return c.prowJobClient.Create(ctx, pj, o)
}

func (c client) presubmits(org, repo string, baseSHAGetter config.RefGetter, headSHA string) ([]config.Presubmit, error) {
	headSHAGetter := func() (string, error) {
		return headSHA, nil
	}
	presubmits, err := c.config.GetPresubmits(c.gc, org+"/"+repo, baseSHAGetter, headSHAGetter)
	if err != nil {
		return nil, fmt.Errorf("failed to get presubmits: %v", err)
	}
	return presubmits, nil
}

func presubmitForContext(presubmits []config.Presubmit, context string) *config.Presubmit {
	for _, p := range presubmits {
		if p.Context == context {
			return &p
		}
	}
	return nil
}

func (c client) LoadRepoOwners(org, repo, base string) (repoowners.RepoOwner, error) {
	return c.ownersClient.LoadRepoOwners(org, repo, base)
}

func init() {
	plugins.RegisterGenericCommentHandler(pluginName, handleGenericComment, helpProvider)
}

func helpProvider(config *plugins.Configuration, _ []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The override plugin allows repo admins to force a github status context to pass",
	}
	overrideConfig := plugins.Override{}
	if config != nil {
		overrideConfig = config.Override
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/override [context]",
		Description: "Forces a github status context to green (one per line).",
		Featured:    false,
		WhoCanUse:   whoCanUse(overrideConfig, "", ""),
		Examples:    []string{"/override pull-repo-whatever", "/override ci/circleci", "/override deleted-job"},
	})
	return pluginHelp, nil
}

func whoCanUse(overrideConfig plugins.Override, org, repo string) string {
	admins := "Repo administrators"
	owners := ""
	teams := ""

	if overrideConfig.AllowTopLevelOwners {
		owners = ", approvers in top level OWNERS file"
	}

	if len(overrideConfig.AllowedGitHubTeams) > 0 {
		repoRef := fmt.Sprintf("%s/%s", org, repo)
		var allTeams []string
		for r, allowedTeams := range overrideConfig.AllowedGitHubTeams {
			if repoRef == "/" || r == repoRef {
				allTeams = append(allTeams, fmt.Sprintf("%s: %s", r, strings.Join(allowedTeams, " ")))
			}
		}
		teams = ", and the following github teams:" + strings.Join(allTeams, ", ")
	}

	return admins + owners + teams + "."
}

func handleGenericComment(pc plugins.Agent, e github.GenericCommentEvent) error {
	c := client{
		gc:            pc.GitClient,
		ghc:           pc.GitHubClient,
		config:        pc.Config,
		prowJobClient: pc.ProwJobClient,
		ownersClient:  pc.OwnersClient,
	}
	return handle(c, pc.Logger, &e, pc.PluginConfig.Override)
}

func authorizedUser(gc githubClient, log *logrus.Entry, org, repo, user string) bool {
	ok, err := gc.HasPermission(org, repo, user, github.RoleAdmin)
	if err != nil {
		log.WithError(err).Warnf("cannot determine whether %s is an admin of %s/%s", user, org, repo)
		return false
	}
	return ok
}

func authorizedTopLevelOwner(oc ownersClient, allowTopLevelOwners bool, log *logrus.Entry, org, repo, user string, pr *github.PullRequest) bool {
	if allowTopLevelOwners {
		owners, err := oc.LoadRepoOwners(org, repo, pr.Base.Ref)
		if err != nil {
			log.WithError(err).Warnf("cannot determine whether %s is a top level owner of %s/%s", user, org, repo)
			return false
		}
		return owners.TopLevelApprovers().Has(github.NormLogin(user))
	}
	return false
}

func validateGitHubTeamSlugs(teamSlugs map[string][]string, org, repo string, githubTeams []github.Team) error {
	validSlugs := sets.NewString()
	for _, team := range githubTeams {
		validSlugs.Insert(team.Slug)
	}
	invalidSlugs := sets.NewString(teamSlugs[fmt.Sprintf("%s/%s", org, repo)]...).Difference(validSlugs)

	if invalidSlugs.Len() > 0 {
		return fmt.Errorf("invalid team slug(s): %s", strings.Join(invalidSlugs.List(), ","))
	}
	return nil
}

func authorizedGitHubTeamMember(gc githubClient, log *logrus.Entry, teamSlugs map[string][]string, org, repo, user string) bool {
	teams, err := gc.ListTeams(org)
	if err != nil {
		log.WithError(err).Warnf("cannot get list of teams for org %s", org)
		return false
	}
	if err := validateGitHubTeamSlugs(teamSlugs, org, repo, teams); err != nil {
		log.WithError(err).Warnf("invalid team slug(s)")
	}

	for _, slug := range teamSlugs[fmt.Sprintf("%s/%s", org, repo)] {
		for _, team := range teams {
			if team.Slug == slug {
				members, err := gc.ListTeamMembers(team.ID, github.RoleAll)
				if err != nil {
					log.WithError(err).Warnf("cannot find members of team %s in org %s", slug, org)
					continue
				}
				for _, member := range members {
					if member.Login == user {
						return true
					}
				}
			}
		}
	}
	return false
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

func handle(oc overrideClient, log *logrus.Entry, e *github.GenericCommentEvent, options plugins.Override) error {

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
		overrides.Insert(strings.TrimSpace(m[2]))
	}

	authorized := authorizedUser(oc, log, org, repo, user)
	if !authorized && len(options.AllowedGitHubTeams) > 0 {
		authorized = authorizedGitHubTeamMember(oc, log, options.AllowedGitHubTeams, org, repo, user)
	}
	if !authorized && !options.AllowTopLevelOwners {
		resp := fmt.Sprintf("%s unauthorized: /override is restricted to %s", user, whoCanUse(options, org, repo))
		log.Debug(resp)
		return oc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, user, resp))
	}

	pr, err := oc.GetPullRequest(org, repo, number)
	if err != nil {
		resp := fmt.Sprintf("Cannot get PR #%d in %s/%s", number, org, repo)
		log.WithError(err).Warn(resp)
		return oc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, user, resp))
	}

	if !authorized && !authorizedTopLevelOwner(oc, options.AllowTopLevelOwners, log, org, repo, user, pr) {
		resp := fmt.Sprintf("%s unauthorized: /override is restricted to %s", user, whoCanUse(options, org, repo))
		log.Debug(resp)
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

	baseSHAGetter := shaGetterFactory(oc, org, repo, pr.Base.Ref)
	presubmits, err := oc.presubmits(org, repo, baseSHAGetter, sha)
	if err != nil {
		msg := fmt.Sprintf("Failed to get presubmits")
		log.WithError(err).Error(msg)
		return oc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, user, msg))
	}
	for _, status := range statuses {
		if status.State == github.StatusSuccess || !overrides.Has(status.Context) {
			continue
		}
		// First create the overridden prow result if necessary
		pre := presubmitForContext(presubmits, status.Context)
		if pre != nil {
			baseSHA, err := baseSHAGetter()
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
			if _, err := oc.Create(context.TODO(), &pj, metav1.CreateOptions{}); err != nil {
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

// shaGetterFactory is a closure to retrieve a sha once. It is not threadsafe.
func shaGetterFactory(oc overrideClient, org, repo, ref string) func() (string, error) {
	var baseSHA string
	return func() (string, error) {
		if baseSHA != "" {
			return baseSHA, nil
		}
		var err error
		baseSHA, err = oc.GetRef(org, repo, "heads/"+ref)
		return baseSHA, err
	}
}
