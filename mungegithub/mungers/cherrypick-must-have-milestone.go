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

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/mungegithub/features"
	"k8s.io/test-infra/mungegithub/github"
	"k8s.io/test-infra/mungegithub/options"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
)

const (
	pickMustHaveMilestoneFormat = "Removing label `%s` because no release milestone was set. This is an invalid state and thus this PR is not being considered for cherry-pick to any release branch. Please add an appropriate release milestone and then re-add the label."
)

var (
	pickMustHaveMilestoneBody = fmt.Sprintf(pickMustHaveMilestoneFormat, cpCandidateLabel)
)

// PickMustHaveMilestone will remove the cherrypick-candidate label from
// any PR that does not have a 'release' milestone set.
type PickMustHaveMilestone struct{}

func init() {
	p := PickMustHaveMilestone{}
	RegisterMungerOrDie(p)
	RegisterStaleIssueComments(p)
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

// RegisterOptions registers options for this munger; returns any that require a restart when changed.
func (PickMustHaveMilestone) RegisterOptions(opts *options.Options) sets.String { return nil }

// Munge is the workhorse that will actually make updates to the PR
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

func (PickMustHaveMilestone) isStaleIssueComment(obj *github.MungeObject, comment *githubapi.IssueComment) bool {
	if !obj.IsRobot(comment.User) {
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

// StaleIssueComments returns a slice of stale issue comments.
func (p PickMustHaveMilestone) StaleIssueComments(obj *github.MungeObject, comments []*githubapi.IssueComment) []*githubapi.IssueComment {
	return forEachCommentTest(obj, comments, p.isStaleIssueComment)
}
