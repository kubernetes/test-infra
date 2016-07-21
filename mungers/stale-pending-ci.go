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
	"time"

	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
	"github.com/spf13/cobra"
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
type StalePendingCI struct {
	features *features.Features
}

func init() {
	s := &StalePendingCI{}
	RegisterMungerOrDie(s)
	RegisterStaleComments(s)
}

// Name is the name usable in --pr-mungers
func (s *StalePendingCI) Name() string { return "stale-pending-ci" }

// RequiredFeatures is a slice of 'features' that must be provided
func (s *StalePendingCI) RequiredFeatures() []string { return []string{features.TestOptionsFeature} }

// Initialize will initialize the munger
func (s *StalePendingCI) Initialize(config *github.Config, features *features.Features) error {
	s.features = features
	return nil
}

// EachLoop is called at the start of every munge loop
func (s *StalePendingCI) EachLoop() error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (s *StalePendingCI) AddFlags(cmd *cobra.Command, config *github.Config) {}

// Munge is the workhorse the will actually make updates to the PR
func (s *StalePendingCI) Munge(obj *github.MungeObject) {
	requiredContexts := s.features.TestOptions.RequiredRetestContexts
	if !obj.IsPR() {
		return
	}

	if !obj.HasLabel(lgtmLabel) {
		return
	}

	if mergeable, err := obj.IsMergeable(); !mergeable || err != nil {
		return
	}

	status := obj.GetStatusState(requiredContexts)
	if status != "pending" {
		return
	}

	for _, context := range requiredContexts {
		statusTime := obj.GetStatusTime(context)
		if statusTime == nil {
			glog.Errorf("%d: unable to determine time %q context was set", *obj.Issue.Number, context)
			return
		}
		if time.Since(*statusTime) > stalePendingCIHours*time.Hour {
			obj.WriteComment(pendingMsgBody)
			return
		}
	}
}

func (s *StalePendingCI) isStaleComment(obj *github.MungeObject, comment *githubapi.IssueComment) bool {
	if !mergeBotComment(comment) {
		return false
	}
	if *comment.Body != pendingMsgBody {
		return false
	}
	stale := commentBeforeLastCI(obj, comment, s.features.TestOptions.RequiredRetestContexts)
	if stale {
		glog.V(6).Infof("Found stale StalePendingCI comment")
	}
	return stale
}

// StaleComments returns a slice of stale comments
func (s *StalePendingCI) StaleComments(obj *github.MungeObject, comments []*githubapi.IssueComment) []*githubapi.IssueComment {
	return forEachCommentTest(obj, comments, s.isStaleComment)
}
