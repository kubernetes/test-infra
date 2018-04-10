/*
Copyright 2016 The Kubernetes Authors.

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
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/mungegithub/features"
	"k8s.io/test-infra/mungegithub/github"
	"k8s.io/test-infra/mungegithub/options"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
)

const (
	cherrypickUnapprovedLabel           = "do-not-merge/cherry-pick-not-approved"
	deprecatedCherrypickUnapprovedLabel = "do-not-merge"
	labelUnapprovedPicksName            = "label-unapproved-picks"
	labelUnapprovedFormat               = "This PR is not for the master branch but does not have the `%s` label. Adding the `%s` label."
)

var (
	labelUnapprovedBody           = fmt.Sprintf(labelUnapprovedFormat, cpApprovedLabel, cherrypickUnapprovedLabel)
	deprecatedLabelUnapprovedBody = fmt.Sprintf(labelUnapprovedFormat, cpApprovedLabel, deprecatedCherrypickUnapprovedLabel)
)

// LabelUnapprovedPicks will add `do-not-merge` to PRs against a release branch which
// do not have `cherrypick-approved`.
type LabelUnapprovedPicks struct{}

func init() {
	l := LabelUnapprovedPicks{}
	RegisterMungerOrDie(l)
	RegisterStaleIssueComments(l)
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

// RegisterOptions registers options for this munger; returns any that require a restart when changed.
func (LabelUnapprovedPicks) RegisterOptions(opts *options.Options) sets.String { return nil }

// Munge is the workhorse the will actually make updates to the PR
func (LabelUnapprovedPicks) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}
	branch, ok := obj.Branch()
	if !ok {
		return
	}
	if !strings.HasPrefix(branch, "release-") {
		return
	}

	if obj.HasLabel(cpApprovedLabel) {
		if obj.HasLabel(cherrypickUnapprovedLabel) {
			obj.RemoveLabel(cherrypickUnapprovedLabel)
		}
		return
	}

	if obj.HasLabel(cherrypickUnapprovedLabel) {
		return
	}

	obj.AddLabel(cherrypickUnapprovedLabel)

	obj.WriteComment(labelUnapprovedBody)
}

func (LabelUnapprovedPicks) isStaleIssueComment(obj *github.MungeObject, comment *githubapi.IssueComment) bool {
	if !obj.IsRobot(comment.User) {
		return false
	}
	if *comment.Body != labelUnapprovedBody && *comment.Body != deprecatedLabelUnapprovedBody {
		return false
	}
	stale := obj.HasLabel(cpApprovedLabel)
	if stale {
		glog.V(6).Infof("Found stale LabelUnapprovedPicks comment")
	}
	return stale
}

// StaleIssueComments returns a list of stale issue comments.
func (l LabelUnapprovedPicks) StaleIssueComments(obj *github.MungeObject, comments []*githubapi.IssueComment) []*githubapi.IssueComment {
	return forEachCommentTest(obj, comments, l.isStaleIssueComment)
}
