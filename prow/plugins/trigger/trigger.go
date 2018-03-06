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

package trigger

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const (
	pluginName = "trigger"
	lgtmLabel  = "lgtm"
)

func init() {
	plugins.RegisterIssueCommentHandler(pluginName, handleIssueComment, helpProvider)
	plugins.RegisterPullRequestHandler(pluginName, handlePullRequest, helpProvider)
	plugins.RegisterPushEventHandler(pluginName, handlePush, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	configInfo := map[string]string{}
	for _, repo := range enabledRepos {
		parts := strings.Split(repo, "/")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid repo in enabledRepos: %q", repo)
		}
		trusted, _ := trustedOrgForRepo(config, parts[0], parts[1])
		configInfo[repo] = fmt.Sprintf("The trusted Github organization for this repository is %q.", trusted)
	}
	pluginHelp := &pluginhelp.PluginHelp{
		Description: `The trigger plugin starts tests in reaction to commands and pull request events. It is responsible for ensuring that test jobs are only run on trusted PRs. A PR is considered trusted if the author is a member of the 'trusted organization' for the repository or if such a member has left an '/ok-to-test' command on the PR.
<br>Trigger starts jobs automatically when a new trusted PR is created or when an untrusted PR becomes trusted, but it can also be used to start jobs manually via the '/test' command.
<br>The '/retest' command can be used to rerun jobs that have reported failure.`,
		Config: configInfo,
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/ok-to-test",
		Description: "Marks a PR as 'trusted' and starts tests.",
		Featured:    false,
		WhoCanUse:   "Members of the trusted organization for the repo.",
		Examples:    []string{"/ok-to-test"},
	})
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/test (<job name>|all)",
		Description: "Manually starts a/all test job(s).",
		Featured:    true,
		WhoCanUse:   "Anyone can trigger this command on a trusted PR.",
		Examples:    []string{"/test all", "/test pull-bazel-test"},
	})
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/retest",
		Description: "Rerun test jobs that have failed.",
		Featured:    true,
		WhoCanUse:   "Anyone can trigger this command on a trusted PR.",
		Examples:    []string{"/retest"},
	})
	return pluginHelp, nil
}

type githubClient interface {
	AddLabel(org, repo string, number int, label string) error
	BotName() (string, error)
	IsMember(org, user string) (bool, error)
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	GetRef(org, repo, ref string) (string, error)
	CreateComment(owner, repo string, number int, comment string) error
	ListIssueComments(owner, repo string, issue int) ([]github.IssueComment, error)
	CreateStatus(owner, repo, ref string, status github.Status) error
	GetCombinedStatus(org, repo, ref string) (*github.CombinedStatus, error)
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
	RemoveLabel(org, repo string, number int, label string) error
	DeleteStaleComments(org, repo string, number int, comments []github.IssueComment, isStale func(github.IssueComment) bool) error
}

type kubeClient interface {
	CreateProwJob(kube.ProwJob) (kube.ProwJob, error)
}

type client struct {
	GitHubClient githubClient
	KubeClient   kubeClient
	Config       *config.Config
	Logger       *logrus.Entry
}

func getClient(pc plugins.PluginClient) client {
	return client{
		GitHubClient: pc.GitHubClient,
		Config:       pc.Config,
		KubeClient:   pc.KubeClient,
		Logger:       pc.Logger,
	}
}

func handlePullRequest(pc plugins.PluginClient, pr github.PullRequestEvent) error {
	trustedOrg, joinOrgURL := trustedOrgForRepo(pc.PluginConfig, pr.Repo.Owner.Login, pr.Repo.Name)
	return handlePR(getClient(pc), trustedOrg, joinOrgURL, pr)
}

func handleIssueComment(pc plugins.PluginClient, ic github.IssueCommentEvent) error {
	trustedOrg, _ := trustedOrgForRepo(pc.PluginConfig, ic.Repo.Owner.Login, ic.Repo.Name)
	return handleIC(getClient(pc), trustedOrg, ic)
}

func handlePush(pc plugins.PluginClient, pe github.PushEvent) error {
	return handlePE(getClient(pc), pe)
}

// trustedOrgForRepo returns the configured trusted organization and a URL for it
// for the provided org and repo combination.
func trustedOrgForRepo(config *plugins.Configuration, org, repo string) (string, string) {
	if trigger := config.TriggerFor(org, repo); trigger != nil && trigger.TrustedOrg != "" {
		return trigger.TrustedOrg, trigger.JoinOrgURL
	}
	return org, fmt.Sprintf("https://github.com/orgs/%s/people", org)
}

