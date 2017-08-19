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

package lgtm

import (
	"fmt"
	"regexp"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

const pluginName = "lgtm"

var (
	lgtmLabel    = "lgtm"
	lgtmRe       = regexp.MustCompile(`(?mi)^/lgtm(?: no-issue)?\s*$`)
	lgtmCancelRe = regexp.MustCompile(`(?mi)^/lgtm cancel\s*$`)
)

type event struct {
	action        string
	org           string
	repo          string
	number        int
	prAuthor      string
	commentAuthor string
	body          string
	assignees     []github.User
	hasLabel      func(label string) (bool, error)
	htmlurl       string
}

func init() {
	plugins.RegisterIssueCommentHandler(pluginName, handleIssueComment)
	plugins.RegisterReviewEventHandler(pluginName, handleReview)
	plugins.RegisterReviewCommentEventHandler(pluginName, handleReviewComment)
}

type githubClient interface {
	IsMember(owner, login string) (bool, error)
	AddLabel(owner, repo string, number int, label string) error
	AssignIssue(owner, repo string, number int, assignees []string) error
	CreateComment(owner, repo string, number int, comment string) error
	RemoveLabel(owner, repo string, number int, label string) error
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
}

// prLabelChecker returns a function that lazily checks if a label is applied to a pr.
func prLabelChecker(gc githubClient, log *logrus.Entry, org, repo string, num int) func(string) (bool, error) {
	return func(label string) (bool, error) {
		labels, err := gc.GetIssueLabels(org, repo, num)
		if err != nil {
			return false, err
		}
		for _, candidate := range labels {
			if candidate.Name == label {
				return true, nil
			}
		}
		return false, nil
	}
}

func handleIssueComment(pc plugins.PluginClient, ic github.IssueCommentEvent) error {
	// Only consider open PRs.
	if !ic.Issue.IsPullRequest() || ic.Issue.State != "open" {
		return nil
	}

	e := &event{
		action:        ic.Action,
		org:           ic.Repo.Owner.Login,
		repo:          ic.Repo.Name,
		number:        ic.Issue.Number,
		prAuthor:      ic.Issue.User.Login,
		commentAuthor: ic.Comment.User.Login,
		body:          ic.Comment.Body,
		assignees:     ic.Issue.Assignees,
		hasLabel:      func(label string) (bool, error) { return ic.Issue.HasLabel(label), nil },
		htmlurl:       ic.Comment.HTMLURL,
	}
	return handle(pc.GitHubClient, pc.Logger, e)
}

func handleReview(pc plugins.PluginClient, re github.ReviewEvent) error {
	e := &event{
		action:        re.Action,
		org:           re.Repo.Owner.Login,
		repo:          re.Repo.Name,
		number:        re.PullRequest.Number,
		prAuthor:      re.PullRequest.User.Login,
		commentAuthor: re.Review.User.Login,
		body:          re.Review.Body,
		assignees:     re.PullRequest.Assignees,
		hasLabel: prLabelChecker(
			pc.GitHubClient,
			pc.Logger,
			re.Repo.Owner.Login,
			re.Repo.Name,
			re.PullRequest.Number,
		),
		htmlurl: re.Review.HTMLURL,
	}
	return handle(pc.GitHubClient, pc.Logger, e)
}

func handleReviewComment(pc plugins.PluginClient, rce github.ReviewCommentEvent) error {
	e := &event{
		action:        rce.Action,
		org:           rce.Repo.Owner.Login,
		repo:          rce.Repo.Name,
		number:        rce.PullRequest.Number,
		prAuthor:      rce.PullRequest.User.Login,
		commentAuthor: rce.Comment.User.Login,
		body:          rce.Comment.Body,
		assignees:     rce.PullRequest.Assignees,
		hasLabel: prLabelChecker(
			pc.GitHubClient,
			pc.Logger,
			rce.Repo.Owner.Login,
			rce.Repo.Name,
			rce.PullRequest.Number,
		),
		htmlurl: rce.Comment.HTMLURL,
	}
	return handle(pc.GitHubClient, pc.Logger, e)
}

func handle(gc githubClient, log *logrus.Entry, e *event) error {
	if e.action != "created" && e.action != "submitted" {
		return nil
	}

	// If we create an "/lgtm" comment, add lgtm if necessary.
	// If we create a "/lgtm cancel" comment, remove lgtm if necessary.
	wantLGTM := false
	if lgtmRe.MatchString(e.body) {
		wantLGTM = true
	} else if lgtmCancelRe.MatchString(e.body) {
		wantLGTM = false
	} else {
		return nil
	}

	// Allow authors to cancel LGTM. Do not allow authors to LGTM, and do not
	// accept commands from any other user.
	isAssignee := false
	for _, user := range e.assignees {
		if user.Login == e.commentAuthor {
			isAssignee = true
			break
		}
	}
	isAuthor := e.commentAuthor == e.prAuthor
	if isAuthor && wantLGTM {
		resp := "you cannot LGTM your own PR."
		log.Infof("Commenting with \"%s\".", resp)
		return gc.CreateComment(e.org, e.repo, e.number, plugins.FormatResponseRaw(e.body, e.htmlurl, e.commentAuthor, resp))
	} else if !isAuthor && !isAssignee {
		log.Infof("Assigning %s/%s#%d to %s", e.org, e.repo, e.number, e.commentAuthor)
		if err := gc.AssignIssue(e.org, e.repo, e.number, []string{e.commentAuthor}); err != nil {
			msg := "assigning you to the PR failed"
			if ok, merr := gc.IsMember(e.org, e.commentAuthor); merr == nil && !ok {
				msg = fmt.Sprintf("only %s org members may be assigned issues", e.org)
			} else if merr != nil {
				log.WithError(merr).Errorf("Failed IsMember(%s, %s)", e.org, e.commentAuthor)
			} else {
				log.WithError(err).Errorf("Failed AssignIssue(%s, %s, %d, %s)", e.org, e.repo, e.number, e.commentAuthor)
			}
			resp := "changing LGTM is restricted to assignees, and " + msg + "."
			log.Infof("Reply to assign via /lgtm request with comment: \"%s\"", resp)
			return gc.CreateComment(e.org, e.repo, e.number, plugins.FormatResponseRaw(e.body, e.htmlurl, e.commentAuthor, resp))
		}
	}

	// Only add the label if it doesn't have it, and vice versa.
	hasLGTM, err := e.hasLabel(lgtmLabel)
	if err != nil {
		return fmt.Errorf("failed to get the labels on %s/%s#%d: %v", e.org, e.repo, e.number, err)
	}
	if hasLGTM && !wantLGTM {
		log.Info("Removing LGTM label.")
		return gc.RemoveLabel(e.org, e.repo, e.number, lgtmLabel)
	} else if !hasLGTM && wantLGTM {
		log.Info("Adding LGTM label.")
		return gc.AddLabel(e.org, e.repo, e.number, lgtmLabel)
	}
	return nil
}
