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
	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

// AssignFixesMunger will assign issues to users based on the config file
// provided by --assignfixes-config.
type AssignFixesMunger struct {
	config              *github.Config
	features            *features.Features
	assignfixesReassign bool
}

func init() {
	assignfixes := &AssignFixesMunger{}
	RegisterMungerOrDie(assignfixes)
}

// Name is the name usable in --pr-mungers
func (a *AssignFixesMunger) Name() string { return "assign-fixes" }

// RequiredFeatures is a slice of 'features' that must be provided
func (a *AssignFixesMunger) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (a *AssignFixesMunger) Initialize(config *github.Config, features *features.Features) error {
	glog.Infof("fixes-issue-reassign: %#v\n", a.assignfixesReassign)

	a.features = features
	a.config = config
	return nil
}

// EachLoop is called at the start of every munge loop
func (a *AssignFixesMunger) EachLoop() error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (a *AssignFixesMunger) AddFlags(cmd *cobra.Command, config *github.Config) {
	cmd.Flags().BoolVar(&a.assignfixesReassign, "fixes-issue-reassign", false, "Assign fixes Issues even if they're already assigned")
}

// Munge is the workhorse the will actually make updates to the PR
func (a *AssignFixesMunger) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}
	// we need the PR for the "User" (creator of the PR not the assignee)
	pr, err := obj.GetPR()
	if err != nil {
		glog.Infof("Couldn't get PR %v", obj.Issue.Number)
		return
	}
	prOwner := github.DescribeUser(pr.User)

	issuesFixed := obj.GetPRFixesList()
	if issuesFixed == nil {
		return
	}
	for _, fixesNum := range issuesFixed {
		// "issue" is the issue referenced by the "fixes #<num>"
		issueObj, err := a.config.GetObject(fixesNum)
		if err != nil {
			glog.Infof("Couldn't get issue %v", fixesNum)
			continue
		}
		issue := issueObj.Issue
		if !a.assignfixesReassign && issue.Assignee != nil {
			glog.V(6).Infof("skipping %v: reassign: %v assignee: %v", *issue.Number, a.assignfixesReassign, github.DescribeUser(issue.Assignee))
			continue
		}
		glog.Infof("Assigning %v to %v (previously assigned to %v)", *issue.Number, prOwner, github.DescribeUser(issue.Assignee))
		// although it says "AssignPR" it's more generic than that and is really just an issue.
		issueObj.AssignPR(prOwner)
	}

}
