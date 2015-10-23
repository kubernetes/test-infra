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
	"os"
	"strings"

	"k8s.io/contrib/mungegithub/github"
	"k8s.io/kubernetes/pkg/util/yaml"

	"github.com/golang/glog"
	github_api "github.com/google/go-github/github"
	"github.com/spf13/cobra"
)

// weightMap is a map of user to a weight for that user.
type weightMap map[string]int64

// A BlunderbussConfig maps a set of file prefixes to a set of owner names (github users)
type BlunderbussConfig struct {
	PrefixMap map[string][]string `json:"prefixMap,omitempty" yaml:"prefixMap,omitempty"`
}

func (b *BlunderbussConfig) findOwners(filename string) weightMap {
	wm := weightMap{}
	for prefix, ownersList := range b.PrefixMap {
		if strings.HasPrefix(filename, prefix) {
			// Give one point for each directory-- so that more specific directories get more weight.
			weight := int64(1 + strings.Count(prefix, "/"))
			for _, owner := range ownersList {
				wm[owner] = wm[owner] + weight
			}
		}
	}
	return wm
}

// BlunderbussMunger will assign issues to users based on the config file
// provided by --blunderbuss-config.
type BlunderbussMunger struct {
	config                *BlunderbussConfig
	blunderbussConfigFile string
	blunderbussReassign   bool
}

func init() {
	blunderbuss := &BlunderbussMunger{}
	RegisterMungerOrDie(blunderbuss)
}

// Name is the name usable in --pr-mungers
func (b *BlunderbussMunger) Name() string { return "blunderbuss" }

// Initialize will initialize the munger
func (b *BlunderbussMunger) Initialize(config *github.Config) error {
	if len(b.blunderbussConfigFile) == 0 {
		glog.Fatalf("--blunderbuss-config is required with the blunderbuss munger")
	}
	file, err := os.Open(b.blunderbussConfigFile)
	if err != nil {
		glog.Fatalf("Failed to load blunderbuss config: %v", err)
	}
	defer file.Close()

	b.config = &BlunderbussConfig{}
	if err := yaml.NewYAMLToJSONDecoder(file).Decode(b.config); err != nil {
		glog.Fatalf("Failed to load blunderbuss config: %v", err)
	}
	glog.V(4).Infof("Loaded config from %s", b.blunderbussConfigFile)
	return nil
}

// EachLoop is called at the start of every munge loop
func (b *BlunderbussMunger) EachLoop() error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (b *BlunderbussMunger) AddFlags(cmd *cobra.Command, config *github.Config) {
	cmd.Flags().StringVar(&b.blunderbussConfigFile, "blunderbuss-config", "./blunderbuss.yml", "Path to the blunderbuss config file")
	cmd.Flags().BoolVar(&b.blunderbussReassign, "blunderbuss-reassign", false, "Assign PRs even if they're already assigned; use with -dry-run to judge changes to the assignment algorithm")
	b.addBlunderbussCommand(cmd)
}

// u may be nil.
func describeUser(u *github_api.User) string {
	if u != nil && u.Login != nil {
		return *u.Login
	}
	return "<nil>"
}

// Munge is the workhorse the will actually make updates to the PR
func (b *BlunderbussMunger) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	issue := obj.Issue
	if !b.blunderbussReassign && issue.Assignee != nil {
		glog.V(6).Infof("skipping %v: reassign: %v assignee: %v", *issue.Number, b.blunderbussReassign, describeUser(issue.Assignee))
		return
	}

	commits, err := obj.GetCommits()
	if err != nil {
		return
	}

	potentialOwners := weightMap{}
	weightSum := int64(0)
	for _, commit := range commits {
		for _, file := range commit.Files {
			fileWeight := int64(1)
			if file.Changes != nil && *file.Changes != 0 {
				fileWeight = int64(*file.Changes)
			}
			// Judge file size on a log scale-- effectively this
			// makes three buckets, we shouldn't have many 10k+
			// line changes.
			fileWeight = int64(math.Log10(float64(fileWeight))) + 1
			fileOwners := b.config.findOwners(*file.Filename)
			if len(fileOwners) == 0 {
				glog.Warningf("Couldn't find an owner for: %s", *file.Filename)
			}
			for owner, ownerWeight := range fileOwners {
				if owner == *issue.User.Login {
					continue
				}
				potentialOwners[owner] = potentialOwners[owner] + fileWeight*ownerWeight
				weightSum += fileWeight * ownerWeight
			}
		}
	}
	if len(potentialOwners) == 0 {
		glog.Errorf("No owners found for PR %d", *issue.Number)
		return
	}
	glog.V(4).Infof("Weights: %#v\nSum: %v", potentialOwners, weightSum)

	if issue.Assignee != nil {
		cur := *issue.Assignee.Login
		glog.Infof("Current assignee %v has a %02.2f%% chance of having been chosen", cur, 100.0*float64(potentialOwners[cur])/float64(weightSum))
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
	glog.Infof("Assigning %v to %v (previously assigned to %v)", *issue.Number, owner, describeUser(issue.Assignee))
	obj.AssignPR(owner)
}
