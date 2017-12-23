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

func handleFound(obj *github.MungeObject, branch string) error {
	msg := fmt.Sprintf("Commit found in the %q branch appears to be this PR. Removing the %q label. If this is an error find help to get your PR picked.", branch, cpCandidateLabel)
	obj.WriteComment(msg)
	obj.RemoveLabel(cpCandidateLabel)
	return nil
}

// foundLog will return if the given `logString` exists on the branch in question.
// it will also return the actual logs for further processing
func (c *ClearPickAfterMerge) foundLog(branch string, logString string) (bool, string) {
	args := []string{"merge-base", "origin/master", "origin/" + branch}
	out, err := c.features.Repos.GitCommand(args)
	base := string(out)
	if err != nil {
		glog.Errorf("Unable to find the fork point for branch %s. %s:%v", branch, base, err)
		return false, ""
	}
	lines := strings.Split(base, "\n")
	if len(lines) < 1 {
		glog.Errorf("Found 0 lines splitting the results of git merge-base")
	}
	base = lines[0]

	// if release-1.2 branched from master at abcdef123 this should result in:
	// abcdef123..origin/release-1.2
	logRefs := fmt.Sprintf("%s..origin/%s", base, branch)

	args = []string{"log", "--pretty=tformat:%H%n%s%n%b", "--grep", logString, logRefs}
	out, err = c.features.Repos.GitCommand(args)
	logs := string(out)
	if err != nil {
		glog.Errorf("Error grepping logs out=%q: %v", logs, err)
		return false, ""
	}
	glog.V(10).Infof("args:%v", args)

	glog.V(10).Infof("Searching for %q in %q", logString, logs)
	if !strings.Contains(logs, logString) {
		return false, ""
	}
	return true, logs
}

// Can we find a commit in the changelog that looks like it was done using git cherry-pick -m1 -x ?
func (c *ClearPickAfterMerge) foundByPickDashX(obj *github.MungeObject, branch string) bool {
	sha := obj.MergeCommit()
	if sha == nil {
		glog.Errorf("Unable to get SHA of merged PR %d", *obj.Issue.Number)
		return false
	}

	cherrypickMsg := fmt.Sprintf("(cherry picked from commit %s)", *sha)
	found, logs := c.foundLog(branch, cherrypickMsg)
	if !found {
		return false
	}

	// double check for the 'non -x' message
	logMsg := fmt.Sprintf("Merge pull request #%d from ", *obj.Issue.Number)
	if !strings.Contains(logs, logMsg) {
		return false
	}
	glog.Infof("Found cherry-pick for %d using -x information in branch %q", *obj.Issue.Number, branch)
	return true
}

// Can we find a commit in the changelog that looks like it was done using git cherry-pick -m1 ?
func (c *ClearPickAfterMerge) foundByPickWithoutDashX(obj *github.MungeObject, branch string) bool {
	logMsg := fmt.Sprintf("Merge pull request #%d from ", *obj.Issue.Number)

	found, _ := c.foundLog(branch, logMsg)
	if found {
		glog.Infof("Found cherry-pick for %d using log matching for `git cherry-pick` in branch %q", *obj.Issue.Number, branch)
	}
	return found
}

// Check that the commit messages for all commits in the PR are on the branch
func (c *ClearPickAfterMerge) foundByAllCommits(obj *github.MungeObject, branch string) bool {
	commits, err := obj.GetCommits()
	if err != nil {
		glog.Infof("unable to get commits")
		return false
	}
	for _, commit := range commits {
		if commit.Commit == nil {
			return false
		}
		if commit.Commit.Message == nil {
			return false
		}
		found, _ := c.foundLog(branch, *commit.Commit.Message)
		if !found {
			return false
		}
	}
	return true
}

// Can we find a commit in the changelog that looks like it was done using the hack/cherry_pick_pull.sh script ?
func (c *ClearPickAfterMerge) foundByScript(obj *github.MungeObject, branch string) bool {
	logMsg := fmt.Sprintf("Cherry pick of #%d on %s.", *obj.Issue.Number, branch)

	found, _ := c.foundLog(branch, logMsg)
	if found {
		glog.Infof("Found cherry-pick for %d using log matching for `hack/cherry_pick_pull.sh` in branch %q", *obj.Issue.Number, branch)
	}
	return found
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

	if c.foundByPickDashX(obj, branch) {
		handleFound(obj, branch)
		return
	}

	if c.foundByAllCommits(obj, branch) {
		handleFound(obj, branch)
		return
	}

	if c.foundByPickWithoutDashX(obj, branch) {
		handleFound(obj, branch)
		return
	}

	if c.foundByScript(obj, branch) {
		handleFound(obj, branch)
		return
	}

	return
}
