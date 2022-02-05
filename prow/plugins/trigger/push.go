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
	"context"
	"sort"
	"time"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pjutil"
)

func listPushEventChanges(pe github.PushEvent) config.ChangedFilesProvider {
	return func() ([]string, error) {
		changed := make(map[string]bool)
		for _, commit := range pe.Commits {
			for _, added := range commit.Added {
				changed[added] = true
			}
			for _, removed := range commit.Removed {
				changed[removed] = true
			}
			for _, modified := range commit.Modified {
				changed[modified] = true
			}
		}
		var changedFiles []string
		for file := range changed {
			changedFiles = append(changedFiles, file)
		}
		return changedFiles, nil
	}
}

func createRefs(pe github.PushEvent) prowapi.Refs {
	return prowapi.Refs{
		Org:      pe.Repo.Owner.Name,
		Repo:     pe.Repo.Name,
		RepoLink: pe.Repo.HTMLURL,
		BaseRef:  pe.Branch(),
		BaseSHA:  pe.After,
		BaseLink: pe.Compare,
		Author:   github.NormLogin(pe.Sender.Login),
	}
}

// getPullRequests returns the list of merged pull requests associated with all the commits
// 	of a push event, sorted by most recently merged first
func getPullRequests(c githubClient, pe github.PushEvent) ([]github.PullRequest, error) {
	org := pe.Repo.Owner.Login
	repo := pe.Repo.Name
	var pulls []github.PullRequest
	for _, commit := range pe.Commits {
		if commit.ID == "" {
			continue
		}
		commitPulls, err := c.ListCommitPullRequests(org, repo, commit.ID)
		if err != nil {
			return nil, err
		}
		for _, pull := range commitPulls {
			if pull.MergedAt.After(time.Time{}) {
				pulls = append(pulls, pull)
			}
		}
	}
	sort.SliceStable(pulls, func(i, j int) bool { return pulls[i].MergedAt.After(pulls[j].MergedAt) })
	return pulls, nil
}

func shouldCommentOn(pj config.Postsubmit) bool {
	if pj.ReporterConfig == nil || pj.ReporterConfig.GitHub == nil {
		return false
	}
	return pj.ReporterConfig.GitHub.CommentOnPostsubmits
}

func handlePE(c Client, pe github.PushEvent) error {
	if pe.Deleted || pe.After == "0000000000000000000000000000000000000000" {
		// we should not trigger jobs for a branch deletion
		return nil
	}

	org := pe.Repo.Owner.Login
	repo := pe.Repo.Name

	shaGetter := func() (string, error) {
		return pe.After, nil
	}

	postsubmits := getPostsubmits(c.Logger, c.GitClient, c.Config, org+"/"+repo, shaGetter)

	var prsFetched bool
	var prs []github.PullRequest
	var err error
	for _, j := range postsubmits {
		if shouldRun, err := j.ShouldRun(pe.Branch(), listPushEventChanges(pe)); err != nil {
			return err
		} else if !shouldRun {
			continue
		}
		if shouldCommentOn(j) && !prsFetched {
			prsFetched = true
			// TODO: It seems there is a bug in GH API, which returns an empty list of
			// PRs for all commits listed by a push event, if it is queried immediately after
			// receiving a push event. Waiting for 1 second seems to successfully work around this.
			// Remove this once the bug is fixed.
			time.Sleep(1 * time.Second)
			prs, err = getPullRequests(c.GitHubClient, pe)
			if err != nil {
				return err
			}
		}

		var pj prowapi.ProwJob
		if shouldCommentOn(j) && len(prs) > 0 {
			pj = pjutil.NewPostsubmit(pjutil.CreateRefs(prs[0], pe.After), j, pe.GUID)
		} else {
			refs := createRefs(pe)
			labels := make(map[string]string)
			for k, v := range j.Labels {
				labels[k] = v
			}
			labels[github.EventGUID] = pe.GUID
			pj = pjutil.NewProwJob(pjutil.PostsubmitSpec(j, refs), labels, j.Annotations)
		}
		c.Logger.WithFields(pjutil.ProwJobFields(&pj)).Info("Creating a new prowjob.")
		if err := createWithRetry(context.TODO(), c.ProwJobClient, &pj); err != nil {
			return err
		}
	}
	return nil
}
