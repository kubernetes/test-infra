package inrepoconfig

import (
	"fmt"
	"strings"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/inrepoconfig/api"
	"k8s.io/test-infra/prow/plugins/trigger"
)

func init() {
	plugins.RegisterPullRequestHandler(api.PluginName, handlePullRequest, helpProvider)
}

const commentTag = "<!-- inrepoconfig report -->"

func helpProvider(_ *plugins.Configuration, _ []string) (*pluginhelp.PluginHelp, error) {
	return &pluginhelp.PluginHelp{
		Description: fmt.Sprintf(`The %s plugin is used to manage Presubmit config inside the tested repo. When activated, it will try to read a file named %s from the repositorys root and create a ProwJob for each presubmit configured below the "presubmit" key at the top level`, api.PluginName, api.ConfigFileName)}, nil
}

func handlePullRequest(pc plugins.Agent, pre github.PullRequestEvent) error {
	if pre.Action == github.PullRequestActionClosed {
		return nil
	}

	log := pc.Logger
	pr := pre.PullRequest
	org, repo, author, sha := pr.Base.Repo.Owner.Login, pr.Base.Repo.Name, pr.User.Login, pr.Head.SHA

	triggerCfg := pc.PluginConfig.TriggerFor(org, repo)
	trusted, err := trigger.TrustedUser(pc.GitHubClient, triggerCfg, author, org, repo)
	if err != nil {
		return fmt.Errorf("failed to check if user is trusted: %v", err)
	}

	status := github.Status{
		State:   "pending",
		Context: api.ContextName,
	}

	if err := pc.GitHubClient.CreateStatus(org, repo, sha, status); err != nil {
		return fmt.Errorf("failed to create status: %v", err)
	}
	if !trusted {
		log.Debugf("Not checking job config because author %s is not trusted", author)
		return nil
	}

	latestBaseSHA, err := pc.GitHubClient.GetRef(org, repo, "heads/"+pre.PullRequest.Base.Ref)
	if err != nil {
		status.State = "error"
		status.Description = "failed to get latest base ref from GitHub"
		if err := pc.GitHubClient.CreateStatus(org, repo, sha, status); err != nil {
			log.WithError(err).Error("failed to update GitHub status")
		}
		return fmt.Errorf("failed to get latest SHA for base ref %q: %v", pre.PullRequest.Base.Ref, err)
	}

	refs := []prowapi.Refs{
		prowapi.Refs{
			Org:     org,
			Repo:    repo,
			BaseRef: pre.PullRequest.Base.Ref,
			BaseSHA: latestBaseSHA,
			Pulls: []prowapi.Pull{
				prowapi.Pull{
					Number: pr.Number,
					SHA:    pr.Head.SHA,
					Ref:    pr.Head.Ref,
				},
			},
		},
	}
	jc, err := api.NewJobConfig(log, refs, &pc.Config.ProwConfig)
	if err != nil {
		log.WithError(err).Error("failed to read JobConfig from repo")

		status.State = "failure"
		if err := pc.GitHubClient.CreateStatus(org, repo, sha, status); err != nil {
			log.WithError(err).Error("failed to create GitHub context")
		}

		comment := fmt.Sprintf("%s\n@%s: Loading `%s` failed with the following error:\n```\n%v\n```",
			commentTag, pre.Sender.Login, api.ConfigFileName, err)
		_, exitingCommentID, err := getOutdatedIssueComments(pc.GitHubClient, org, repo, pr.Number)
		if err != nil {
			log.WithError(err).Error("failed to list comments")
		}
		if exitingCommentID == 0 {
			if err := pc.GitHubClient.CreateComment(org, repo, pr.Number, comment); err != nil {
				log.WithError(err).Error("failed to create comment")
			}
		} else {
			if err := pc.GitHubClient.EditComment(org, repo, exitingCommentID, comment); err != nil {
				log.WithError(err).Error("failed to update comment")
			}
		}

		return fmt.Errorf("failed to read %q: %v", api.ConfigFileName, err)
	}

	// TODO: alvaroaleman: DRY this out, this effectively duplicates buildAll in prow/plugins/trigger/pull-request.go
	changes := config.NewGitHubDeferredChangedFilesProvider(pc.GitHubClient, org, repo, pre.Number)
	toTest, toSkip, err := pjutil.FilterPresubmits(pjutil.TestAllFilter(), changes, pr.Base.Ref, jc.Presubmits, log)
	if err != nil {
		return fmt.Errorf("failed to filter presubmits: %v", err)
	}

	client := trigger.Client{
		GitHubClient:  pc.GitHubClient,
		ProwJobClient: pc.ProwJobClient,
		Config:        pc.Config,
		Logger:        log,
	}

	if err := trigger.RunAndSkipJobs(client, &pr, toTest, toSkip, pre.GUID, false); err != nil {
		return fmt.Errorf("failed to create ProwJobs: %v", err)
	}

	status.State = "success"
	if err := pc.GitHubClient.CreateStatus(org, repo, sha, status); err != nil {
		return fmt.Errorf("failed to set GitHub context to %q after creating ProwJobs: %v", status.State, err)
	}

	return removeOutdatedIssueComments(pc.GitHubClient, org, repo, pr.Number)
}

func removeOutdatedIssueComments(ghc github.Client, org, repo string, pr int) error {
	issueCommentsToDelete, _, err := getOutdatedIssueComments(ghc, org, repo, pr)
	if err != nil {
		return err
	}
	for _, issueCommentToDelete := range issueCommentsToDelete {
		if err := ghc.DeleteComment(org, repo, issueCommentToDelete); err != nil {
			return fmt.Errorf("failed to delete comment: %v", err)
		}
	}
	return nil
}

func getOutdatedIssueComments(ghc github.Client, org, repo string, pr int) (all []int, latest int, err error) {
	ics, err := ghc.ListIssueComments(org, repo, pr)
	if err != nil {
		err = fmt.Errorf("failed to list comments: %v", err)
		return
	}

	botName, err := ghc.BotName()
	if err != nil {
		err = fmt.Errorf("failed to get botName: %v", err)
		return
	}

	for _, ic := range ics {
		if ic.User.Login != botName {
			continue
		}
		if !strings.Contains(ic.Body, commentTag) {
			continue
		}
		all = append(all, ic.ID)
		latest = ic.ID
	}

	return
}
