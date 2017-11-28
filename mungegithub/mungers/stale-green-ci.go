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
	"sync"
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
	staleGreenCIHours = 96
	greenMsgFormat    = `/test all

Tests are more than %d hours old. Re-running tests.`
)

var greenMsgBody = fmt.Sprintf(greenMsgFormat, staleGreenCIHours)

// StaleGreenCI will re-run passed tests for LGTM PRs if they are more than
// 96 hours old.
type StaleGreenCI struct {
	getRetestContexts func() []string
	features          *features.Features
	opts              *options.Options

	waitingForPending map[int]struct{}
	sync.Mutex
}

func init() {
	s := &StaleGreenCI{}
	RegisterMungerOrDie(s)
	RegisterStaleIssueComments(s)
}

// Name is the name usable in --pr-mungers
func (s *StaleGreenCI) Name() string { return "stale-green-ci" }

// RequiredFeatures is a slice of 'features' that must be provided
func (s *StaleGreenCI) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (s *StaleGreenCI) Initialize(config *github.Config, features *features.Features) error {
	s.features = features
	s.waitingForPending = map[int]struct{}{}
	return nil
}

// EachLoop is called at the start of every munge loop
func (s *StaleGreenCI) EachLoop() error { return nil }

// RegisterOptions registers options for this munger; returns any that require a restart when changed.
func (s *StaleGreenCI) RegisterOptions(opts *options.Options) sets.String {
	s.opts = opts
	return nil
}

// Munge is the workhorse the will actually make updates to the PR
func (s *StaleGreenCI) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	// Avoid leaving multiple comments before the retest job is triggered.
	s.Lock()
	_, ok := s.waitingForPending[*obj.Issue.Number]
	s.Unlock()
	if ok {
		return // Already commented with trigger command. Still waiting for pending state.
	}

	if !obj.HasLabel(lgtmLabel) {
		return
	}

	if obj.HasLabel(retestNotRequiredLabel) || obj.HasLabel(retestNotRequiredDocsOnlyLabel) {
		return
	}

	if mergeable, ok := obj.IsMergeable(); !mergeable || !ok {
		return
	}

	s.opts.Lock()
	requiredContexts := mungeopts.RequiredContexts.Retest
	prMaxWaitTime := mungeopts.PRMaxWaitTime
	s.opts.Unlock()
	if success, ok := obj.IsStatusSuccess(requiredContexts); !success || !ok {
		return
	}

	for _, context := range requiredContexts {
		statusTime, ok := obj.GetStatusTime(context)
		if statusTime == nil || !ok {
			glog.Errorf("%d: unable to determine time %q context was set", *obj.Issue.Number, context)
			return
		}
		if time.Since(*statusTime) > staleGreenCIHours*time.Hour {
			err := obj.WriteComment(greenMsgBody)
			if err != nil {
				glog.Errorf("Failed to write retrigger old test comment")
				return
			}
			s.Lock()
			s.waitingForPending[*obj.Issue.Number] = struct{}{}
			s.Unlock()
			go s.waitForPending(requiredContexts, obj, prMaxWaitTime)
			return
		}
	}
}

// waitForPending is an asynchronous wrapper for obj.WaitForPending that marks the obj as handled
// when the status changes to pending or the timeout expires.
func (s *StaleGreenCI) waitForPending(requiredContexts []string, obj *github.MungeObject, maxWait time.Duration) {
	if !obj.WaitForPending(requiredContexts, maxWait) {
		glog.Errorf("Failed waiting for PR #%d to start testing", *obj.Issue.Number)
	}
	s.Lock()
	defer s.Unlock()
	delete(s.waitingForPending, *obj.Issue.Number)
}

func (s *StaleGreenCI) isStaleIssueComment(obj *github.MungeObject, comment *githubapi.IssueComment) bool {
	if !obj.IsRobot(comment.User) {
		return false
	}
	if *comment.Body != greenMsgBody {
		return false
	}
	stale := commentBeforeLastCI(obj, comment, mungeopts.RequiredContexts.Retest)
	if stale {
		glog.V(6).Infof("Found stale StaleGreenCI comment")
	}
	return stale
}

// StaleIssueComments returns a slice of stale issue comments.
func (s *StaleGreenCI) StaleIssueComments(obj *github.MungeObject, comments []*githubapi.IssueComment) []*githubapi.IssueComment {
	return forEachCommentTest(obj, comments, s.isStaleIssueComment)
}

func commentBeforeLastCI(obj *github.MungeObject, comment *githubapi.IssueComment, requiredContexts []string) bool {
	if success, ok := obj.IsStatusSuccess(requiredContexts); !success || !ok {
		return false
	}
	if comment.CreatedAt == nil {
		return false
	}
	commentTime := *comment.CreatedAt

	for _, context := range requiredContexts {
		statusTimeP, ok := obj.GetStatusTime(context)
		if statusTimeP == nil || !ok {
			return false
		}
		statusTime := statusTimeP.Add(30 * time.Minute)
		if commentTime.After(statusTime) {
			return false
		}
	}
	return true
}