func isUserTrusted(ghc githubClient, user, trustedOrg, org string) (bool, error) {
	orgMember, err := ghc.IsMember(trustedOrg, user)
	if err != nil {
		return false, err
	} else if orgMember {
		return true, nil
	}
	if org != trustedOrg {
		orgMember, err = ghc.IsMember(org, user)
		if err != nil {
			return false, err
		}
	}
	return orgMember, nil
}

func fileChangesGetter(ghc githubClient, org, repo string, num int) func() ([]string, error) {
	var changedFiles []string
	return func() ([]string, error) {
		// Fetch the changed files from github at most once.
		if changedFiles == nil {
			changes, err := ghc.GetPullRequestChanges(org, repo, num)
			if err != nil {
				return nil, fmt.Errorf("error getting pull request changes: %v", err)
			}
			changedFiles = []string{}
			for _, change := range changes {
				changedFiles = append(changedFiles, change.Filename)
			}
		}
		return changedFiles, nil
	}
}

func runOrSkipRequested(c client, pr *github.PullRequest, requestedJobs []config.Presubmit, forceRunContexts map[string]bool, body, eventGUID string) error {
	org := pr.Base.Repo.Owner.Login
	repo := pr.Base.Repo.Name
	number := pr.Number

	baseSHA, err := c.GitHubClient.GetRef(org, repo, "heads/"+pr.Base.Ref)
	if err != nil {
		return err
	}

	// Use a closure to lazily retrieve the file changes only if they are needed.
	// We only have to fetch the changes if there is at least one RunIfChanged
	// job that is not being force run (due to a `/retest` after a failure or
	// because it is explicitly triggered with `/test foo`).
	getChanges := fileChangesGetter(c.GitHubClient, org, repo, number)
	// shouldRun indicates if a job should actually run.
	shouldRun := func(j config.Presubmit) (bool, error) {
		if !j.RunsAgainstBranch(pr.Base.Ref) {
			return false, nil
		}
		if j.RunIfChanged == "" || forceRunContexts[j.Context] || j.TriggerMatches(body) {
			return true, nil
		}
		changes, err := getChanges()
		if err != nil {
			return false, err
		}
		return j.RunsAgainstChanges(changes), nil
	}

	// For each job determine if any sharded version of the job runs.
	// This in turn determines which jobs to run and which contexts to mark as "Skipped".
	//
	// Note: Job sharding is achieved with presubmit configurations that overlap on
	// name, but run under disjoint circumstances. For example, a job 'foo' can be
	// sharded to have different pod specs for different branches by
	// creating 2 presubmit configurations with the name foo, but different pod
	// specs, and specifying different branches for each job.
	var toRunJobs []config.Presubmit
	toRun := sets.NewString()
	toSkip := sets.NewString()
	for _, job := range requestedJobs {
		runs, err := shouldRun(job)
		if err != nil {
			return err
		}
		if runs {
			toRunJobs = append(toRunJobs, job)
			toRun.Insert(job.Context)
		} else if !job.SkipReport {
			toSkip.Insert(job.Context)
		}
	}
	// 'Skip' any context that is requested, but doesn't have any job shards that
	// will run.
	for _, context := range toSkip.Difference(toRun).List() {
		if err := c.GitHubClient.CreateStatus(org, repo, pr.Head.SHA, github.Status{
			State:       github.StatusSuccess,
			Context:     context,
			Description: "Skipped",
		}); err != nil {
			return err
		}
	}

	var errors []error
	for _, job := range toRunJobs {
		c.Logger.Infof("Starting %s build.", job.Name)
		kr := kube.Refs{
			Org:     org,
			Repo:    repo,
			BaseRef: pr.Base.Ref,
			BaseSHA: baseSHA,
			Pulls: []kube.Pull{
				{
					Number: number,
					Author: pr.User.Login,
					SHA:    pr.Head.SHA,
				},
			},
		}
		labels := make(map[string]string)
		for k, v := range job.Labels {
			labels[k] = v
		}
		labels[github.EventGUID] = eventGUID
		pj := pjutil.NewProwJob(pjutil.PresubmitSpec(job, kr), labels)
		c.Logger.WithFields(pjutil.ProwJobFields(&pj)).Info("Creating a new prowjob.")
		if _, err := c.KubeClient.CreateProwJob(pj); err != nil {
			errors = append(errors, err)
		}
	}
	if len(errors) > 0 {
		return fmt.Errorf("errors starting jobs: %v", errors)
	}
	return nil
}
