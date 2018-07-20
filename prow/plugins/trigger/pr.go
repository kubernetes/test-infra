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
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

const (
	needsOkToTest = "needs-ok-to-test"
)

func handlePR(c client, trigger *plugins.Trigger, pr github.PullRequestEvent) error {
	org, repo, a := orgRepoAuthor(pr.PullRequest)
	author := string(a)
	num := pr.PullRequest.Number
	switch pr.Action {
	case github.PullRequestActionOpened:
		// When a PR is opened, if the author is in the org then build it.
		// Otherwise, ask for "/ok-to-test". There's no need to look for previous
		// "/ok-to-test" comments since the PR was just opened!
		member, err := trustedUser(c.GitHubClient, trigger, author, org, repo)
		if err != nil {
			return fmt.Errorf("could not check membership: %s", err)
		}
		if member {
			c.Logger.Info("Starting all jobs for new PR.")
			return buildAll(c, &pr.PullRequest, pr.GUID)
		}
		c.Logger.Infof("Welcome message to PR author %q.", author)
		if err := welcomeMsg(c.GitHubClient, trigger, pr.PullRequest); err != nil {
			return fmt.Errorf("could not welcome non-org member %q: %v", author, err)
		}
	case github.PullRequestActionReopened:
		// When a PR is reopened, check that the user is in the org or that an org
		// member had said "/ok-to-test" before building.
		comments, err := c.GitHubClient.ListIssueComments(org, repo, num)
		if err != nil {
			return err
		}
		trusted, err := trustedPullRequest(c.GitHubClient, trigger, author, org, repo, comments)
		if err != nil {
			return fmt.Errorf("could not validate PR: %s", err)
		} else if trusted {
			err = clearStaleComments(c.GitHubClient, pr.PullRequest, comments)
			if err != nil {
				c.Logger.Warnf("Failed to clear stale comments: %v.", err)
			}
			// Just try to remove "needs-ok-to-test" label if existing, we don't care about the result.
			c.GitHubClient.RemoveLabel(org, repo, num, needsOkToTest)
			c.Logger.Info("Starting all jobs for updated PR.")
			return buildAll(c, &pr.PullRequest, pr.GUID)
		}
	case github.PullRequestActionEdited:
		// if someone changes the base of their PR, we will get this
		// event and the changes field will list that the base SHA and
		// ref changes so we can detect such a case and retrigger tests
		var changes struct {
			Base struct {
				Ref struct {
					From string `json:"from"`
				} `json:"ref"`
				Sha struct {
					From string `json:"from"`
				} `json:"sha"`
			} `json:"base"`
		}
		if err := json.Unmarshal(pr.Changes, &changes); err != nil {
			// we're detecting this best-effort so we can forget about
			// the event
			return nil
		} else if changes.Base.Ref.From != "" || changes.Base.Sha.From != "" {
			// the base of the PR changed and we need to re-test it
			return buildAllIfTrusted(c, trigger, pr)
		}
	case github.PullRequestActionSynchronize:
		return buildAllIfTrusted(c, trigger, pr)
	case github.PullRequestActionLabeled:
		comments, err := c.GitHubClient.ListIssueComments(org, repo, num)
		if err != nil {
			return err
		}
		// When a PR is LGTMd, if it is untrusted then build it once.
		if pr.Label.Name == lgtmLabel {
			trusted, err := trustedPullRequest(c.GitHubClient, trigger, author, org, repo, comments)
			if err != nil {
				return fmt.Errorf("could not validate PR: %s", err)
			} else if !trusted {
				c.Logger.Info("Starting all jobs for untrusted PR with LGTM.")
				return buildAll(c, &pr.PullRequest, pr.GUID)
			}
		}
	}
	return nil
}

type login string

func orgRepoAuthor(pr github.PullRequest) (string, string, login) {
	org := pr.Base.Repo.Owner.Login
	repo := pr.Base.Repo.Name
	author := pr.User.Login
	return org, repo, login(author)
}

func buildAllIfTrusted(c client, trigger *plugins.Trigger, pr github.PullRequestEvent) error {
	// When a PR is updated, check that the user is in the org or that an org
	// member has said "/ok-to-test" before building. There's no need to ask
	// for "/ok-to-test" because we do that once when the PR is created.
	org, repo, a := orgRepoAuthor(pr.PullRequest)
	author := string(a)
	comments, err := c.GitHubClient.ListIssueComments(org, repo, pr.PullRequest.Number)
	if err != nil {
		return err
	}
	trusted, err := trustedPullRequest(c.GitHubClient, trigger, author, org, repo, comments)
	if err != nil {
		return fmt.Errorf("could not validate PR: %s", err)
	} else if trusted {
		err = clearStaleComments(c.GitHubClient, pr.PullRequest, comments)
		if err != nil {
			c.Logger.Warnf("Failed to clear stale comments: %v.", err)
		}
		c.Logger.Info("Starting all jobs for updated PR.")
		return buildAll(c, &pr.PullRequest, pr.GUID)
	}
	return nil
}

