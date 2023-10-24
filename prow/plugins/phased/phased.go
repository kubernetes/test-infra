/*
Copyright 2023 The Kubernetes Authors.

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

package phased

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const (
	// PluginName is the name of the phased plugin
	PluginName = "phased"
)

// untrustedReason represents a combination (by ORing the appropriate consts) of reasons
// why a user is not trusted by TrustedUser. It is used to generate messaging for users.
type untrustedReason int

const (
	notMember untrustedReason = 1 << iota
	notCollaborator
	notSecondaryMember
)

// String constructs a string explaining the reason for a user's denial of trust
// from untrustedReason as described above.
func (u untrustedReason) String() string {
	var response string
	if u&notMember != 0 {
		response += "User is not a member of the org. "
	}
	if u&notCollaborator != 0 {
		response += "User is not a collaborator. "
	}
	if u&notSecondaryMember != 0 {
		response += "User is not a member of the trusted secondary org. "
	}
	response += "Satisfy at least one of these conditions to make the user trusted."
	return response
}

func init() {
	plugins.RegisterPullRequestHandler(PluginName, handlePullRequest, helpProvider)
}

// TODO update
func helpProvider(config *plugins.Configuration, enabledRepos []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	configInfo := map[string]string{}
	for _, repo := range enabledRepos {
		trigger := config.TriggerFor(repo.Org, repo.Repo)
		org := repo.Org
		if trigger.TrustedOrg != "" {
			org = trigger.TrustedOrg
		}
		configInfo[repo.String()] = fmt.Sprintf("The trusted GitHub organization for this repository is %q.", org)
	}
	yamlSnippet, err := plugins.CommentMap.GenYaml(&plugins.Configuration{
		Triggers: []plugins.Trigger{
			{
				Repos: []string{
					"org/repo1",
					"org/repo2",
				},
				JoinOrgURL:     "https://github.com/kubernetes/community/blob/master/community-membership.md",
				OnlyOrgMembers: true,
				IgnoreOkToTest: true,
			},
		},
	})
	if err != nil {
		logrus.WithError(err).Warnf("cannot generate comments for %s plugin", PluginName)
	}
	pluginHelp := &pluginhelp.PluginHelp{
		Description: `The trigger plugin starts jobs in reaction to various events.
<br>Presubmit jobs are run automatically on pull requests that are trusted and not in a draft state with file changes matching the file filters and targeting a branch matching the branch filters.
<br>A pull request is considered trusted if the author is a member of the 'trusted organization' for the repository or if such a member has left an '/ok-to-test' command on the PR.
<br>Trigger will not automatically start jobs for a PR in draft state, and if a PR is changed to draft it cancels pending jobs.
<br>If jobs are not run automatically for a PR because it is not trusted or is in draft state, a trusted user can still start jobs manually via the '/test' command.
<br>The '/retest' command can be used to rerun jobs that have reported failure.
<br>Trigger starts postsubmit jobs when commits are pushed if the filters on the job match files and branches affected by that push.`,
		Config:  configInfo,
		Snippet: yamlSnippet,
	}
	return pluginHelp, nil
}

type githubClient interface {
	GetRef(org, repo, ref string) (string, error)
	CreateComment(owner, repo string, number int, comment string) error
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
}

type prowJobClient interface {
	Create(context.Context, *prowapi.ProwJob, metav1.CreateOptions) (*prowapi.ProwJob, error)
	List(ctx context.Context, opts metav1.ListOptions) (*prowapi.ProwJobList, error)
	Update(context.Context, *prowapi.ProwJob, metav1.UpdateOptions) (*prowapi.ProwJob, error)
}

// Client holds the necessary structures to work with prow via logging, github, kubernetes and its configuration.
type Client struct {
	GitHubClient  githubClient
	ProwJobClient prowJobClient
	Config        *config.Config
	Logger        *logrus.Entry
	GitClient     git.ClientFactory
}

func getClient(pc plugins.Agent) Client {
	return Client{
		GitHubClient:  pc.GitHubClient,
		Config:        pc.Config,
		ProwJobClient: pc.ProwJobClient,
		Logger:        pc.Logger,
		GitClient:     pc.GitClient,
	}
}

func handlePullRequest(pc plugins.Agent, pr github.PullRequestEvent) error {
	org, repo, _ := orgRepoAuthor(pr.PullRequest)
	return handlePR(getClient(pc), pc.PluginConfig.TriggerFor(org, repo), pr)
}

func getPresubmits(log *logrus.Entry, gc git.ClientFactory, cfg *config.Config, orgRepo string, baseSHAGetter, headSHAGetter config.RefGetter) []config.Presubmit {
	presubmits, err := cfg.GetPresubmits(gc, orgRepo, baseSHAGetter, headSHAGetter)
	if err != nil {
		// Fall back to static presubmits to avoid deadlocking when a presubmit is used to verify
		// inrepoconfig. Tide will still respect errors here and not merge.
		log.WithError(err).Debug("Failed to get presubmits")
		presubmits = cfg.GetPresubmitsStatic(orgRepo)
	}
	return presubmits
}
