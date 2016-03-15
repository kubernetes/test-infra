/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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
	"fmt"

	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"

	"github.com/spf13/cobra"
)

const (
	labelUnapprovedPicksName = "label-unapproved-picks"
)

// LabelUnapprovedPicks will remove the LGTM flag from an PR which has been
// updated since the reviewer added LGTM
type LabelUnapprovedPicks struct{}

func init() {
	RegisterMungerOrDie(LabelUnapprovedPicks{})
}

// Name is the name usable in --pr-mungers
func (LabelUnapprovedPicks) Name() string { return labelUnapprovedPicksName }

// RequiredFeatures is a slice of 'features' that must be provided
func (LabelUnapprovedPicks) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (LabelUnapprovedPicks) Initialize(config *github.Config, features *features.Features) error {
	return nil
}

// EachLoop is called at the start of every munge loop
func (LabelUnapprovedPicks) EachLoop() error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (LabelUnapprovedPicks) AddFlags(cmd *cobra.Command, config *github.Config) {}

// Munge is the workhorse the will actually make updates to the PR
func (LabelUnapprovedPicks) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	if obj.IsForBranch("master") {
		return
	}

	if obj.HasLabel(cpApprovedLabel) {
		return
	}

	obj.AddLabel(doNotMergeLabel)

	msg := fmt.Sprintf("This PR is not for the master branch but does not have the `%s` label. Adding the `%s` label.", cpApprovedLabel, doNotMergeLabel)
	obj.WriteComment(msg)
}
