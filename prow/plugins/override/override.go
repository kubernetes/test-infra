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
	"sort"
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
	overrideRe = regexp.MustCompile(`(?mi)^/override( ([^\r\n]+))?[\r\n]?$`)
)

type Context struct {
	Context     string
	Description string
	State       string
}

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
	CreateStatus(org, repo, ref string, s github.Status) error
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	GetRef(org, repo, ref string) (string, error)
	HasPermission(org, repo, user string, role ...string) (bool, error)
	ListStatuses(org, repo, ref string) ([]github.Status, error)
	GetBranchProtection(org, repo, branch string) (*github.BranchProtection, error)
	ListTeams(org string) ([]github.Team, error)
	ListTeamMembersBySlug(org, teamSlug, role string) ([]github.TeamMember, error)
	ListCheckRuns(org, repo, ref string) (*github.CheckRunList, error)
	CreateCheckRun(org, repo string, checkRun github.CheckRun) error
	UsesAppAuth() bool
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
func (c client) GetBranchProtection(org, repo, branch string) (*github.BranchProtection, error) {
	return c.ghc.GetBranchProtection(org, repo, branch)
}
func (c client) HasPermission(org, repo, user string, role ...string) (bool, error) {
	return c.ghc.HasPermission(org, repo, user, role...)
}
func (c client) ListTeams(org string) ([]github.Team, error) {
	return c.ghc.ListTeams(org)
}
func (c client) ListTeamMembersBySlug(org, teamSlug, role string) ([]github.TeamMember, error) {
	return c.ghc.ListTeamMembersBySlug(org, teamSlug, role)
}
func (c client) ListCheckRuns(org, teamSlug, role string) (*github.CheckRunList, error) {
	return c.ghc.ListCheckRuns(org, teamSlug, role)
}

func (c client) CreateCheckRun(org, repo string, checkRun github.CheckRun) error {
	return c.ghc.CreateCheckRun(org, repo, checkRun)
}

func (c client) UsesAppAuth() bool {
	return c.ghc.UsesAppAuth()
}

func (c client) Create(ctx context.Context, pj *prowapi.ProwJob, o metav1.CreateOptions) (*prowapi.ProwJob, error) {
	return c.prowJobClient.Create(ctx, pj, o)
}

