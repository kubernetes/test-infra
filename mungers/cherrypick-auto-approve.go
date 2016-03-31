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
	"strconv"
	"strings"

	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

const (
	cherrypickAutoApprove = "cherrypick-auto-approve"
)

var (
	cpRe = regexp.MustCompile(`Cherry pick of #([[:digit:]]+) on release-([[:digit:]]+\.[[:digit:]]+).`)
)

// CherrypickAutoApprove will add 'cherrypick-approved' to PRs which are
// for 'cherrypick-approved' parents. This only works if the PR (against
// the 'release-*' branch was done using the script.
type CherrypickAutoApprove struct {
	config *github.Config
}

func init() {
	RegisterMungerOrDie(&CherrypickAutoApprove{})
}

// Name is the name usable in --pr-mungers
func (c *CherrypickAutoApprove) Name() string { return cherrypickAutoApprove }

// RequiredFeatures is a slice of 'features' that must be provided
func (c *CherrypickAutoApprove) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (c *CherrypickAutoApprove) Initialize(config *github.Config, features *features.Features) error {
	c.config = config
	return nil
}

// EachLoop is called at the start of every munge loop
func (c *CherrypickAutoApprove) EachLoop() error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (c *CherrypickAutoApprove) AddFlags(cmd *cobra.Command, config *github.Config) {}

// Munge is the workhorse the will actually make updates to the PR
func (c *CherrypickAutoApprove) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}
	if obj.IsForBranch("master") {
		return
	}
	if obj.HasLabel(cpApprovedLabel) && obj.ReleaseMilestone() != "" {
		return
	}

	if obj.Issue.Body == nil {
		glog.Errorf("Found a nil body in %d", *obj.Issue.Number)
	}
	body := *obj.Issue.Body

	// foundOne tracks if we found any valid lines. PR without any valid lines
	// shouldn't get autolabeled.
	foundOne := false
	parentReleaseMilestone := ""

	lines := strings.Split(body, "\n")
	for _, line := range lines {
		matches := cpRe.FindStringSubmatch(line)
		if len(matches) != 3 {
			glog.V(6).Infof("%d: line:%v len(matches)=%d", *obj.Issue.Number, line, len(matches))
			continue
		}
		parentPRNum, err := strconv.Atoi(matches[1])
		if err != nil {
			glog.Errorf("%d: Unable to convert %q to parent PR number", *obj.Issue.Number, matches[1])
			return
		}
		parentPR, err := c.config.GetObject(parentPRNum)
		if err != nil {
			glog.Errorf("Unable to get object for %d", parentPRNum)
			return
		}

		if !parentPR.HasLabel(cpApprovedLabel) {
			return
		}

		parentReleaseMilestone = parentPR.ReleaseMilestone()
		milestone := fmt.Sprintf("v%s", matches[2])

		// If the parent was for milestone v1.2 but this PR has
		// comments saying it was 'on branch release-1.1' we should
		// not auto approve
		if parentReleaseMilestone != milestone {
			glog.Errorf("%d: parentReleaseMilestone=%q but comments are for %q", *obj.Issue.Number, parentReleaseMilestone, milestone)
			return
		}

		// If the comment is 'ont branch release-1.1' but the PR is
		// against release-1.2 we should not auto approve.
		targetBranch := fmt.Sprintf("release-%s", matches[2])
		if !obj.IsForBranch(targetBranch) {
			glog.Errorf("%d: is not for the expected branch: %q", *obj.Issue.Number, targetBranch)
			return
		}
		foundOne = true
	}
	if foundOne {
		if obj.ReleaseMilestone() == "" {
			obj.SetMilestone(parentReleaseMilestone)
		}
		obj.AddLabel(cpApprovedLabel)
	}
}
