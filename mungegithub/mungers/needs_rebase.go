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
	"regexp"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/mungegithub/features"
	"k8s.io/test-infra/mungegithub/github"
	"k8s.io/test-infra/mungegithub/options"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
)

var (
	rebaseRE = regexp.MustCompile(`@\S+ PR needs rebase`)
)

const needsRebase = "needs-rebase"

// NeedsRebaseMunger will add the "needs-rebase" label to any issue which is
// unable to be automatically merged
type NeedsRebaseMunger struct{}

const (
	needsRebaseLabel = "needs-rebase"
)

func init() {
	n := NeedsRebaseMunger{}
	RegisterMungerOrDie(n)
	RegisterStaleIssueComments(n)
}

// Name is the name usable in --pr-mungers
func (NeedsRebaseMunger) Name() string { return "needs-rebase" }

// RequiredFeatures is a slice of 'features' that must be provided
func (NeedsRebaseMunger) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (NeedsRebaseMunger) Initialize(config *github.Config, features *features.Features) error {
	return nil
}

// EachLoop is called at the start of every munge loop
func (NeedsRebaseMunger) EachLoop() error { return nil }

// RegisterOptions registers options for this munger; returns any that require a restart when changed.
func (NeedsRebaseMunger) RegisterOptions(opts *options.Options) sets.String { return nil }

// Munge is the workhorse the will actually make updates to the PR
func (NeedsRebaseMunger) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	mergeable, ok := obj.IsMergeable()
	if !ok {
		glog.V(2).Infof("Skipping %d - problem determining mergeable", *obj.Issue.Number)
		return
	}
	if mergeable && obj.HasLabel(needsRebaseLabel) {
		obj.RemoveLabel(needsRebaseLabel)
	}
	if !mergeable && !obj.HasLabel(needsRebaseLabel) {
		obj.AddLabels([]string{needsRebaseLabel})

		body := fmt.Sprintf("@%s PR needs rebase", *obj.Issue.User.Login)
		if err := obj.WriteComment(body); err != nil {
			return
		}
	}
}

func (NeedsRebaseMunger) isStaleIssueComment(obj *github.MungeObject, comment *githubapi.IssueComment) bool {
	if !obj.IsRobot(comment.User) {
		return false
	}
	if !rebaseRE.MatchString(*comment.Body) {
		return false
	}
	stale := !obj.HasLabel(needsRebaseLabel)
	if stale {
		glog.V(6).Infof("Found stale NeedsRebaseMunger comment")
	}
	return stale
}

// StaleIssueComments returns a slice of stale issue comments.
func (n NeedsRebaseMunger) StaleIssueComments(obj *github.MungeObject, comments []*githubapi.IssueComment) []*githubapi.IssueComment {
	return forEachCommentTest(obj, comments, n.isStaleIssueComment)
}
