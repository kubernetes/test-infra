/*
Copyright 2017 The Kubernetes Authors.

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

package blunderbuss

import (
	"fmt"
	"math/rand"
	"sort"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const (
	pluginName = "blunderbuss"
)

func init() {
	plugins.RegisterPullRequestHandler(pluginName, handlePullRequest, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	var pluralSuffix string
	if config.Blunderbuss.ReviewerCount != 1 {
		pluralSuffix = "s"
	}
	// Omit the fields [WhoCanUse, Usage, Examples] because this plugin is not triggered by human actions.
	return &pluginhelp.PluginHelp{
			Description: "The blunderbuss plugin automatically requests reviews from reviewers when a new PR is created. The reviewers are selected based on the reviewers specified in the OWNERS files that apply to the files modified by the PR.",
			Config: map[string]string{
				"": fmt.Sprintf("Blunderbuss is currently configured to request reviews from %d reviewer%s.", config.Blunderbuss.ReviewerCount, pluralSuffix),
			},
		},
		nil
}

// weightMap is a map of user to a weight for that user.
type weightMap map[string]int64

type ownersClient interface {
	FindReviewersOwnersForPath(path string) string
	Reviewers(path string) sets.String
	LeafReviewers(path string) sets.String
}

type githubClient interface {
	RequestReview(org, repo string, number int, logins []string) error
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
}

func handlePullRequest(pc plugins.PluginClient, pre github.PullRequestEvent) error {
	if pre.Action != github.PullRequestActionOpened {
		return nil
	}

	oc, err := pc.OwnersClient.LoadRepoOwners(pre.Repo.Owner.Login, pre.Repo.Name)
	if err != nil {
		return fmt.Errorf("error loading RepoOwners: %v", err)
	}

	return handle(pc.GitHubClient, oc, pc.Logger, pc.PluginConfig.Blunderbuss.ReviewerCount, &pre)
}

func handle(ghc githubClient, oc ownersClient, log *logrus.Entry, reviewerCount int, pre *github.PullRequestEvent) error {
	changes, err := ghc.GetPullRequestChanges(pre.Repo.Owner.Login, pre.Repo.Name, pre.Number)
	if err != nil {
		return fmt.Errorf("error getting PR changes: %v", err)
	}

	reviewers, err := getReviewers(oc, pre.PullRequest.User.Login, changes, reviewerCount)
	if err != nil {
		return err
	}
	if missing := reviewerCount - len(reviewers); missing > 0 {
		log.Warnf("Not enough reviewers found in OWNERS files for files touched by this PR. %d/%d reviewers found.", len(reviewers), reviewerCount)
	}
	if len(reviewers) > 0 {
		log.Infof("Requesting reviews from users %s.", reviewers)
		return ghc.RequestReview(pre.Repo.Owner.Login, pre.Repo.Name, pre.Number, reviewers)
	}
	return nil
}

func getReviewers(owners ownersClient, author string, files []github.PullRequestChange, minReviewers int) ([]string, error) {
	authorSet := sets.NewString(author)
	reviewers := sets.NewString()
	leafReviewers := sets.NewString()
	ownersSeen := sets.NewString()
	// first build 'reviewers' by taking a unique reviewer from each OWNERS file.
	for _, file := range files {
		ownersFile := owners.FindReviewersOwnersForPath(file.Filename)
		if ownersSeen.Has(ownersFile) {
			continue
		}
		ownersSeen.Insert(ownersFile)

		fileUnusedLeafs := owners.LeafReviewers(file.Filename).Difference(reviewers).Difference(authorSet)
		if fileUnusedLeafs.Len() == 0 {
			continue
		}
		leafReviewers = leafReviewers.Union(fileUnusedLeafs)
		reviewers.Insert(popRandom(fileUnusedLeafs))
	}
	// now ensure that we request review from at least minReviewers reviewers. Favor leaf reviewers.
	unusedLeafs := leafReviewers.Difference(reviewers)
	for reviewers.Len() < minReviewers && unusedLeafs.Len() > 0 {
		reviewers.Insert(popRandom(unusedLeafs))
	}
	for _, file := range files {
		if reviewers.Len() >= minReviewers {
			break
		}
		fileReviewers := owners.Reviewers(file.Filename).Difference(authorSet)
		for reviewers.Len() < minReviewers && fileReviewers.Len() > 0 {
			reviewers.Insert(popRandom(fileReviewers))
		}
	}
	return reviewers.List(), nil
}

// popRandom randomly selects an element of 'set' and pops it.
func popRandom(set sets.String) string {
	list := set.List()
	sort.Strings(list)
	sel := list[rand.Intn(len(list))]
	set.Delete(sel)
	return sel
}
