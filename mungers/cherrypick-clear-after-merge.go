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
	"bytes"
	"fmt"
	"strings"

	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

const (
	clearAfterMergeName = "cherrypick-clear-after-merge"
)

// ClearPickAfterMerge will remove the the cherrypick-candidate label from
// any PR that does not have a 'release' milestone set.
type ClearPickAfterMerge struct {
	features *features.Features
}

func init() {
	RegisterMungerOrDie(&ClearPickAfterMerge{})
}

// Name is the name usable in --pr-mungers
func (c *ClearPickAfterMerge) Name() string { return clearAfterMergeName }

// RequiredFeatures is a slice of 'features' that must be provided
func (c *ClearPickAfterMerge) RequiredFeatures() []string { return []string{features.RepoFeatureName} }

// Initialize will initialize the munger
func (c *ClearPickAfterMerge) Initialize(config *github.Config, features *features.Features) error {
	c.features = features
	return nil
}

// EachLoop is called at the start of every munge loop
func (c *ClearPickAfterMerge) EachLoop() error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (c *ClearPickAfterMerge) AddFlags(cmd *cobra.Command, config *github.Config) {}

func handleFound(obj *github.MungeObject, gitMsg []byte, branch string) error {
	msg := string(gitMsg)
	o := strings.SplitN(msg, "\n", 2)
	sha := o[0]
	msg = fmt.Sprintf("Commit %s found in the %q branch appears to be this PR. Removing the %q label. If this s an error find help to get your PR picked.", sha, branch, cpCandidateLabel)
	obj.WriteComment(msg)
	obj.RemoveLabel(cpCandidateLabel)
	return nil
}

// Munge is the workhorse the will actually make updates to the PR
func (c *ClearPickAfterMerge) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}
	if !obj.HasLabel(cpCandidateLabel) {
		return
	}

	if merged, err := obj.IsMerged(); !merged || err != nil {
		return
	}

	releaseMilestone := obj.ReleaseMilestone()
	if releaseMilestone == "" || len(releaseMilestone) != 4 {
		glog.Errorf("Found invalid milestone: %q", releaseMilestone)
		return
	}
	rel := releaseMilestone[1:]
	branch := "release-" + rel

	sha := obj.MergeCommit()
	if sha == nil {
		glog.Errorf("Unable to get SHA of merged %d", sha)
		return
	}

	logMsg := fmt.Sprintf("Merge pull request #%d from ", *obj.Issue.Number)
	bLogMsg := []byte(logMsg)

	cherrypickMsg := fmt.Sprintf("(cherry picked from commit %s)", *sha)
	args := []string{"log", "--pretty=tformat:%H%n%s%n%b", "--grep", cherrypickMsg, "origin/" + branch}
	out, err := c.features.Repos.GitCommand(args)
	if err != nil {
		glog.Errorf("Error grepping for cherrypick -x message out=%q: %v", string(out), err)
		return
	}
	if bytes.Contains(out, bLogMsg) {
		glog.Infof("Found cherry-pick using -x information")
		handleFound(obj, out, branch)
		return
	}

	args = []string{"log", "--pretty=tformat:%H%n%s%n%b", "--grep", logMsg, "origin/" + branch}
	out, err = c.features.Repos.GitCommand(args)
	if err != nil {
		glog.Errorf("Error grepping for log message out=%q: %v", string(out), err)
		return
	}
	if bytes.Contains(out, bLogMsg) {
		glog.Infof("Found cherry-pick using log matching")
		handleFound(obj, out, branch)
		return
	}

	return
}
