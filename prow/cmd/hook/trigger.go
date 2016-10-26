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

package main

import (
	"fmt"

	"k8s.io/test-infra/prow/github"
)

const lgtmLabel = "lgtm"

func (ga *GitHubAgent) prTrigger(pr github.PullRequestEvent) error {
	switch pr.Action {
	case "opened":
		// When a PR is opened, if the author is in the org then build it.
		// Otherwise, ask for "ok to test". There's no need to look for previous
		// "ok to test" comments since the PR was just opened!
		member, err := ga.isMember(pr.PullRequest.User.Login)
		if err != nil {
			return fmt.Errorf("could not check membership: %s", err)
		} else if member {
			ga.buildAll(pr.PullRequest)
		} else if err := ga.askToJoin(pr.PullRequest); err != nil {
			return fmt.Errorf("could not ask to join: %s", err)
		}
	case "reopened", "synchronize":
		// When a PR is updated, check that the user is in the org or that an org
		// member has said "ok to test" before building. There's no need to ask
		// for "ok to test" because we do that once when the PR is created.
		trusted, err := ga.trustedPullRequest(pr.PullRequest)
		if err != nil {
			return fmt.Errorf("could not validate PR: %s", err)
		} else if trusted {
			ga.buildAll(pr.PullRequest)
		}
	case "closed":
		ga.deleteAll(pr.PullRequest)
	case "labeled":
		// When a PR is LGTMd, if it is untrusted then build it once.
		if pr.Label.Name == lgtmLabel {
			trusted, err := ga.trustedPullRequest(pr.PullRequest)
			if err != nil {
				return fmt.Errorf("could not validate PR: %s", err)
			} else if !trusted {
				ga.buildAll(pr.PullRequest)
			}
		}
	}
	return nil
}

func (ga *GitHubAgent) commentTrigger(ic github.IssueCommentEvent) error {
	owner := ic.Repo.Owner.Login
	name := ic.Repo.Name
	number := ic.Issue.Number
	author := ic.Comment.User.Login
	switch ic.Action {
	case "created", "edited":
		// If it's not an open PR, skip it.
		if ic.Issue.PullRequest == nil {
			return nil
		}
		if ic.Issue.State != "open" {
			return nil
		}
		// Skip bot comments.
		if author == "k8s-bot" || author == "k8s-ci-robot" {
			return nil
		}

		// Which jobs does the comment want us to run?
		requestedJobs := ga.JenkinsJobs.MatchingJobs(ic.Repo.FullName, ic.Comment.Body)
		if len(requestedJobs) == 0 {
			return nil
		}

		// Skip untrusted users.
		orgMember, err := ga.isMember(author)
		if err != nil {
			return err
		} else if !orgMember {
			return nil
		}

		pr, err := ga.GitHubClient.GetPullRequest(owner, name, number)
		if err != nil {
			return err
		}

		for _, job := range requestedJobs {
			kr := makeKubeRequest(job, *pr)
			ga.BuildRequests <- kr
		}
	}
	return nil
}

func (ga *GitHubAgent) buildAll(pr github.PullRequest) {
	for _, job := range ga.JenkinsJobs.AllJobs(pr.Base.Repo.FullName) {
		if !job.AlwaysRun {
			continue
		}
		kr := makeKubeRequest(job, pr)
		ga.BuildRequests <- kr
	}
}

func (ga *GitHubAgent) deleteAll(pr github.PullRequest) {
	for _, job := range ga.JenkinsJobs.AllJobs(pr.Base.Repo.FullName) {
		kr := makeKubeRequest(job, pr)
		ga.DeleteRequests <- kr
	}
}

func makeKubeRequest(job JenkinsJob, pr github.PullRequest) KubeRequest {
	return KubeRequest{
		JobName: job.Name,
		Context: job.Context,

		RerunCommand: job.RerunCommand,

		RepoOwner: pr.Base.Repo.Owner.Login,
		RepoName:  pr.Base.Repo.Name,
		PR:        pr.Number,
		Author:    pr.User.Login,
		BaseRef:   pr.Base.Ref,
		BaseSHA:   pr.Base.SHA,
		PullSHA:   pr.Head.SHA,
	}
}

// trustedPullRequest returns whether or not the given PR should be tested.
// It first checks if the author is in the org, then looks for "ok to test
// comments by org members.
func (ga *GitHubAgent) trustedPullRequest(pr github.PullRequest) (bool, error) {
	author := pr.User.Login
	// First check if the author is a member of the org.
	orgMember, err := ga.isMember(author)
	if err != nil {
		return false, err
	} else if orgMember {
		return true, nil
	}
	// Next look for "ok to test" comments on the PR.
	comments, err := ga.GitHubClient.ListIssueComments(pr.Base.Repo.Owner.Login, pr.Base.Repo.Name, pr.Number)
	if err != nil {
		return false, err
	}
	for _, comment := range comments {
		commentAuthor := comment.User.Login
		// Skip comments by the PR author.
		if commentAuthor == author {
			continue
		}
		// Skip bot comments.
		if commentAuthor == "k8s-bot" || commentAuthor == "k8s-ci-robot" {
			continue
		}
		// Look for "ok to test"
		if !okToTest.MatchString(comment.Body) {
			continue
		}
		// Ensure that the commenter is in the org.
		commentAuthorMember, err := ga.isMember(commentAuthor)
		if err != nil {
			return false, err
		} else if commentAuthorMember {
			return true, nil
		}
	}
	return false, nil
}

func (ga *GitHubAgent) askToJoin(pr github.PullRequest) error {
	commentTemplate := `
Can a [%s](https://github.com/orgs/%s/people) member verify that this patch is reasonable to test? If so, please reply with "@k8s-bot ok to test" on its own line. Until that is done, I will not automatically test new commits in this PR, but the usual testing commands will still work. Regular contributors should join the org to skip this step.

If you have questions or suggestions related to this bot's behavior, please file an issue against the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/issues/new?title=Prow%20issue:) repository.`
	comment := fmt.Sprintf(commentTemplate, ga.Org, ga.Org)

	owner := pr.Base.Repo.Owner.Login
	name := pr.Base.Repo.Name
	return ga.GitHubClient.CreateComment(owner, name, pr.Number, comment)
}
