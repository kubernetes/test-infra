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

	"k8s.io/contrib/mungegithub/github"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
	"github.com/spf13/cobra"
)

// RebuildMunger looks for situations where a someone has asked for an e2e rebuild, but hasn't provided
// an issue
type RebuildMunger struct{}

var (
	buildMatcher = regexp.MustCompile("@k8s-bot\\s+(?:e2e\\s+)?(?:unit\\s+)?test\\s+this.*")
	issueMatcher = regexp.MustCompile("\\s+(?:github\\s+)?(issue|flake)\\:?\\s+#(?:IGNORE|[0-9]+)")
)

func init() {
	RegisterMungerOrDie(RebuildMunger{})
}

// Name is the name usable in --pr-mungers
func (RebuildMunger) Name() string { return "rebuild-request" }

// Initialize will initialize the munger
func (RebuildMunger) Initialize(config *github.Config) error { return nil }

// EachLoop is called at the start of every munge loop
func (RebuildMunger) EachLoop() error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (RebuildMunger) AddFlags(cmd *cobra.Command, config *github.Config) {}

// Munge is the workhorse the will actually make updates to the PR
func (RebuildMunger) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	comments, err := obj.ListComments(*obj.Issue.Number)
	if err != nil {
		glog.Errorf("unexpected error getting comments: %v", err)
	}

	for ix := range comments {
		comment := &comments[ix]
		// Skip all robot comments
		for _, robot := range []string{"k8s-bot", "k8s-merge-robot", "googlebot"} {
			if *comment.User.Login == robot {
				glog.V(4).Infof("Skipping comment by robot %s: %s", robot, *comment.Body)
				continue
			}
		}
		if isRebuildComment(comment) && rebuildCommentMissingIssueNumber(comment) {
			if err := obj.DeleteComment(comment); err != nil {
				glog.Errorf("Error deleting comment: %v", err)
				continue
			}
			body := fmt.Sprintf(`@%s an issue is required for any manual rebuild.  Expecting comment of the form 'github issue: #<number>'
[Open test flakes](https://github.com/kubernetes/kubernetes/issues?q=is%%3Aissue%%20label%%3Akind%%2Fflake)`, *comment.User.Login)
			err := obj.WriteComment(body)
			if err != nil {
				glog.Errorf("unexpected error adding comment: %v", err)
				continue
			}
			if obj.HasLabel("lgtm") {
				if err := obj.RemoveLabel("lgtm"); err != nil {
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