func (c client) presubmits(org, repo string, baseSHAGetter config.RefGetter, headSHA string) ([]config.Presubmit, error) {
	headSHAGetter := func() (string, error) {
		return headSHA, nil
	}
	presubmits, err := c.config.GetPresubmits(c.gc, org+"/"+repo, "", baseSHAGetter, headSHAGetter)
	if err != nil {
		return nil, fmt.Errorf("failed to get presubmits: %w", err)
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
	yamlSnippet, err := plugins.CommentMap.GenYaml(&plugins.Configuration{
		Override: plugins.Override{
			AllowTopLevelOwners: true,
			AllowedGitHubTeams: map[string][]string{
				"kubernetes/kubernetes": {"team1", "team2"},
			},
		},
	})
	if err != nil {
		logrus.WithError(err).Warnf("cannot generate comments for %s plugin", pluginName)
	}
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The override plugin allows repo admins to force a github status context to pass",
		Snippet:     yamlSnippet,
	}
	overrideConfig := plugins.Override{}
	if config != nil {
		overrideConfig = config.Override
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/override [context1] [context2]",
		Description: "Forces github status contexts to green (multiple can be given). If the desired context has spaces, it must be quoted.",
		Featured:    false,
		WhoCanUse:   whoCanUse(overrideConfig, "", ""),
		Examples:    []string{"/override pull-repo-whatever", "/override \"test / Unit Tests\"", "/override ci/circleci", "/override deleted-job other-job"},
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
			if repoRef == "/" || r == repoRef || r == org {
				allTeams = append(allTeams, fmt.Sprintf("%s: %s", r, strings.Join(allowedTeams, " ")))
			}
		}
		sort.Strings(allTeams)
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
	validSlugs := sets.New[string]()
	for _, team := range githubTeams {
		validSlugs.Insert(team.Slug)
	}
	invalidSlugs := sets.New[string](teamSlugs[fmt.Sprintf("%s/%s", org, repo)]...).Difference(validSlugs)

	if invalidSlugs.Len() > 0 {
		return fmt.Errorf("invalid team slug(s): %s", strings.Join(sets.List(invalidSlugs), ","))
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

	slugs := teamSlugs[fmt.Sprintf("%s/%s", org, repo)]
	slugs = append(slugs, teamSlugs[org]...)
	for _, slug := range slugs {
		members, err := gc.ListTeamMembersBySlug(org, slug, github.RoleAll)
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

type descriptionAndState struct {
	description string
	state       string
}

// Parses /override foo something
func parseOverrideInput(in string) []string {
	quoted := false
	f := strings.FieldsFunc(in, func(r rune) bool {
		if r == '"' {
			quoted = !quoted
		}
		return !quoted && r == ' '
	})
	var retval []string
	for _, val := range f {
		retval = append(retval, strings.Trim(strings.TrimSpace(val), `"`))
	}

	return retval
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

	overrides := sets.New[string]()
	for _, m := range mat {
		if m[1] == "" {
			resp := "/override requires failed status contexts to operate on, but none was given"
			log.Debug(resp)
			return oc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, user, resp))
		}
		overrides.Insert(parseOverrideInput(m[2])...)
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

	// Get CheckRuns and add them to contexts
	var checkruns *github.CheckRunList
	var checkrunContexts []Context
	if oc.UsesAppAuth() {
		checkruns, err = oc.ListCheckRuns(org, repo, sha)
		if err != nil {
			resp := fmt.Sprintf("Cannot get commit checkruns for PR #%d in %s/%s", number, org, repo)
			log.WithError(err).Warn(resp)
			return oc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, user, resp))
		}

		checkrunContexts = make([]Context, len(checkruns.CheckRuns))
		for _, checkrun := range checkruns.CheckRuns {
			var state string
			if checkrun.CompletedAt == "" {
				state = "PENDING"
			} else if strings.ToUpper(checkrun.Conclusion) == "NEUTRAL" {
				state = "SUCCESS"
			} else {
				state = strings.ToUpper(checkrun.Conclusion)
			}
			checkrunContexts = append(checkrunContexts, Context{
				Context:     checkrun.Name,
				Description: checkrun.DetailsURL,
				State:       state,
			})
		}

		// dedupe checkruns and pick the best one
		checkrunContexts = deduplicateContexts(checkrunContexts)
	}

	baseSHAGetter := shaGetterFactory(oc, org, repo, pr.Base.Ref)
	presubmits, err := oc.presubmits(org, repo, baseSHAGetter, sha)
	if err != nil {
		msg := "Failed to get presubmits"
		log.WithError(err).Error(msg)
		return oc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, user, msg))
	}

	contexts := sets.New[string]()
	for _, status := range statuses {
		if status.State == github.StatusSuccess {
			continue
		}

		contexts.Insert(status.Context)

		for _, job := range presubmits {
			if job.Context == status.Context {
				contexts.Insert(job.Name)
				break
			}
		}
	}

	// add all checkruns that are not successful or pending to the list of contexts being tracked
	for _, cr := range checkrunContexts {
		if cr.Context != "" && cr.State != "SUCCESS" && cr.State != "PENDING" {
			contexts.Insert(cr.Context)
		}
	}

	branch := pr.Base.Ref
	branchProtection, err := oc.GetBranchProtection(org, repo, branch)
	if err != nil {
		resp := fmt.Sprintf("Cannot get branch protection for branch %s in %s/%s", branch, org, repo)
		log.WithError(err).Warn(resp)
		return oc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, user, resp))
	}

	if branchProtection != nil && branchProtection.RequiredStatusChecks != nil {
		for _, context := range branchProtection.RequiredStatusChecks.Contexts {
			if !contexts.Has(context) {
				contexts.Insert(context)
				statuses = append(statuses, github.Status{Context: context})
			}
		}
	}

	if unknown := overrides.Difference(contexts); unknown.Len() > 0 {
		resp := fmt.Sprintf(`/override requires failed status contexts, check run or a prowjob name to operate on.
The following unknown contexts/checkruns were given:
%s

Only the following failed contexts/checkruns were expected:
%s

If you are trying to override a checkrun that has a space in it, you must put a double quote on the context.
`, formatList(sets.List(unknown)), formatList(sets.List(contexts)))
		log.Debug(resp)
		return oc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, user, resp))
	}

	done := sets.Set[string]{}
	contextsWithCreatedJobs := sets.Set[string]{}

	defer func() {
		if len(done) == 0 {
			return
		}
		msg := fmt.Sprintf("Overrode contexts on behalf of %s: %s", user, strings.Join(sets.List(done), ", "))
		log.Info(msg)
		oc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, user, msg))
	}()

	for _, status := range statuses {
		pre := presubmitForContext(presubmits, status.Context)
		if status.State == github.StatusSuccess || !(overrides.Has(status.Context) || pre != nil && overrides.Has(pre.Name)) || contextsWithCreatedJobs.Has(status.Context) {
			continue
		}

		// Create the overridden prow result if necessary
		if pre != nil {
			baseSHA, err := baseSHAGetter()
			if err != nil {
				resp := "Cannot get base ref of PR"
				log.WithError(err).Warn(resp)
				return oc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, user, resp))
			}

			pj := pjutil.NewPresubmit(*pr, baseSHA, *pre, e.GUID, nil)
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
			contextsWithCreatedJobs.Insert(status.Context)
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

	// We want to interate over the checkrunContexts, create a new checkrun with the same name as the context and mark it as successful.
	// Tide has logic to pick the best checkrun result
	// Checkruns have been converted to contexts and deduped
	if oc.UsesAppAuth() {
		for _, checkrun := range checkrunContexts {
			if overrides.Has(checkrun.Context) {
				prowOverrideCR := github.CheckRun{
					Name:       checkrun.Context,
					HeadSHA:    sha,
					Status:     "completed",
					Conclusion: "success",
					Output: github.CheckRunOutput{
						Title:   fmt.Sprintf("Prow override - %s", checkrun.Context),
						Summary: fmt.Sprintf("Prow has received override command for the %s checkrun.", checkrun.Context),
					},
				}
				if err := oc.CreateCheckRun(org, repo, prowOverrideCR); err != nil {
					resp := fmt.Sprintf("cannot create prow-override CheckRun %v", prowOverrideCR)
					log.WithError(err).Warn(resp)
					return oc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, user, resp))
				}
				done.Insert(checkrun.Context)
			}
		}
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

func isStateBetter(previous, current string) bool {
	if current == "SUCCESS" {
		return true
	}
	if current == "PENDING" && (previous == "ERROR" || previous == "FAILURE" || previous == "EXPECTED") {
		return true
	}
	if previous == "EXPECTED" && (current == "ERROR" || current == "FAILURE") {
		return true
	}

	return false
}

// This function deduplicates checkruns and picks the best result
func deduplicateContexts(contexts []Context) []Context {
	result := map[string]descriptionAndState{}
	for _, context := range contexts {
		previousResult, found := result[context.Context]
		if !found {
			result[context.Context] = descriptionAndState{description: context.Description, state: context.State}
			continue
		}
		if isStateBetter(previousResult.state, context.State) {
			result[context.Context] = descriptionAndState{description: context.Description, state: context.State}
		}
	}

	var resultSlice []Context
	for name, descriptionAndState := range result {
		resultSlice = append(resultSlice, Context{Context: name, Description: descriptionAndState.description, State: descriptionAndState.state})
	}

	return resultSlice
}
