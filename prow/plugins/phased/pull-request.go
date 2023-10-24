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
	"fmt"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/plugins"
)

func handlePR(c Client, trigger plugins.Trigger, pr github.PullRequestEvent) error {
	org, repo, _ := orgRepoAuthor(pr.PullRequest)

	baseSHA := ""
	baseSHAGetter := func() (string, error) {
		var err error
		baseSHA, err = c.GitHubClient.GetRef(org, repo, "heads/"+pr.PullRequest.Base.Ref)
		if err != nil {
			return "", fmt.Errorf("failed to get baseSHA: %w", err)
		}
		return baseSHA, nil
	}
	headSHAGetter := func() (string, error) {
		return pr.PullRequest.Head.SHA, nil
	}

	presubmits := getPresubmits(c.Logger, c.GitClient, c.Config, org+"/"+repo, baseSHAGetter, headSHAGetter)
	if len(presubmits) == 0 {
		return nil
	}

	if baseSHA == "" {
		if _, err := baseSHAGetter(); err != nil {
			return err
		}
	}

	switch pr.Action {
	case github.PullRequestActionLabeled:
		if pr.Label.Name == labels.LGTM {
			l, err := c.GitHubClient.GetIssueLabels(org, repo, pr.PullRequest.Number)
			if err != nil {
				return err
			}
			if github.HasLabel(labels.Approved, l) {
				c.Logger.Info("Starting all required manual jobs for lgtmed PR.")
				return listRequiredManual(c, &pr.PullRequest, pr.GUID, baseSHA, presubmits)
			}
		}
		if pr.Label.Name == labels.Approved {
			l, err := c.GitHubClient.GetIssueLabels(org, repo, pr.PullRequest.Number)
			if err != nil {
				return err
			}
			if github.HasLabel(labels.LGTM, l) {
				c.Logger.Info("Starting all required manual jobs for approved PR.")
				return listRequiredManual(c, &pr.PullRequest, pr.GUID, baseSHA, presubmits)
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

func listRequiredManual(c Client, pr *github.PullRequest, eventGUID string, baseSHA string, presubmits []config.Presubmit) error {
	if pr.Draft {
		return nil
	}

	org, repo, number, branch := pr.Base.Repo.Owner.Login, pr.Base.Repo.Name, pr.Number, pr.Base.Ref
	changes := config.NewGitHubDeferredChangedFilesProvider(c.GitHubClient, org, repo, number)
	toTest, err := pjutil.FilterPresubmits(NewRequiredManualFilter(), changes, branch, presubmits, c.Logger)
	if err != nil {
		return err
	}

	return listRequested(c, pr, baseSHA, toTest, eventGUID)
}

func listRequested(c Client, pr *github.PullRequest, baseSHA string, requestedJobs []config.Presubmit, eventGUID string) error {
	org, repo, _ := orgRepoAuthor(*pr)
	if !(org == "kubevirt" && repo == "kubevirt") && !(org == "org" && repo == "repo") {
		return nil
	}

	// If the PR is not mergeable (e.g. due to merge conflicts), we will not trigger any jobs,
	// to reduce the load on resources and reduce spam comments which will lead to a better review experience.
	if pr.Mergable != nil && !*pr.Mergable {
		return nil
	}

	var result string
	for _, job := range requestedJobs {
		result += "/test " + job.Name + "\n"
	}

	if result != "" {
		if err := c.GitHubClient.CreateComment(org, repo, pr.Number, result); err != nil {
			return err
		}
	}

	return nil
}

type TestRequiredManualFilter struct{}

func NewRequiredManualFilter() *TestRequiredManualFilter {
	return &TestRequiredManualFilter{}
}

func (tf *TestRequiredManualFilter) ShouldRun(p config.Presubmit) (bool, bool, bool) {
	return !p.Optional && !p.AlwaysRun && p.RegexpChangeMatcher.RunIfChanged == "" && p.RegexpChangeMatcher.SkipIfOnlyChanged == "", !p.Optional && !p.AlwaysRun && p.RegexpChangeMatcher.RunIfChanged == "" && p.RegexpChangeMatcher.SkipIfOnlyChanged == "", false
}

func (tf *TestRequiredManualFilter) Name() string {
	return "test-required-manual-filter"
}
