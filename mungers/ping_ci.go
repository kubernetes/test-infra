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
	"time"

	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

var (
	validBranches = []string{"master"}
)

// PingCIMunger looks for situations CI (Travis | Shippable) has flaked for some
// reason and we want to re-run them.  Achieves this by closing and re-opening the pr
type PingCIMunger struct{}

func init() {
	RegisterMungerOrDie(PingCIMunger{})
}

// Name is the name usable in --pr-mungers
func (PingCIMunger) Name() string { return "ping-ci" }

// RequiredFeatures is a slice of 'features' that must be provided
func (PingCIMunger) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (PingCIMunger) Initialize(config *github.Config, features *features.Features) error { return nil }

// EachLoop is called at the start of every munge loop
func (PingCIMunger) EachLoop() error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (PingCIMunger) AddFlags(cmd *cobra.Command, config *github.Config) {}

// Munge is the workhorse the will actually make updates to the PR
func (PingCIMunger) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	// This munger only runs on certain branches, since travis/CI only listens
	// on certain branches
	validBranch := false
	for _, b := range validBranches {
		if obj.IsForBranch(b) {
			validBranch = true
			break
		}
	}
	if !validBranch {
		return
	}

	if !obj.HasLabel(lgtmLabel) {
		return
	}
	mergeable, err := obj.IsMergeable()
	if err != nil {
		glog.V(2).Infof("ping CI skipping %d - problem determining mergeability", *obj.Issue.Number)
		return
	}
	if !mergeable {
		glog.V(2).Infof("ping CI skipping %d - not mergeable", *obj.Issue.Number)
		return
	}
	if state := obj.GetStatusState([]string{travisContext}); state == "incomplete" {
		msg := "Travis continuous integration appears to have missed, closing and re-opening to trigger it"
		obj.WriteComment(msg)

		obj.ClosePR()
		time.Sleep(5 * time.Second)
		obj.OpenPR(10)
	}
}
