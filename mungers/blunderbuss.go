/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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

	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

// weightMap is a map of user to a weight for that user.
type weightMap map[string]int64

// A BlunderbussConfig maps a set of file prefixes to a set of owner names (github users)
type BlunderbussConfig struct {
	PrefixMap map[string][]string `json:"prefixMap,omitempty" yaml:"prefixMap,omitempty"`
}

// BlunderbussMunger will assign issues to users based on the config file
// provided by --blunderbuss-config.
type BlunderbussMunger struct {
	config              *BlunderbussConfig
	features            *features.Features
	blunderbussReassign bool
}

func init() {
	blunderbuss := &BlunderbussMunger{}
	RegisterMungerOrDie(blunderbuss)
}

// Name is the name usable in --pr-mungers
func (b *BlunderbussMunger) Name() string { return "blunderbuss" }

// RequiredFeatures is a slice of 'features' that must be provided
func (b *BlunderbussMunger) RequiredFeatures() []string { return []string{features.RepoFeatureName} }

// Initialize will initialize the munger
func (b *BlunderbussMunger) Initialize(config *github.Config, features *features.Features) error {
	glog.Infof("blunderbuss-reassign: %#v\n", b.blunderbussReassign)

	b.features = features
	return nil
}

// EachLoop is called at the start of every munge loop
func (b *BlunderbussMunger) EachLoop() error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (b *BlunderbussMunger) AddFlags(cmd *cobra.Command, config *github.Config) {
	cmd.Flags().BoolVar(&b.blunderbussReassign, "blunderbuss-reassign", false, "Assign PRs even if they're already assigned; use with -dry-run to judge changes to the assignment algorithm")
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

	files, err := obj.ListFiles()
	if err != nil {
		return
	}

	potentialOwners := weightMap{}
	weightSum := int64(0)
	for _, file := range files {
		fileWeight := int64(1)
		if file.Changes != nil && *file.Changes != 0 {
			fileWeight = int64(*file.Changes)
		}
		// Judge file size on a log scale-- effectively this
		// makes three buckets, we shouldn't have many 10k+
		// line changes.
		fileWeight = int64(math.Log10(float64(fileWeight))) + 1
		fileOwners := b.features.Repos.LeafAssignees(*file.Filename)
		if fileOwners.Len() == 0 {
			glog.Warningf("Couldn't find an owner for: %s", *file.Filename)
		}
		for _, owner := range fileOwners.List() {
			if owner == *issue.User.Login {
				continue
			}
			potentialOwners[owner] = potentialOwners[owner] + fileWeight
			weightSum += fileWeight
		}
	}
	if len(potentialOwners) == 0 {
		glog.Errorf("No owners found for PR %d", *issue.Number)
		return
	}
	printChance(potentialOwners, weightSum)
	if issue.Assignee != nil {
		cur := *issue.Assignee.Login
		c := chance(potentialOwners[cur], weightSum)
		glog.Infof("Current assignee %v has a %02.2f%% chance of having been chosen", cur, c)
	}
	selection := rand.Int63n(weightSum)
	owner := ""
	for o, w := range potentialOwners {
		owner = o
		selection -= w
		if selection <= 0 {
			break
		}
	}
	c := chance(potentialOwners[owner], weightSum)
	glog.Infof("Assigning %v to %v who had a %02.2f%% chance to be assigned (previously assigned to %v)", *issue.Number, owner, c, github.DescribeUser(issue.Assignee))
	obj.AssignPR(owner)
}
