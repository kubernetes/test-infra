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
	"time"

	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
	"github.com/spf13/cobra"
)

const (
	day              = time.Hour * 24
	keepOpenLabel    = "keep-open"
	stalePullRequest = 90 * day // Close the PR if no human interaction for `stalePullRequest`
	startWarning     = 60 * day
	remindWarning    = 30 * day
	closingComment   = `This PR hasn't been active in %s. Feel free to reopen.

You can add 'keep-open' label to prevent this from happening again.`
	warningComment = `This PR hasn't been active in %s. Will be closed in %s.

You can add 'keep-open' label to prevent this from happening.`
)

var (
	closingCommentRE = regexp.MustCompile(`This PR hasn't been active in \d+ days?\. Feel free to reopen.

You can add 'keep-open' label to prevent this from happening again\.`)
	warningCommentRE = regexp.MustCompile(`This PR hasn't been active in \d+ days?\. Will be closed in \d+ days?\.

You can add 'keep-open' label to prevent this from happening\.`)
)

// CloseStalePR will ask the Bot to close any PullRequest that didn't
// have any human interactions in `stalePullRequest` duration.
//
// This is done by checking both review and issue comments, and by
// ignoring comments done with a bot name. We also consider re-open on the PR.
type CloseStalePR struct{}

func init() {
	s := CloseStalePR{}
	RegisterMungerOrDie(s)
	RegisterStaleComments(s)
}

// Name is the name usable in --pr-mungers
func (CloseStalePR) Name() string { return "close-stale-pr" }

// RequiredFeatures is a slice of 'features' that must be provided
func (CloseStalePR) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (CloseStalePR) Initialize(config *github.Config, features *features.Features) error {
	return nil
}

// EachLoop is called at the start of every munge loop
func (CloseStalePR) EachLoop() error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (CloseStalePR) AddFlags(cmd *cobra.Command, config *github.Config) {}

func findLastHumanPullRequestUpdate(obj *github.MungeObject) (*time.Time, error) {
	pr, err := obj.GetPR()
	if err != nil {
		return nil, err
	}

	comments, err := obj.ListReviewComments()
	if err != nil {
		return nil, err
	}

	lastHuman := pr.CreatedAt
	for i := range comments {
		comment := comments[i]
		if comment.User == nil || comment.User.Login == nil || comment.CreatedAt == nil || comment.Body == nil {
			continue
		}
		if *comment.User.Login == botName || *comment.User.Login == jenkinsBotName {
			continue
		}
		if lastHuman.Before(*comment.UpdatedAt) {
			lastHuman = comment.UpdatedAt
		}
	}

	return lastHuman, nil
}

func findLastHumanIssueUpdate(obj *github.MungeObject) (*time.Time, error) {
	lastHuman := obj.Issue.CreatedAt

	comments, err := obj.ListComments()
	if err != nil {
		return nil, err
	}

	for i := range comments {
		comment := comments[i]
		if !validComment(comment) {
			continue
		}
		if mergeBotComment(comment) || jenkinsBotComment(comment) {
			continue
		}
		if lastHuman.Before(*comment.UpdatedAt) {
			lastHuman = comment.UpdatedAt
		}
	}

	return lastHuman, nil
}

func findLastInterestingEventUpdate(obj *github.MungeObject) (*time.Time, error) {
	lastInteresting := obj.Issue.CreatedAt

	events, err := obj.GetEvents()
	if err != nil {
		return nil, err
	}

	for i := range events {
		event := events[i]
		if event.Event == nil || *event.Event != "reopened" {
			continue
		}

		if lastInteresting.Before(*event.CreatedAt) {
			lastInteresting = event.CreatedAt
		}
	}

	return lastInteresting, nil
}

