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
	"fmt"

	"k8s.io/test-infra/mungegithub/features"
	"k8s.io/test-infra/mungegithub/github"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
	"github.com/spf13/cobra"
)

const (
	pickMustHaveMilestoneFormat = "Removing label `%s` because no release milestone was set. This is an invalid state and thus this PR is not being considered for cherry-pick to any release branch. Please add an appropriate release milestone and then re-add the label."
)

var (
	pickMustHaveMilestoneBody = fmt.Sprintf(pickMustHaveMilestoneFormat, cpCandidateLabel)
)

// PickMustHaveMilestone will remove the the cherrypick-candidate label from
// any PR that does not have a 'release' milestone set.
type PickMustHaveMilestone struct{}

func init() {
	p := PickMustHaveMilestone{}
	RegisterMungerOrDie(p)
	RegisterStaleComments(p)
}

// Name is the name usable in --pr-mungers
func (PickMustHaveMilestone) Name() string { return "cherrypick-must-have-milestone" }

// RequiredFeatures is a slice of 'features' that must be provided
func (PickMustHaveMilestone) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (PickMustHaveMilestone) Initialize(config *github.Config, features *features.Features) error {
	return nil
}

// EachLoop is called at the start of every munge loop
func (PickMustHaveMilestone) EachLoop() error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (PickMustHaveMilestone) AddFlags(cmd *cobra.Command, config *github.Config) {}

// Munge is the workhorse the will actually make updates to the PR
func (PickMustHaveMilestone) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}
	if !obj.HasLabel(cpCandidateLabel) {
		return
	}

	releaseMilestone, ok := obj.ReleaseMilestone()
	if !ok {
		return
	}
	hasLabel := obj.HasLabel(cpCandidateLabel)

	if hasLabel && releaseMilestone == "" {
		obj.WriteComment(pickMustHaveMilestoneBody)
		obj.RemoveLabel(cpCandidateLabel)
	}
}

func (PickMustHaveMilestone) isStaleComment(obj *github.MungeObject, comment *githubapi.IssueComment) bool {
	if !mergeBotComment(comment) {
		return false
	}
	if *comment.Body != pickMustHaveMilestoneBody {
		return false
	}
	milestone, ok := obj.ReleaseMilestone()
	if !ok {
		return false
	}
	stale := milestone != ""
	if stale {
		glog.V(6).Infof("Found stale PickMustHaveMilestone comment")
	}
	return stale
}

// StaleComments returns a slice of stale comments
func (p PickMustHaveMilestone) StaleComments(obj *github.MungeObject, comments []*githubapi.IssueComment) []*githubapi.IssueComment {
	return forEachCommentTest(obj, comments, p.isStaleComment)
}