func welcomeMsg(ghc githubClient, trigger *plugins.Trigger, pr github.PullRequest) error {
	commentTemplate := `Hi @%s. Thanks for your PR.

I'm waiting for a [%s](https://github.com/orgs/%s/people) %smember to verify that this patch is reasonable to test. If it is, they should reply with ` + "`/ok-to-test`" + ` on its own line. Until that is done, I will not automatically test new commits in this PR, but the usual testing commands by org members will still work. Regular contributors should [join the org](%s) to skip this step.

I understand the commands that are listed [here](https://go.k8s.io/bot-commands).

<details>

%s
</details>
`
	org, repo, a := orgRepoAuthor(pr)
	author := string(a)
	var more string
	if trigger != nil && trigger.TrustedOrg != "" && trigger.TrustedOrg != org {
		more = fmt.Sprintf("or [%s](https://github.com/orgs/%s/people) ", trigger.TrustedOrg, trigger.TrustedOrg)
	}

	var joinOrgURL string
	if trigger != nil && trigger.JoinOrgURL != "" {
		joinOrgURL = trigger.JoinOrgURL
	} else {
		joinOrgURL = fmt.Sprintf("https://github.com/orgs/%s/people", org)
	}
	comment := fmt.Sprintf(commentTemplate, author, org, org, more, joinOrgURL, plugins.AboutThisBotWithoutCommands)

	err1 := ghc.AddLabel(org, repo, pr.Number, needsOkToTest)
	err2 := ghc.CreateComment(org, repo, pr.Number, comment)
	if err1 != nil || err2 != nil {
		return fmt.Errorf("welcomeMsg: error adding label: %v, error creating comment: %v", err1, err2)
	}
	return nil
}

// trustedPullRequest returns whether or not the given PR should be tested.
// It first checks if the author is in the org, then looks for "/ok-to-test"
// comments by org members.
func trustedPullRequest(ghc githubClient, trigger *plugins.Trigger, author, org, repo string, comments []github.IssueComment) (bool, error) {
	// First check if the author is a member of the org.
	if orgMember, err := trustedUser(ghc, trigger, author, org, repo); err != nil {
		return false, fmt.Errorf("error checking %s for trust: %v", author, err)
	} else if orgMember {
		return true, nil
	}
	botName, err := ghc.BotName()
	if err != nil {
		return false, fmt.Errorf("error finding bot name: %v", err)
	}
	// Next look for "/ok-to-test" comments on the PR.
	for _, comment := range comments {
		commentAuthor := comment.User.Login
		// Skip comments: by the PR author, or by bot, or not matching "/ok-to-test".
		if commentAuthor == author || commentAuthor == botName || !okToTestRe.MatchString(comment.Body) {
			continue
		}
		// Ensure that the commenter is in the org.
		if commentAuthorMember, err := trustedUser(ghc, trigger, commentAuthor, org, repo); err != nil {
			return false, fmt.Errorf("error checking %s for trust: %v", commentAuthor, err)
		} else if commentAuthorMember {
			return true, nil
		}
	}
	return false, nil
}

func buildAll(c client, pr *github.PullRequest, eventGUID string) error {
	var matchingJobs []config.Presubmit
	for _, job := range c.Config.Presubmits[pr.Base.Repo.FullName] {
		if job.AlwaysRun || job.RunIfChanged != "" {
			matchingJobs = append(matchingJobs, job)
		}
	}
	return runOrSkipRequested(c, pr, matchingJobs, nil, "", eventGUID)
}

// clearStaleComments deletes old comments that are no longer applicable.
func clearStaleComments(gc githubClient, pr github.PullRequest, comments []github.IssueComment) error {
	botName, err := gc.BotName()
	if err != nil {
		return err
	}

	org, repo, _ := orgRepoAuthor(pr)
	const waitingComment = "member to verify that this patch is reasonable to test."

	return gc.DeleteStaleComments(
		org,
		repo,
		pr.Number,
		comments,
		func(c github.IssueComment) bool { // isStale function
			return c.User.Login == botName && strings.Contains(c.Body, waitingComment)
		},
	)
}
