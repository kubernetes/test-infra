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
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/mungegithub/features"
	"k8s.io/test-infra/mungegithub/github"
	"k8s.io/test-infra/mungegithub/mungeopts"
	"k8s.io/test-infra/mungegithub/options"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
)

const (
	stalePendingCIHours = 24
	pendingMsgFormat    = `@` + jenkinsBotName + ` test this issue: #IGNORE

Tests have been pending for %d hours`
)

var (
	pendingMsgBody = fmt.Sprintf(pendingMsgFormat, stalePendingCIHours)
)

// StalePendingCI will ask the testBot-to test any PR with a LGTM that has
// been pending for more than 24 hours. This can happen when the jenkins VM
// is restarted.
//
// The real fix would be for the jenkins VM restart to not move every single
// PR to pending without actually testing...
//
// But this is our world and so we should really do this for all PRs which
// aren't likely to get another push (everything that is mergeable). Since that
// can be a lot of PRs, I'm just doing it for the LGTM PRs automatically...
type StalePendingCI struct{}

func init() {
	s := &StalePendingCI{}
	RegisterMungerOrDie(s)
	RegisterStaleIssueComments(s)
}

// Name is the name usable in --pr-mungers
func (s *StalePendingCI) Name() string { return "stale-pending-ci" }

// RequiredFeatures is a slice of 'features' that must be provided
func (s *StalePendingCI) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (s *StalePendingCI) Initialize(config *github.Config, features *features.Features) error {
	return nil
}

// EachLoop is called at the start of every munge loop
func (s *StalePendingCI) EachLoop() error { return nil }

// RegisterOptions registers options for this munger; returns any that require a restart when changed.
func (s *StalePendingCI) RegisterOptions(opts *options.Options) sets.String { return nil }

// Munge is the workhorse the will actually make updates to the PR
func (s *StalePendingCI) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	if !obj.HasLabel(lgtmLabel) {
		return
	}

	if mergeable, ok := obj.IsMergeable(); !ok || !mergeable {
		return
	}

	status, ok := obj.GetStatusState(mungeopts.RequiredContexts.Retest)
	if !ok || status != "pending" {
		return
	}

	for _, context := range mungeopts.RequiredContexts.Retest {
		statusTime, ok := obj.GetStatusTime(context)
		if !ok || statusTime == nil {
			glog.Errorf("%d: unable to determine time %q context was set", *obj.Issue.Number, context)
			return
		}
		if time.Since(*statusTime) > stalePendingCIHours*time.Hour {
			obj.WriteComment(pendingMsgBody)
			return
		}
	}
}

func isStaleIssueComment(obj *github.MungeObject, comment *githubapi.IssueComment) bool {
	if !obj.IsRobot(comment.User) {
		return false
	}
	if *comment.Body != pendingMsgBody {
		return false
	}
	stale := commentBeforeLastCI(obj, comment, mungeopts.RequiredContexts.Retest)
	if stale {
		glog.V(6).Infof("Found stale StalePendingCI comment")
	}
	return stale
}

// StaleIssueComments returns a slice of stale issue comments.
func (s *StalePendingCI) StaleIssueComments(obj *github.MungeObject, comments []*githubapi.IssueComment) []*githubapi.IssueComment {
	if mungeopts.RequiredContexts.Retest == nil {
		return nil // mungers not initialized, cannot clean stale comments.
	}
	return forEachCommentTest(obj, comments, isStaleIssueComment)
}
