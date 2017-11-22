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
	"k8s.io/test-infra/mungegithub/mungers/mungerutil"
	"k8s.io/test-infra/mungegithub/options"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
)

const (
	lgtmRemovedBody = "/lgtm cancel //PR changed after LGTM, removing LGTM. %s"
)

var (
	lgtmRemovedRegex = regexp.MustCompile("/lgtm cancel //PR changed after LGTM, removing LGTM.")
)

// LGTMAfterCommitMunger will remove the LGTM flag from an PR which has been
// updated since the reviewer added LGTM
type LGTMAfterCommitMunger struct{}

func init() {
	l := LGTMAfterCommitMunger{}
	RegisterMungerOrDie(l)
	RegisterStaleIssueComments(l)
}

// Name is the name usable in --pr-mungers
func (LGTMAfterCommitMunger) Name() string { return "lgtm-after-commit" }

// RequiredFeatures is a slice of 'features' that must be provided
func (LGTMAfterCommitMunger) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (LGTMAfterCommitMunger) Initialize(config *github.Config, features *features.Features) error {
	return nil
}

// EachLoop is called at the start of every munge loop
func (LGTMAfterCommitMunger) EachLoop() error { return nil }

// RegisterOptions registers options for this munger; returns any that require a restart when changed.
func (LGTMAfterCommitMunger) RegisterOptions(opts *options.Options) sets.String { return nil }

// Munge is the workhorse the will actually make updates to the PR
func (LGTMAfterCommitMunger) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	if !obj.HasLabel(lgtmLabel) {
		return
	}

	lastModified, ok := obj.LastModifiedTime()
	lgtmTime, ok2 := obj.LabelTime(lgtmLabel)

	if !ok || !ok2 || lastModified == nil || lgtmTime == nil {
		glog.Errorf("PR %d unable to determine lastModified or lgtmTime", *obj.Issue.Number)
		return
	}

	if lastModified.After(*lgtmTime) {
		glog.Infof("PR: %d lgtm:%s  lastModified:%s", *obj.Issue.Number, lgtmTime.String(), lastModified.String())
		body := fmt.Sprintf(lgtmRemovedBody, mungerutil.GetIssueUsers(obj.Issue).AllUsers().Mention().Join())
		if err := obj.WriteComment(body); err != nil {
			return
		}
		obj.RemoveLabel(lgtmLabel)
	}
}

func (LGTMAfterCommitMunger) isStaleIssueComment(obj *github.MungeObject, comment *githubapi.IssueComment) bool {
	if !obj.IsRobot(comment.User) {
		return false
	}
	if !lgtmRemovedRegex.MatchString(*comment.Body) {
		return false
	}
	if !obj.HasLabel(lgtmLabel) {
		return false
	}
	lgtmTime, ok := obj.LabelTime(lgtmLabel)
	if lgtmTime == nil || !ok {
		return false
	}
	stale := lgtmTime.After(*comment.CreatedAt)
	if stale {
		glog.V(6).Infof("Found stale LGTMAfterCommitMunger comment")
	}
	return stale
}

// StaleIssueComments returns a list of stale issue comments.
func (l LGTMAfterCommitMunger) StaleIssueComments(obj *github.MungeObject, comments []*githubapi.IssueComment) []*githubapi.IssueComment {
	return forEachCommentTest(obj, comments, l.isStaleIssueComment)
}