func findLastModificationTime(obj *github.MungeObject) (*time.Time, error) {
	lastHumanIssue, err := findLastHumanIssueUpdate(obj)
	if err != nil {
		return nil, err
	}
	lastHumanPR, err := findLastHumanPullRequestUpdate(obj)
	if err != nil {
		return nil, err
	}
	lastInterestingEvent, err := findLastInterestingEventUpdate(obj)
	if err != nil {
		return nil, err
	}

	lastModif := lastHumanPR
	if lastHumanIssue.After(*lastModif) {
		lastModif = lastHumanIssue
	}
	if lastInterestingEvent.After(*lastModif) {
		lastModif = lastInterestingEvent
	}

	return lastModif, nil
}

func findLatestWarningComment(obj *github.MungeObject) *githubapi.IssueComment {
	var lastFoundComment *githubapi.IssueComment

	comments, err := obj.ListComments()
	if err != nil {
		return nil
	}

	for i := range comments {
		comment := comments[i]
		if !validComment(comment) {
			continue
		}
		if !mergeBotComment(comment) {
			continue
		}

		if !warningCommentRE.MatchString(*comment.Body) {
			continue
		}

		if lastFoundComment == nil || lastFoundComment.CreatedAt.Before(*comment.UpdatedAt) {
			if lastFoundComment != nil {
				obj.DeleteComment(lastFoundComment)
			}
			lastFoundComment = &comment
		}
	}

	return lastFoundComment
}

func durationToDays(duration time.Duration) string {
	days := duration / day
	dayString := "days"
	if days == 1 || days == -1 {
		dayString = "day"
	}
	return fmt.Sprintf("%d %s", days, dayString)
}

func closePullRequest(obj *github.MungeObject, inactiveFor time.Duration) {
	comment := findLatestWarningComment(obj)
	if comment != nil {
		obj.DeleteComment(comment)
	}

	obj.WriteComment(fmt.Sprintf(closingComment, durationToDays(inactiveFor)))
	obj.ClosePR()
}

func postWarningComment(obj *github.MungeObject, inactiveFor time.Duration, closeIn time.Duration) {
	obj.WriteComment(fmt.Sprintf(
		warningComment,
		durationToDays(inactiveFor),
		durationToDays(closeIn)))
}

func checkAndWarn(obj *github.MungeObject, inactiveFor time.Duration, closeIn time.Duration) {
	if closeIn < day {
		// We are going to close the PR in less than a day. Too late to warn
		return
	}
	comment := findLatestWarningComment(obj)
	if comment == nil {
		// We don't already have the comment. Post it
		postWarningComment(obj, inactiveFor, closeIn)
	} else if time.Since(*comment.UpdatedAt) > remindWarning {
		// It's time to warn again
		obj.DeleteComment(comment)
		postWarningComment(obj, inactiveFor, closeIn)
	} else {
		// We already have a warning, and it's not expired. Do nothing
	}
}

// Munge is the workhorse that will actually close the PRs
func (CloseStalePR) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	if obj.HasLabel(keepOpenLabel) {
		return
	}

	lastModif, err := findLastModificationTime(obj)
	if err != nil {
		glog.Errorf("Failed to find last modification: %v", err)
		return
	}

	closeIn := -time.Since(lastModif.Add(stalePullRequest))
	inactiveFor := time.Since(*lastModif)
	if closeIn <= 0 {
		closePullRequest(obj, inactiveFor)
	} else if closeIn <= startWarning {
		checkAndWarn(obj, inactiveFor, closeIn)
	} else {
		// Pull-request is active. Do nothing
	}
}

func (CloseStalePR) isStaleComment(obj *github.MungeObject, comment githubapi.IssueComment) bool {
	if !mergeBotComment(comment) {
		return false
	}

	if !closingCommentRE.MatchString(*comment.Body) {
		return false
	}

	return true
}

// StaleComments returns a slice of stale comments
func (s CloseStalePR) StaleComments(obj *github.MungeObject, comments []githubapi.IssueComment) []githubapi.IssueComment {
	return forEachCommentTest(obj, comments, s.isStaleComment)
}
