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
	"k8s.io/contrib/mungegithub/github"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

// NeedsRebaseMunger will add the "needs-rebase" label to any issue which is
// unable to be automatically merged
type NeedsRebaseMunger struct{}

const needsRebase = "needs-rebase"

func init() {
	RegisterMungerOrDie(NeedsRebaseMunger{})
}

// Name is the name usable in --pr-mungers
func (NeedsRebaseMunger) Name() string { return "needs-rebase" }

// Initialize will initialize the munger
func (NeedsRebaseMunger) Initialize(config *github.Config) error { return nil }

// EachLoop is called at the start of every munge loop
func (NeedsRebaseMunger) EachLoop(_ *github.Config) error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (NeedsRebaseMunger) AddFlags(cmd *cobra.Command, config *github.Config) {}

// MungePullRequest is the workhorse the will actually make updates to the PR
func (NeedsRebaseMunger) MungePullRequest(config *github.Config, obj *github.MungeObject) {
	pr := obj.PR

	mergeable, err := config.IsPRMergeable(obj)
	if err != nil {
		glog.V(2).Infof("Skipping %d - problem determining mergeable", *pr.Number)
		return
	}
	if mergeable && github.HasLabel(obj, needsRebase) {
		config.RemoveLabel(obj, needsRebase)
	}
	if !mergeable && !github.HasLabel(obj, needsRebase) {
		config.AddLabels(obj, []string{needsRebase})
	}
}
