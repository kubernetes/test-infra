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

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/plugins"
)

const (
	needsOkToTest = "needs-ok-to-test"
)

func handlePR(c client, trustedOrg string, pr github.PullRequestEvent) error {
	author := pr.PullRequest.User.Login
	switch pr.Action {
	case github.PullRequestActionOpened:
		// When a PR is opened, if the author is in the org then build it.
		// Otherwise, ask for "/ok-to-test". There's no need to look for previous
		// "/ok-to-test" comments since the PR was just opened!
		member, err := c.GitHubClient.IsMember(trustedOrg, author)
		if err != nil {
			return fmt.Errorf("could not check membership: %s", err)
		} else if member {
			c.Logger.Info("Starting all jobs for new PR.")
			return buildAll(c, pr.PullRequest, pr.GUID)
		} else {
			c.Logger.Infof("Welcome message to PR author %q.", author)
			if err := welcomeMsg(c.GitHubClient, pr.PullRequest, trustedOrg); err != nil {
				return fmt.Errorf("could not welcome non-org member %q: %v", author, err)
			}
		}
	case github.PullRequestActionReopened:
		// When a PR is reopened, check that the user is in the org or that an org
		// member had said "/ok-to-test" before building.
		comments, err := c.GitHubClient.ListIssueComments(pr.PullRequest.Base.Repo.Owner.Login, pr.PullRequest.Base.Repo.Name, pr.PullRequest.Number)
		if err != nil {
			return err
		}
		trusted, err := trustedPullRequest(c.GitHubClient, pr.PullRequest, trustedOrg, comments)
		if err != nil {
			return fmt.Errorf("could not validate PR: %s", err)
		} else if trusted {
			err = clearStaleComments(c.GitHubClient, trustedOrg, pr.PullRequest, comments)
			if err != nil {
				c.Logger.Warnf("Failed to clear stale comments: %v.", err)
			}
			// Just try to remove "needs-ok-to-test" label if existing, we don't care about the result.
			c.GitHubClient.RemoveLabel(trustedOrg, pr.PullRequest.Base.Repo.Name, pr.PullRequest.Number, needsOkToTest)
			c.Logger.Info("Starting all jobs for updated PR.")
			return buildAll(c, pr.PullRequest, pr.GUID)
		}
	case github.PullRequestActionSynchronize:
		// When a PR is updated, check that the user is in the org or that an org
		// member has said "/ok-to-test" before building. There's no need to ask
		// for "/ok-to-test" because we do that once when the PR is created.
		comments, err := c.GitHubClient.ListIssueComments(pr.PullRequest.Base.Repo.Owner.Login, pr.PullRequest.Base.Repo.Name, pr.PullRequest.Number)
		if err != nil {
			return err
		}
		trusted, err := trustedPullRequest(c.GitHubClient, pr.PullRequest, trustedOrg, comments)
		if err != nil {
			return fmt.Errorf("could not validate PR: %s", err)
		} else if trusted {
			err = clearStaleComments(c.GitHubClient, trustedOrg, pr.PullRequest, comments)
			if err != nil {
				c.Logger.Warnf("Failed to clear stale comments: %v.", err)
			}
			c.Logger.Info("Starting all jobs for updated PR.")
			return buildAll(c, pr.PullRequest, pr.GUID)
		}
	case github.PullRequestActionLabeled:
		comments, err := c.GitHubClient.ListIssueComments(pr.PullRequest.Base.Repo.Owner.Login, pr.PullRequest.Base.Repo.Name, pr.PullRequest.Number)
		if err != nil {
			return err
		}
		// When a PR is LGTMd, if it is untrusted then build it once.
		if pr.Label.Name == lgtmLabel {
			trusted, err := trustedPullRequest(c.GitHubClient, pr.PullRequest, trustedOrg, comments)
			if err != nil {
				return fmt.Errorf("could not validate PR: %s", err)
			} else if !trusted {
				c.Logger.Info("Starting all jobs for untrusted PR with LGTM.")
				return buildAll(c, pr.PullRequest, pr.GUID)
			}
		}
	}
	return nil
}

