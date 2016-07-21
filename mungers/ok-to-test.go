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
	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
	"github.com/spf13/cobra"
)

const (
	okToTestBody = `ok to test
@` + jenkinsBotName + ` test this

pr builder appears to be missing, activating due to 'lgtm' label.`
)

// OkToTestMunger looks for situations where a reviewer has LGTM'd a PR, but it
// isn't ok to test by the testBot, and adds an 'ok to test' comment to the PR.
type OkToTestMunger struct {
	features *features.Features
}

func init() {
	ok := &OkToTestMunger{}
	RegisterMungerOrDie(ok)
	RegisterStaleComments(ok)
}

// Name is the name usable in --pr-mungers
func (ok *OkToTestMunger) Name() string { return "ok-to-test" }

// RequiredFeatures is a slice of 'features' that must be provided
func (ok *OkToTestMunger) RequiredFeatures() []string { return []string{features.TestOptionsFeature} }

// Initialize will initialize the munger
func (ok *OkToTestMunger) Initialize(config *github.Config, features *features.Features) error {
	ok.features = features
	return nil
}

// EachLoop is called at the start of every munge loop
func (ok *OkToTestMunger) EachLoop() error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (ok *OkToTestMunger) AddFlags(cmd *cobra.Command, config *github.Config) {}

// Munge is the workhorse the will actually make updates to the PR
func (ok *OkToTestMunger) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	if !obj.HasLabel(lgtmLabel) {
		return
	}
	state := obj.GetStatusState(ok.features.TestOptions.RequiredRetestContexts)
	if state == "incomplete" {
		glog.V(2).Infof("status is incomplete, adding ok to test")
		obj.WriteComment(okToTestBody)
	}
}

func (ok *OkToTestMunger) isStaleComment(obj *github.MungeObject, comment *githubapi.IssueComment) bool {
	if !mergeBotComment(comment) {
		return false
	}
	if *comment.Body != okToTestBody {
		return false
	}
	stale := commentBeforeLastCI(obj, comment, ok.features.TestOptions.RequiredRetestContexts)
	if stale {
		glog.V(6).Infof("Found stale OkToTestMunger comment")
	}
	return stale
}

// StaleComments returns a slice of comments which are stale
func (ok *OkToTestMunger) StaleComments(obj *github.MungeObject, comments []*githubapi.IssueComment) []*githubapi.IssueComment {
	return forEachCommentTest(obj, comments, ok.isStaleComment)
}
