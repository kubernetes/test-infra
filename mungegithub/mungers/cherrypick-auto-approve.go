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
	"regexp"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/mungegithub/features"
	"k8s.io/test-infra/mungegithub/github"
	"k8s.io/test-infra/mungegithub/options"

	"github.com/golang/glog"
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

// RegisterOptions registers options for this munger; returns any that require a restart when changed.
func (c *CherrypickAutoApprove) RegisterOptions(opts *options.Options) sets.String { return nil }

func getCherrypickParentPRs(obj *github.MungeObject, config *github.Config) []*github.MungeObject {
	out := []*github.MungeObject{}
	if obj.Issue.Body == nil {
		glog.Errorf("Found a nil body in %d", *obj.Issue.Number)
		return nil
	}
	body := *obj.Issue.Body

	// foundOne tracks if we found any valid lines. PR without any valid lines
	// shouldn't get autolabeled.

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
			return nil
		}
		parentPR, err := config.GetObject(parentPRNum)
		if err != nil {
			glog.Errorf("Unable to get object for %d", parentPRNum)
			return nil
		}
		out = append(out, parentPR)
	}
	return out
}

// Munge is the workhorse the will actually make updates to the PR
func (c *CherrypickAutoApprove) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}
	branch, ok := obj.Branch()
	if !ok {
		return
	}
	if !strings.HasPrefix(branch, "release-") {
		return
	}

	milestone, ok := obj.ReleaseMilestone()
	if !ok {
		return
	}
	if obj.HasLabel(cpApprovedLabel) && milestone != "" {
		return
	}

	parents := getCherrypickParentPRs(obj, c.config)
	if len(parents) == 0 {
		return
	}

	major := 0
	minor := 0
	if l, err := fmt.Sscanf(branch, "release-%d.%d", &major, &minor); err != nil || l != 2 {
		return
	}
	branchImpliedMilestone := fmt.Sprintf("v%d.%d", major, minor)

	if milestone != "" && milestone != branchImpliedMilestone {
		glog.Errorf("Found PR %d on branch %q but have milestone %q", *obj.Issue.Number, branch, milestone)
		return
	}

	for _, parent := range parents {
		if !parent.HasLabel(cpApprovedLabel) {
			return
		}

		// If the parent was for milestone v1.2 but this PR has
		// comments saying it was 'on branch release-1.1' we should
		// not auto approve
		parentMilestone, ok := parent.ReleaseMilestone()
		if parentMilestone != branchImpliedMilestone || !ok {
			branch, _ := obj.Branch()
			glog.Errorf("%d: parentReleaseMilestone=%q but branch is %q", *obj.Issue.Number, parentMilestone, branch)
			return
		}
	}
	if milestone == "" {
		obj.SetMilestone(branchImpliedMilestone)
	}
	if !obj.HasLabel(cpApprovedLabel) {
		obj.AddLabel(cpApprovedLabel)
	}
}
