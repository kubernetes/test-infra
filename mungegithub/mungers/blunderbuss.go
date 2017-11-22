/*
Copyright 2015 The Kubernetes Authors.

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

package mungers

import (
	"math"
	"math/rand"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/mungegithub/features"
	"k8s.io/test-infra/mungegithub/github"
	"k8s.io/test-infra/mungegithub/options"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
)

// weightMap is a map of user to a weight for that user.
type weightMap map[string]int64

// A BlunderbussConfig maps a set of file prefixes to a set of owner names (github users)
type BlunderbussConfig struct {
	PrefixMap map[string][]string `json:"prefixMap,omitempty" yaml:"prefixMap,omitempty"`
}

// BlunderbussMunger will assign issues to users based on the config file
// provided by --blunderbuss-config and/or OWNERS files.
type BlunderbussMunger struct {
	config   *BlunderbussConfig
	features *features.Features

	blunderbussReassign bool
	numAssignees        int
}

func init() {
	blunderbuss := &BlunderbussMunger{}
	RegisterMungerOrDie(blunderbuss)
}

// Name is the name usable in --pr-mungers
func (b *BlunderbussMunger) Name() string { return "blunderbuss" }

// RequiredFeatures is a slice of 'features' that must be provided
func (b *BlunderbussMunger) RequiredFeatures() []string {
	return []string{features.RepoFeatureName}
}

// Initialize will initialize the munger
func (b *BlunderbussMunger) Initialize(config *github.Config, features *features.Features) error {
	b.features = features
	return nil
}

// EachLoop is called at the start of every munge loop
func (b *BlunderbussMunger) EachLoop() error { return nil }

// RegisterOptions registers options for this munger; returns any that require a restart when changed.
func (b *BlunderbussMunger) RegisterOptions(opts *options.Options) sets.String {
	opts.RegisterBool(&b.blunderbussReassign, "blunderbuss-reassign", false, "Assign PRs even if they're already assigned; use with -dry-run to judge changes to the assignment algorithm")
	opts.RegisterInt(&b.numAssignees, "blunderbuss-number-assignees", 2, "Number of assignees to select for each PR.")
	return nil
}

func chance(val, total int64) float64 {
	return 100.0 * float64(val) / float64(total)
}

func printChance(owners weightMap, total int64) {
	if !glog.V(4) {
		return
	}
	glog.Infof("Owner\tPercent")
	for name, weight := range owners {
		glog.Infof("%s\t%02.2f%%", name, chance(weight, total))
	}
}

func getPotentialOwners(author string, feats *features.Features, files []*githubapi.CommitFile, leafOnly bool) (weightMap, int64) {
	potentialOwners := weightMap{}
	weightSum := int64(0)
	var fileOwners sets.String
	for _, file := range files {
		if file == nil {
			continue
		}
		fileWeight := int64(1)
		if file.Changes != nil && *file.Changes != 0 {
			fileWeight = int64(*file.Changes)
		}
		// Judge file size on a log scale-- effectively this
		// makes three buckets, we shouldn't have many 10k+
		// line changes.
		fileWeight = int64(math.Log10(float64(fileWeight))) + 1
		if leafOnly {
			fileOwners = feats.Repos.LeafReviewers(*file.Filename)
		} else {
			fileOwners = feats.Repos.Reviewers(*file.Filename)
		}

		if fileOwners.Len() == 0 {
			glog.Warningf("Couldn't find an owner for: %s", *file.Filename)
		}

		for _, owner := range fileOwners.List() {
			if owner == author {
				continue
			}
			potentialOwners[owner] = potentialOwners[owner] + fileWeight
			weightSum += fileWeight
		}
	}
	return potentialOwners, weightSum
}

func selectMultipleOwners(potentialOwners weightMap, weightSum int64, count int) []string {
	// Make a copy of the map
	pOwners := weightMap{}
	for k, v := range potentialOwners {
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

// Munge is the workhorse the will actually make updates to the PR
func (b *BlunderbussMunger) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	issue := obj.Issue
	if !b.blunderbussReassign && issue.Assignee != nil {
		glog.V(6).Infof("skipping %v: reassign: %v assignee: %v", *issue.Number, b.blunderbussReassign, github.DescribeUser(issue.Assignee))
		return
	}

	files, ok := obj.ListFiles()
	if !ok {
		return
	}

	potentialOwners, weightSum := getPotentialOwners(*obj.Issue.User.Login, b.features, files, true)
	if len(potentialOwners) == 0 {
		potentialOwners, weightSum = getPotentialOwners(*obj.Issue.User.Login, b.features, files, false)
		if len(potentialOwners) == 0 {
			glog.Errorf("No OWNERS found for PR %d", *issue.Number)
			return
		}
	}
	printChance(potentialOwners, weightSum)
	if issue.Assignee != nil {
		cur := *issue.Assignee.Login
		c := chance(potentialOwners[cur], weightSum)
		glog.Infof("Current assignee %v has a %02.2f%% chance of having been chosen", cur, c)
	}

	owners := selectMultipleOwners(potentialOwners, weightSum, b.numAssignees)

	for _, owner := range owners {
		c := chance(potentialOwners[owner], weightSum)
		glog.Infof("Assigning %v to %v who had a %02.2f%% chance to be assigned", *issue.Number, owner, c)
		obj.AddAssignee(owner)
	}
}