func welcomeMsg(ghc githubClient, pr github.PullRequest, trustedOrg string) error {
	commentTemplate := `Hi @%s. Thanks for your PR.

I'm waiting for a [%s](https://github.com/orgs/%s/people) member to verify that this patch is reasonable to test. If it is, they should reply with ` + "`/ok-to-test`" + ` on its own line. Until that is done, I will not automatically test new commits in this PR, but the usual testing commands by org members will still work. Regular contributors should join the org to skip this step.

I understand the commands that are listed [here](https://github.com/kubernetes/test-infra/blob/master/commands.md).

<details>

%s
</details>
`
	comment := fmt.Sprintf(commentTemplate, pr.User.Login, trustedOrg, trustedOrg, plugins.AboutThisBotWithoutCommands)

	owner := pr.Base.Repo.Owner.Login
	name := pr.Base.Repo.Name
	err1 := ghc.AddLabel(owner, name, pr.Number, needsOkToTest)
	err2 := ghc.CreateComment(owner, name, pr.Number, comment)
	if err1 != nil || err2 != nil {
		return fmt.Errorf("welcomeMsg: error adding label: %v, error creating comment: %v", err1, err2)
	}
	return nil
}

// trustedPullRequest returns whether or not the given PR should be tested.
// It first checks if the author is in the org, then looks for "/ok-to-test"
// comments by org members.
func trustedPullRequest(ghc githubClient, pr github.PullRequest, trustedOrg string, comments []github.IssueComment) (bool, error) {
	author := pr.User.Login
	// First check if the author is a member of the org.
	orgMember, err := ghc.IsMember(trustedOrg, author)
	if err != nil {
		return false, err
	} else if orgMember {
		return true, nil
	}
	botName, err := ghc.BotName()
	if err != nil {
		return false, err
	}
	// Next look for "/ok-to-test" comments on the PR.
	for _, comment := range comments {
		commentAuthor := comment.User.Login
		// Skip comments: by the PR author, or by bot, or not matching "/ok-to-test".
		if commentAuthor == author || commentAuthor == botName || !okToTest.MatchString(comment.Body) {
			continue
		}
		// Ensure that the commenter is in the org.
		commentAuthorMember, err := ghc.IsMember(trustedOrg, commentAuthor)
		if err != nil {
			return false, err
		} else if commentAuthorMember {
			return true, nil
		}
	}
	return false, nil
}

func buildAll(c client, pr github.PullRequest, eventGUID string) error {
	org := pr.Base.Repo.Owner.Login
	repo := pr.Base.Repo.Name
	var ref string
	var changes []string // lazily initialized

	for _, job := range c.Config.Presubmits[pr.Base.Repo.FullName] {
		skip := false
		if job.RunIfChanged != "" {
			if changes == nil {
				changesFull, err := c.GitHubClient.GetPullRequestChanges(org, repo, pr.Number)
				if err != nil {
					return err
				}
				// We only care about the filenames here
				for _, change := range changesFull {
					changes = append(changes, change.Filename)
				}
			}
			skip = !job.RunsAgainstChanges(changes)
		} else if !job.AlwaysRun {
			continue
		}

		if (skip || !job.RunsAgainstBranch(pr.Base.Ref)) && !job.SkipReport {
			if err := c.GitHubClient.CreateStatus(org, repo, pr.Head.SHA, github.Status{
				State:       github.StatusSuccess,
				Context:     job.Context,
				Description: "Skipped",
			}); err != nil {
				return err
			}
			continue
		}

		// Only get master ref once.
		if ref == "" {
			r, err := c.GitHubClient.GetRef(org, repo, "heads/"+pr.Base.Ref)
			if err != nil {
				return err
			}
			ref = r
		}
		kr := kube.Refs{
			Org:     org,
			Repo:    repo,
			BaseRef: pr.Base.Ref,
			BaseSHA: ref,
			Pulls: []kube.Pull{
				{
					Number: pr.Number,
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
			return err
		}
	}
	return nil
}

// clearStaleComments deletes old comments that are no longer applicable.
func clearStaleComments(gc githubClient, trustedOrg string, pr github.PullRequest, comments []github.IssueComment) error {
	botName, err := gc.BotName()
	if err != nil {
		return err
	}

	waitingComment := fmt.Sprintf("I'm waiting for a [%s](https://github.com/orgs/%s/people) member to verify that this patch is reasonable to test.", trustedOrg, trustedOrg)

	return gc.DeleteStaleComments(
		trustedOrg,
		pr.Base.Repo.Name,
		pr.Number,
		comments,
		func(c github.IssueComment) bool { // isStale function
			return c.User.Login == botName && strings.Contains(c.Body, waitingComment)
		},
	)
}
