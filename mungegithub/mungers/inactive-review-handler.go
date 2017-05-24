/*
Copyright 2017 The Kubernetes Authors.

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
	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
	"github.com/spf13/cobra"
	"k8s.io/test-infra/mungegithub/features"
	"k8s.io/test-infra/mungegithub/github"
	"k8s.io/test-infra/mungegithub/mungers/matchers"
	"time"
)

const (
	NOTIFNAME       = "INACTIVE-PULL-REQUEST"
	CREATIONTIMECAP = 36 * 30 * 24 * time.Hour //period since PR creation time
	COMMENTTIMECAP  = 7 * 24 * time.Hour       //period since last IssueComment and PullRequestComment being posted
	REMINDERNUMCAP  = 5                        //maximum number of times this munger will post reminder IssueComment
	LEAFOWNERSONLY  = false                    //setting for Blunderbuss to fetch only leaf owners or all owners
)

type InactiveReviewHandler struct {
	features *features.Features
}

func init() {
	h := &InactiveReviewHandler{}
	RegisterMungerOrDie(h)
}

func (i *InactiveReviewHandler) Name() string { return "inactive-review-handler" }

func (i *InactiveReviewHandler) RequiredFeatures() []string {
	return []string{features.RepoFeatureName, features.AliasesFeature}
}

func (i *InactiveReviewHandler) Initialize(config *github.Config, features *features.Features) error {
	i.features = features
	return nil
}

// EachLoop is called at the start of every munge loop
func (i *InactiveReviewHandler) EachLoop() error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (i *InactiveReviewHandler) AddFlags(cmd *cobra.Command, config *github.Config) {}

func (i *InactiveReviewHandler) haveNonAuthorHuman(authorName *string, comments []*githubapi.IssueComment, reviewComments []*githubapi.PullRequestComment) bool {
	return matchers.Items{}.
		AddComments(comments...).
		AddReviewComments(reviewComments...).
		Filter(matchers.HumanActor()).
		Filter(matchers.Not(matchers.AuthorLogin(*authorName))).
		Len() != 0
}

// Suggest a new reviewer who is NOT any of the existing reviewers
// (1) get all current assignees for the PR
// (2) get potential owners of the PR using Blunderbuss algorithm (calling getPotentialOwners() function)
// (3) filter out current assignees from the potential owners
// (4) if there is no any new reviewer available, the bot will encourage the PR author to ping all existing assignees
// (5) otherwise, select a new reviewer using Blunderbuss algorithm (calling selectMultipleOwners() function with number of assignees parameter of one)
// Note: the munger will suggest a new reviewer when the PR currently does not have any reviewer
func (i *InactiveReviewHandler) suggestNewReviewer(issue *githubapi.Issue, potentialOwners weightMap, weightSum int64) string {
	var newReviewer string

	if len(issue.Assignees) > 0 {
		for _, oldReviewer := range issue.Assignees {
			login := *oldReviewer.Login

			for potentialOwner := range potentialOwners {
				if login == potentialOwner {
					weightSum -= potentialOwners[login]
					delete(potentialOwners, login)
					break
				}
			}
		}
	}

	if len(potentialOwners) > 0 {
		newReviewer = selectMultipleOwners(potentialOwners, weightSum, 1)[0]
	}

	return newReviewer
}

// Munge is the workhorse encouraging PR author to assign a new reviewer
// after getting no response from current reviewer for "COMMENTTIMECAP" duration
// The algorithm:
// (1) find latest comment posting time
// (2) if the time is "COMMENTTIMECAP" or longer before today's time, create a comment
//     encouraging the author to assign a new reviewer and unassign the old reviewer
// (3) suggest the new reviewer using Blunderbuss algorithm, making sure the old reviewer is not suggested
// Note: the munger will post at most "REMINDERNUMCAP" number of times
func (i *InactiveReviewHandler) Munge(obj *github.MungeObject) {
	issue := obj.Issue

	// do not suggest new reviewer if it is not a PR, the PR has no author information, or
	// the PR has been created more than 3 years ago (36 months with 30 days per month)
	if !obj.IsPR() || issue.User == nil || issue.User.Login == nil ||
		time.Since(*issue.CreatedAt) > CREATIONTIMECAP {
		return
	}

	comments, ok := obj.ListComments()
	if !ok {
		return
	}

	reviewComments, ok := obj.ListReviewComments()
	if !ok {
		return
	}

	files, ok := obj.ListFiles()
	if !ok || len(files) == 0 {
		glog.Errorf("failed to detect any changed file when assigning a new reviewer for inactive PR #%v", *obj.Issue.Number)
		return
	}

	// return if there is no non-author human
	if !i.haveNonAuthorHuman(issue.User.Login, comments, reviewComments) {
		return
	}

	// return if the munger has created comments for "REMINDERNUMCAP" number of times
	pinger := matchers.NewPinger(NOTIFNAME).SetTimePeriod(COMMENTTIMECAP).SetMaxCount(REMINDERNUMCAP)
	if pinger.IsMaxReached(comments, issue.CreatedAt) {
		return
	}

	// only run Blunderbuss algorithm when ping limit is not reached
	potentialOwners, weightSum := getPotentialOwners(*issue.User.Login, i.features, files, LEAFOWNERSONLY)
	newReviewer := i.suggestNewReviewer(issue, potentialOwners, weightSum)
	var msg string

	if len(issue.Assignees) == 0 {
		msg = fmt.Sprintf("To expedite a review, consider assigning _%s_.", newReviewer)
	} else if len(newReviewer) == 0 {
		msg = fmt.Sprintf("Sorry the review process for your PR has stalled. Your reviewer(s) may be on vacation or otherwise occupied. Consider pinging them.")
	} else {
		msg = fmt.Sprintf("Sorry the review process for your PR has stalled. Your reviewer(s) may be on vacation or otherwise occupied. Consider unassigning them using `/unassign` command, and assigning _%s_.", newReviewer)
	}

	// return if the munger has created the comment within "COMMENTTIMECAP", or
	// the PR is created within "CREATIONTIMECAP"
	if pinger.PingNotification(comments, msg, issue.CreatedAt) == nil {
		return
	}

	if err := obj.WriteComment(msg); err != nil {
		glog.Errorf("failed to leave comment encouraging %s to assign a new reviewer for inactive PR #%v", *issue.User.Login, *issue.Number)
	}
}
