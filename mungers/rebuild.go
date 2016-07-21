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
	"regexp"
	"strings"

	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
	"github.com/spf13/cobra"
	"k8s.io/kubernetes/pkg/util/sets"
)

// RebuildMunger looks for situations where a someone has asked for an e2e rebuild, but hasn't provided
// an issue
type RebuildMunger struct {
	robots   sets.String
	features *features.Features
}

const (
	issueURLRe    = "(?:https?://)?github.com/kubernetes/kubernetes/issues/[0-9]+"
	rebuildFormat = `@%s
You must link to the test flake issue which caused you to request this manual re-test.
Re-test requests should be in the form of: ` + "`" + jenkinsBotName + ` test this issue: #<number>` + "`" + `
Here is the [list of open test flakes](https://github.com/kubernetes/kubernetes/issues?q=is:issue+label:kind/flake+is:open).`
)

var (
	buildMatcherStr = fmt.Sprintf("@%s\\s+(?:e2e\\s+)?(?:unit\\s+)?test\\s+this.*", jenkinsBotName)
	buildMatcher    = regexp.MustCompile(buildMatcherStr)
	issueMatcher    = regexp.MustCompile("\\s+(?:github\\s+)?(issue|flake)\\:?\\s+(?:#(?:IGNORE|[0-9]+)|" + issueURLRe + ")")

	// take the format and replace the %s with \S+
	rebuildCommentREString = strings.Replace(rebuildFormat, `@%s`, `@\S+`, 1)
	rebuildCommentRE       = regexp.MustCompile(rebuildCommentREString)
)

func init() {
	r := &RebuildMunger{}
	RegisterMungerOrDie(r)
	RegisterStaleComments(r)
}

// Name is the name usable in --pr-mungers
func (r *RebuildMunger) Name() string { return "rebuild-request" }

// RequiredFeatures is a slice of 'features' that must be provided
func (r *RebuildMunger) RequiredFeatures() []string { return []string{features.TestOptionsFeature} }

// Initialize will initialize the munger
func (r *RebuildMunger) Initialize(config *github.Config, features *features.Features) error {
	r.robots = sets.NewString("googlebot", jenkinsBotName, botName)
	r.features = features
	return nil
}

// EachLoop is called at the start of every munge loop
func (r *RebuildMunger) EachLoop() error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (r *RebuildMunger) AddFlags(cmd *cobra.Command, config *github.Config) {}

// Munge is the workhorse the will actually make updates to the PR
func (r *RebuildMunger) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	comments, err := obj.ListComments()
	if err != nil {
		glog.Errorf("unexpected error getting comments: %v", err)
	}

	for ix := range comments {
		comment := comments[ix]
		// Skip all robot comments
		if r.robots.Has(*comment.User.Login) {
			glog.V(4).Infof("Skipping comment by robot %s: %s", *comment.User.Login, *comment.Body)
			continue
		}
		if isRebuildComment(comment) && rebuildCommentMissingIssueNumber(comment) {
			if err := obj.DeleteComment(comment); err != nil {
				glog.Errorf("Error deleting comment: %v", err)
				continue
			}
			body := fmt.Sprintf(rebuildFormat, *comment.User.Login)
			err := obj.WriteComment(body)
			if err != nil {
				glog.Errorf("unexpected error adding comment: %v", err)
				continue
			}
			if obj.HasLabel(lgtmLabel) {
				if err := obj.RemoveLabel(lgtmLabel); err != nil {
					glog.Errorf("unexpected error removing lgtm label: %v", err)
				}
			}
		}
	}
}

func isRebuildComment(comment *githubapi.IssueComment) bool {
	return buildMatcher.MatchString(*comment.Body)
}

func rebuildCommentMissingIssueNumber(comment *githubapi.IssueComment) bool {
	return !issueMatcher.MatchString(*comment.Body)
}

func (r *RebuildMunger) isStaleComment(obj *github.MungeObject, comment *githubapi.IssueComment) bool {
	if !mergeBotComment(comment) {
		return false
	}
	if !rebuildCommentRE.MatchString(*comment.Body) {
		return false
	}
	stale := commentBeforeLastCI(obj, comment, r.features.TestOptions.RequiredRetestContexts)
	if stale {
		glog.V(6).Infof("Found stale RebuildMunger comment")
	}
	return stale
}

// StaleComments returns a slice of stale comments
func (r *RebuildMunger) StaleComments(obj *github.MungeObject, comments []*githubapi.IssueComment) []*githubapi.IssueComment {
	return forEachCommentTest(obj, comments, r.isStaleComment)
}
