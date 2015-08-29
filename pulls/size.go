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

package pulls

import (
	"fmt"
	"strings"

	"k8s.io/contrib/mungegithub/opts"

	"github.com/golang/glog"
	"github.com/google/go-github/github"
)

type PRSizeMunger struct{}

func init() {
	RegisterMungerOrDie(PRSizeMunger{})
}

func (PRSizeMunger) Name() string { return "size" }

const labelSizePrefix = "size/"

func (PRSizeMunger) MungePullRequest(client *github.Client, pr *github.PullRequest, issue *github.Issue, commits []github.RepositoryCommit, events []github.IssueEvent, opts opts.MungeOptions) {
	if pr.Number == nil {
		glog.Warningf("PR has no Number: %+v", *pr)
		return
	}
	if pr.Additions == nil {
		glog.Warningf("PR %d has nil Additions", *pr.Number)
		return
	}
	if pr.Deletions == nil {
		glog.Warningf("PR %d has nil Deletions", *pr.Number)
		return
	}

	adds := *pr.Additions
	dels := *pr.Deletions

	for _, c := range commits {
		for _, f := range c.Files {
			if strings.HasPrefix(*f.Filename, "Godeps/") {
				adds = adds - *f.Additions
				dels = dels - *f.Deletions
				continue
			}
		}
	}

	newSize := calculateSize(adds, dels)
	newLabel := labelSizePrefix + newSize

	existing := GetLabelsWithPrefix(issue.Labels, labelSizePrefix)
	needsUpdate := true
	for _, l := range existing {
		if l == newLabel {
			needsUpdate = false
			continue
		}
		if opts.Dryrun {
			glog.Infof("PR #%d: would have removed label %s", *pr.Number, l)
		} else {
			if _, err := client.Issues.RemoveLabelForIssue(opts.Org, opts.Project, *pr.Number, l); err != nil {
				glog.Errorf("PR #%d: error removing label %q: %v", *pr.Number, l, err)
			}
		}
	}
	if needsUpdate {
		if opts.Dryrun {
			glog.Infof("PR #%d: would have added label %s", *pr.Number, newLabel)
		} else {
			if _, _, err := client.Issues.AddLabelsToIssue(opts.Org, opts.Project, *pr.Number, []string{newLabel}); err != nil {
				glog.Errorf("PR #%d: error adding label %q: %v", *pr.Number, newLabel, err)
			}
			body := fmt.Sprintf("Labelling this PR as %s", newLabel)
			if _, _, err := client.Issues.CreateComment(opts.Org, opts.Project, *pr.Number, &github.IssueComment{Body: &body}); err != nil {
				glog.Errorf("PR #%d: error adding comment: %v", *pr.Number, err)
			}
		}
	}
}

const (
	sizeXS  = "XS"
	sizeS   = "S"
	sizeM   = "M"
	sizeL   = "L"
	sizeXL  = "XL"
	sizeXXL = "XXL"
)

func calculateSize(adds, dels int) string {
	lines := adds + dels

	// This is a totally arbitrary heuristic and is open for tweaking.
	if lines < 10 {
		return sizeXS
	}
	if lines < 30 {
		return sizeS
	}
	if lines < 100 {
		return sizeM
	}
	if lines < 500 {
		return sizeL
	}
	if lines < 1000 {
		return sizeXL
	}
	return sizeXXL
}
