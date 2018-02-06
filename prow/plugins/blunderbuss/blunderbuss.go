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
	"math"
	"math/rand"
	"sort"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/assign"
)

const (
	pluginName = "blunderbuss"
)

func init() {
	plugins.RegisterPullRequestHandler(pluginName, handlePullRequest, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	var pluralSuffix string
	var reviewCount int
	if config.Blunderbuss.ReviewerCount != nil {
		reviewCount = *config.Blunderbuss.ReviewerCount
	} else if config.Blunderbuss.ReviewerCount != nil {
		reviewCount = *config.Blunderbuss.FileWeightCount
	}
	if reviewCount != 1 {
		pluralSuffix = "s"
	}
	// Omit the fields [WhoCanUse, Usage, Examples] because this plugin is not triggered by human actions.
	return &pluginhelp.PluginHelp{
			Description: "The blunderbuss plugin automatically requests reviews from reviewers when a new PR is created. The reviewers are selected based on the reviewers specified in the OWNERS files that apply to the files modified by the PR.",
			Config: map[string]string{
				"": fmt.Sprintf("Blunderbuss is currently configured to request reviews from %d reviewer%s.", reviewCount, pluralSuffix),
			},
		},
		nil
}

type ownersClient interface {
	FindReviewersOwnersForFile(path string) string
	Reviewers(path string) sets.String
	LeafReviewers(path string) sets.String
}

type githubClient interface {
	RequestReview(org, repo string, number int, logins []string) error
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
}

func handlePullRequest(pc plugins.PluginClient, pre github.PullRequestEvent) error {
	if pre.Action != github.PullRequestActionOpened || assign.CCRegexp.MatchString(pre.PullRequest.Body) {
		return nil
	}

	oc, err := pc.OwnersClient.LoadRepoOwners(pre.Repo.Owner.Login, pre.Repo.Name)
	if err != nil {
		return fmt.Errorf("error loading RepoOwners: %v", err)
	}

	return handle(
		pc.GitHubClient,
		oc, pc.Logger,
		pc.PluginConfig.Blunderbuss.ReviewerCount,
		pc.PluginConfig.Blunderbuss.FileWeightCount,
		&pre,
	)
}

func handle(ghc githubClient, oc ownersClient, log *logrus.Entry, reviewerCount, oldReviewCount *int, pre *github.PullRequestEvent) error {
	changes, err := ghc.GetPullRequestChanges(pre.Repo.Owner.Login, pre.Repo.Name, pre.Number)
	if err != nil {
		return fmt.Errorf("error getting PR changes: %v", err)
	}

	var reviewers []string
	switch {
	case oldReviewCount != nil:
		reviewers = getReviewersOld(log, oc, pre.PullRequest.User.Login, changes, *oldReviewCount)
	case reviewerCount != nil:
		reviewers, err = getReviewers(oc, pre.PullRequest.User.Login, changes, *reviewerCount)
		if err != nil {
			return err
		}
		if missing := *reviewerCount - len(reviewers); missing > 0 {
			log.Warnf("Not enough reviewers found in OWNERS files for files touched by this PR. %d/%d reviewers found.", len(reviewers), reviewerCount)
		}
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
		ownersFile := owners.FindReviewersOwnersForFile(file.Filename)
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

func getReviewersOld(log *logrus.Entry, oc ownersClient, author string, changes []github.PullRequestChange, reviewerCount int) []string {
	potentialReviewers, weightSum := getPotentialReviewers(oc, author, changes, true)
	reviewers := selectMultipleReviewers(log, potentialReviewers, weightSum, reviewerCount)
	if len(reviewers) < reviewerCount {
		// Didn't find enough leaf reviewers, need to include reviewers from parent OWNERS files.
		potentialReviewers, weightSum := getPotentialReviewers(oc, author, changes, false)
		for _, reviewer := range reviewers {
			delete(potentialReviewers, reviewer)
		}
		reviewers = append(reviewers, selectMultipleReviewers(log, potentialReviewers, weightSum, reviewerCount-len(reviewers))...)
		if missing := reviewerCount - len(reviewers); missing > 0 {
			log.Errorf("Not enough reviewers found in OWNERS files for files touched by this PR. %d/%d reviewers found.", len(reviewers), reviewerCount)
		}
	}
	return reviewers
}

// weightMap is a map of user to a weight for that user.
type weightMap map[string]int64

func getPotentialReviewers(owners ownersClient, author string, files []github.PullRequestChange, leafOnly bool) (weightMap, int64) {
	potentialReviewers := weightMap{}
	weightSum := int64(0)
	var fileOwners sets.String
	for _, file := range files {
		fileWeight := int64(1)
		if file.Changes != 0 {
			fileWeight = int64(file.Changes)
		}
		// Judge file size on a log scale-- effectively this
		// makes three buckets, we shouldn't have many 10k+
		// line changes.
		fileWeight = int64(math.Log10(float64(fileWeight))) + 1
		if leafOnly {
			fileOwners = owners.LeafReviewers(file.Filename)
		} else {
			fileOwners = owners.Reviewers(file.Filename)
		}

		for _, owner := range fileOwners.List() {
			if owner == author {
				continue
			}
			potentialReviewers[owner] = potentialReviewers[owner] + fileWeight
			weightSum += fileWeight
		}
	}
	return potentialReviewers, weightSum
}

func selectMultipleReviewers(log *logrus.Entry, potentialReviewers weightMap, weightSum int64, count int) []string {
	for name, weight := range potentialReviewers {
		log.Debugf("Reviewer %s had chance %02.2f%%", name, chance(weight, weightSum))
	}

	// Make a copy of the map
	pOwners := weightMap{}
	for k, v := range potentialReviewers {
		pOwners[k] = v
	}

	owners := []string{}

	for i := 0; i < count; i++ {
		if len(pOwners) == 0 || weightSum == 0 {
			break
		}
		selection := rand.Int63n(weightSum)
		owner := ""
		for o, w := range pOwners {
			owner = o
			selection -= w
			if selection <= 0 {
				break
			}
		}

		owners = append(owners, owner)
		weightSum -= pOwners[owner]

		// Remove this person from the map.
		delete(pOwners, owner)
	}
	return owners
}

func chance(val, total int64) float64 {
	return 100.0 * float64(val) / float64(total)
}
