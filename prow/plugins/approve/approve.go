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

package approve

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/approve/approvers"
	"k8s.io/test-infra/prow/repoowners"
)

const (
	// PluginName defines this plugin's registered name.
	PluginName = "approve"

	approveCommand  = "APPROVE"
	cancelArgument  = "cancel"
	noIssueArgument = "no-issue"
)

var (
	associatedIssueRegexFormat = `(?:%s/[^/]+/issues/|#)(\d+)`
	commandRegex               = regexp.MustCompile(`(?m)^/([^\s]+)[\t ]*([^\n\r]*)`)
	notificationRegex          = regexp.MustCompile(`(?is)^\[` + approvers.ApprovalNotificationName + `\] *?([^\n]*)(?:\n\n(.*))?`)

	// handleFunc is used to allow mocking out the behavior of 'handle' while testing.
	handleFunc = handle
)

type githubClient interface {
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
	ListIssueComments(org, repo string, number int) ([]github.IssueComment, error)
	ListReviews(org, repo string, number int) ([]github.Review, error)
	ListPullRequestComments(org, repo string, number int) ([]github.ReviewComment, error)
	DeleteComment(org, repo string, ID int) error
	CreateComment(org, repo string, number int, comment string) error
	BotName() (string, error)
	AddLabel(org, repo string, number int, label string) error
	RemoveLabel(org, repo string, number int, label string) error
	ListIssueEvents(org, repo string, num int) ([]github.ListedIssueEvent, error)
}

type ownersClient interface {
	LoadRepoOwners(org, repo, base string) (repoowners.RepoOwner, error)
}

type state struct {
	org    string
	repo   string
	branch string
	number int

	body      string
	author    string
	assignees []github.User
	htmlURL   string
}

func init() {
	plugins.RegisterGenericCommentHandler(PluginName, handleGenericCommentEvent, helpProvider)
	plugins.RegisterReviewEventHandler(PluginName, handleReviewEvent, helpProvider)
	plugins.RegisterPullRequestHandler(PluginName, handlePullRequestEvent, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	doNot := func(b bool) string {
		if b {
			return ""
		}
		return "do not "
	}
	willNot := func(b bool) string {
		if b {
			return "will "
		}
		return "will not "
	}

	approveConfig := map[string]string{}
	for _, repo := range enabledRepos {
		opts := config.ApproveFor(repo.Org, repo.Repo)
		approveConfig[repo.String()] = fmt.Sprintf("Pull requests %s require an associated issue.<br>Pull request authors %s implicitly approve their own PRs.<br>A GitHub approved or changes requested review %s act as approval or cancel respectively.", doNot(opts.IssueRequired), doNot(opts.HasSelfApproval()), willNot(opts.ConsiderReviewState()))
	}

	yamlSnippet, err := plugins.CommentMap.GenYaml(&plugins.Configuration{
		Approve: []plugins.Approve{
			{
				Repos: []string{
					"ORGANIZATION",
					"ORGANIZATION/REPOSITORY",
				},
				RequireSelfApproval: new(bool),
				IgnoreReviewState:   new(bool),
			},
		},
	})
	if err != nil {
		logrus.WithError(err).Warnf("cannot generate comments for %s plugin", PluginName)
	}

	pluginHelp := &pluginhelp.PluginHelp{
		Description: `The approve plugin implements a pull request approval process that manages the '` + labels.Approved + `' label and an approval notification comment. Approval is achieved when the set of users that have approved the PR is capable of approving every file changed by the PR. A user is able to approve a file if their username or an alias they belong to is listed in the 'approvers' section of an OWNERS file in the directory of the file or higher in the directory tree.
<br>
<br>Per-repo configuration may be used to require that PRs link to an associated issue before approval is granted. It may also be used to specify that the PR authors implicitly approve their own PRs.
<br>For more information see <a href="https://git.k8s.io/test-infra/prow/plugins/approve/approvers/README.md">here</a>.`,
		Config:  approveConfig,
		Snippet: yamlSnippet,
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/approve [no-issue|cancel]",
		Description: "Approves a pull request",
		Featured:    true,
		WhoCanUse:   "Users listed as 'approvers' in appropriate OWNERS files.",
		Examples:    []string{"/approve", "/approve no-issue"},
	})
	return pluginHelp, nil
}

func handleGenericCommentEvent(pc plugins.Agent, ce github.GenericCommentEvent) error {
	return handleGenericComment(
		pc.Logger,
		pc.GitHubClient,
		pc.OwnersClient,
		pc.Config.GitHubOptions,
		pc.PluginConfig,
		&ce,
	)
}

func handleGenericComment(log *logrus.Entry, ghc githubClient, oc ownersClient, githubConfig config.GitHubOptions, config *plugins.Configuration, ce *github.GenericCommentEvent) error {
	funcStart := time.Now()
	defer func() {
		log.WithField("duration", time.Since(funcStart).String()).Debug("Completed handleGenericComment")
	}()
	if ce.Action != github.GenericCommentActionCreated || !ce.IsPR || ce.IssueState == "closed" {
		log.Debug("Event is not a creation of a comment on an open PR, skipping.")
		return nil
	}

	botName, err := ghc.BotName()
	if err != nil {
		return err
	}

	opts := config.ApproveFor(ce.Repo.Owner.Login, ce.Repo.Name)
	if !isApprovalCommand(botName, &comment{Body: ce.Body, Author: ce.User.Login}) {
		log.Debug("Comment does not constitute approval, skipping event.")
		return nil
	}

	log.Debug("Resolving pull request...")
	pr, err := ghc.GetPullRequest(ce.Repo.Owner.Login, ce.Repo.Name, ce.Number)
	if err != nil {
		return err
	}

	log.Debug("Resolving repository owners...")
	repo, err := oc.LoadRepoOwners(ce.Repo.Owner.Login, ce.Repo.Name, pr.Base.Ref)
	if err != nil {
		return err
	}

	return handleFunc(
		log,
		ghc,
		repo,
		githubConfig,
		opts,
		&state{
			org:       ce.Repo.Owner.Login,
			repo:      ce.Repo.Name,
			branch:    pr.Base.Ref,
			number:    ce.Number,
			body:      ce.IssueBody,
			author:    ce.IssueAuthor.Login,
			assignees: ce.Assignees,
			htmlURL:   ce.IssueHTMLURL,
		},
	)
}

// handleReviewEvent should only handle reviews that have no approval command.
// Reviews with approval commands will be handled by handleGenericCommentEvent.
func handleReviewEvent(pc plugins.Agent, re github.ReviewEvent) error {
	return handleReview(
		pc.Logger,
		pc.GitHubClient,
		pc.OwnersClient,
		pc.Config.GitHubOptions,
		pc.PluginConfig,
		&re,
	)
}

func handleReview(log *logrus.Entry, ghc githubClient, oc ownersClient, githubConfig config.GitHubOptions, config *plugins.Configuration, re *github.ReviewEvent) error {
	funcStart := time.Now()
	defer func() {
		log.WithField("duration", time.Since(funcStart).String()).Debug("Completed handleReview")
	}()
	if re.Action != github.ReviewActionSubmitted && re.Action != github.ReviewActionDismissed {
		log.Debug("Event is not a creation or dismissal of a review on an open PR, skipping.")
		return nil
	}

	botName, err := ghc.BotName()
	if err != nil {
		return err
	}

	opts := config.ApproveFor(re.Repo.Owner.Login, re.Repo.Name)

	// Check for an approval command is in the body. If one exists, let the
	// genericCommentEventHandler handle this event. Approval commands override
	// review state.
	if isApprovalCommand(botName, &comment{Body: re.Review.Body, Author: re.Review.User.Login}) {
		log.Debug("Review constitutes approval, skipping event.")
		return nil
	}

	// Check for an approval command via review state. If none exists, don't
	// handle this event.
	if !isApprovalState(botName, opts.ConsiderReviewState(), &comment{Author: re.Review.User.Login, ReviewState: re.Review.State}) {
		log.Debug("Review does not constitute approval, skipping event.")
		return nil
	}

	log.Debug("Resolving repository owners...")
	repo, err := oc.LoadRepoOwners(re.Repo.Owner.Login, re.Repo.Name, re.PullRequest.Base.Ref)
	if err != nil {
		return err
	}

	return handleFunc(
		log,
		ghc,
		repo,
		githubConfig,
		opts,
		&state{
			org:       re.Repo.Owner.Login,
			repo:      re.Repo.Name,
			branch:    re.PullRequest.Base.Ref,
			number:    re.PullRequest.Number,
			body:      re.PullRequest.Body,
			author:    re.PullRequest.User.Login,
			assignees: re.PullRequest.Assignees,
			htmlURL:   re.PullRequest.HTMLURL,
		},
	)

}

func handlePullRequestEvent(pc plugins.Agent, pre github.PullRequestEvent) error {
	return handlePullRequest(
		pc.Logger,
		pc.GitHubClient,
		pc.OwnersClient,
		pc.Config.GitHubOptions,
		pc.PluginConfig,
		&pre,
	)
}

func handlePullRequest(log *logrus.Entry, ghc githubClient, oc ownersClient, githubConfig config.GitHubOptions, config *plugins.Configuration, pre *github.PullRequestEvent) error {
	funcStart := time.Now()
	defer func() {
		log.WithField("duration", time.Since(funcStart).String()).Debug("Completed handlePullRequest")
	}()
	if pre.Action != github.PullRequestActionOpened &&
		pre.Action != github.PullRequestActionReopened &&
		pre.Action != github.PullRequestActionSynchronize &&
		pre.Action != github.PullRequestActionLabeled {
		log.Debug("Pull request event action cannot constitute approval, skipping...")
		return nil
	}
	botName, err := ghc.BotName()
	if err != nil {
		return err
	}
	if pre.Action == github.PullRequestActionLabeled &&
		(pre.Label.Name != labels.Approved || pre.Sender.Login == botName || pre.PullRequest.State == "closed") {
		log.Debug("Pull request label event does not constitute approval, skipping...")
		return nil
	}

	log.Debug("Resolving repository owners...")
	repo, err := oc.LoadRepoOwners(pre.Repo.Owner.Login, pre.Repo.Name, pre.PullRequest.Base.Ref)
	if err != nil {
		return err
	}

	return handleFunc(
		log,
		ghc,
		repo,
		githubConfig,
		config.ApproveFor(pre.Repo.Owner.Login, pre.Repo.Name),
		&state{
			org:       pre.Repo.Owner.Login,
			repo:      pre.Repo.Name,
			branch:    pre.PullRequest.Base.Ref,
			number:    pre.Number,
			body:      pre.PullRequest.Body,
			author:    pre.PullRequest.User.Login,
			assignees: pre.PullRequest.Assignees,
			htmlURL:   pre.PullRequest.HTMLURL,
		},
	)
}

// Returns associated issue, or 0 if it can't find any.
// This is really simple, and could be improved later.
func findAssociatedIssue(body, org string) (int, error) {
	associatedIssueRegex, err := regexp.Compile(fmt.Sprintf(associatedIssueRegexFormat, org))
	if err != nil {
		return 0, err
	}
	match := associatedIssueRegex.FindStringSubmatch(body)
	if len(match) == 0 {
		return 0, nil
	}
	v, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, err
	}
	return v, nil
}

// handle is the workhorse the will actually make updates to the PR.
// The algorithm goes as:
// - Initially, we build an approverSet
//   - Go through all comments in order of creation.
//     - (Issue/PR comments, PR review comments, and PR review bodies are considered as comments)
//   - If anyone said "/approve", add them to approverSet.
//   - If anyone created an approved review AND ReviewActsAsApprove is enabled, add them to approverSet.
// - Then, for each file, we see if any approver of this file is in approverSet and keep track of files without approval
//   - An approver of a file is defined as:
//     - Someone listed as an "approver" in an OWNERS file in the files directory OR
//     - in one of the file's parent directories
// - Iff all files have been approved, the bot will add the "approved" label.
// - Iff a cancel command is found, that reviewer will be removed from the approverSet
// 	and the munger will remove the approved label if it has been applied
func handle(log *logrus.Entry, ghc githubClient, repo approvers.Repo, githubConfig config.GitHubOptions, opts *plugins.Approve, pr *state) error {
	funcStart := time.Now()
	defer func() {
		log.WithField("duration", time.Since(funcStart).String()).Debug("Completed handle")
	}()
	fetchErr := func(context string, err error) error {
		return fmt.Errorf("failed to get %s for %s/%s#%d: %v", context, pr.org, pr.repo, pr.number, err)
	}

	start := time.Now()
	changes, err := ghc.GetPullRequestChanges(pr.org, pr.repo, pr.number)
	if err != nil {
		return fetchErr("PR file changes", err)
	}
	var filenames []string
	for _, change := range changes {
		filenames = append(filenames, change.Filename)
	}
	issueLabels, err := ghc.GetIssueLabels(pr.org, pr.repo, pr.number)
	if err != nil {
		return fetchErr("issue labels", err)
	}
	hasApprovedLabel := false
	for _, label := range issueLabels {
		if label.Name == labels.Approved {
			hasApprovedLabel = true
			break
		}
	}
	botName, err := ghc.BotName()
	if err != nil {
		return fetchErr("bot name", err)
	}
	issueComments, err := ghc.ListIssueComments(pr.org, pr.repo, pr.number)
	if err != nil {
		return fetchErr("issue comments", err)
	}
	reviewComments, err := ghc.ListPullRequestComments(pr.org, pr.repo, pr.number)
	if err != nil {
		return fetchErr("review comments", err)
	}
	reviews, err := ghc.ListReviews(pr.org, pr.repo, pr.number)
	if err != nil {
		return fetchErr("reviews", err)
	}
	log.WithField("duration", time.Since(start).String()).Debug("Completed github functions in handle")

	start = time.Now()
	approversHandler := approvers.NewApprovers(
		approvers.NewOwners(
			log,
			filenames,
			repo,
			int64(pr.number),
		),
	)
	approversHandler.AssociatedIssue, err = findAssociatedIssue(pr.body, pr.org)
	if err != nil {
		log.WithError(err).Errorf("Failed to find associated issue from PR body: %v", err)
	}
	approversHandler.RequireIssue = opts.IssueRequired
	approversHandler.ManuallyApproved = humanAddedApproved(ghc, log, pr.org, pr.repo, pr.number, botName, hasApprovedLabel)

	// Author implicitly approves their own PR if config allows it
	if opts.HasSelfApproval() {
		approversHandler.AddAuthorSelfApprover(pr.author, pr.htmlURL+"#", false)
	} else {
		// Treat the author as an assignee, and suggest them if possible
		approversHandler.AddAssignees(pr.author)
	}
	log.WithField("duration", time.Since(start).String()).Debug("Completed configuring approversHandler in handle")

	start = time.Now()
	commentsFromIssueComments := commentsFromIssueComments(issueComments)
	comments := append(commentsFromReviewComments(reviewComments), commentsFromIssueComments...)
	comments = append(comments, commentsFromReviews(reviews)...)
	sort.SliceStable(comments, func(i, j int) bool {
		return comments[i].CreatedAt.Before(comments[j].CreatedAt)
	})
	approveComments := filterComments(comments, approvalMatcher(botName, opts.ConsiderReviewState()))
	addApprovers(&approversHandler, approveComments, pr.author, opts.ConsiderReviewState())
	log.WithField("duration", time.Since(start).String()).Debug("Completed filtering approval comments in handle")

	for _, user := range pr.assignees {
		approversHandler.AddAssignees(user.Login)
	}

	start = time.Now()
	notifications := filterComments(commentsFromIssueComments, notificationMatcher(botName))
	latestNotification := getLast(notifications)
	newMessage := updateNotification(githubConfig.LinkURL, opts.CommandHelpLink, opts.PrProcessLink, pr.org, pr.repo, pr.branch, latestNotification, approversHandler)
	log.WithField("duration", time.Since(start).String()).Debug("Completed getting notifications in handle")
	start = time.Now()
	if newMessage != nil {
		for _, notif := range notifications {
			if err := ghc.DeleteComment(pr.org, pr.repo, notif.ID); err != nil {
				log.WithError(err).Errorf("Failed to delete comment from %s/%s#%d, ID: %d.", pr.org, pr.repo, pr.number, notif.ID)
			}
		}
		if err := ghc.CreateComment(pr.org, pr.repo, pr.number, *newMessage); err != nil {
			log.WithError(err).Errorf("Failed to create comment on %s/%s#%d: %q.", pr.org, pr.repo, pr.number, *newMessage)
		}
	}
	log.WithField("duration", time.Since(start).String()).Debug("Completed adding/deleting approval comments in handle")

	start = time.Now()
	if !approversHandler.IsApproved() {
		if hasApprovedLabel {
			if err := ghc.RemoveLabel(pr.org, pr.repo, pr.number, labels.Approved); err != nil {
				log.WithError(err).Errorf("Failed to remove %q label from %s/%s#%d.", labels.Approved, pr.org, pr.repo, pr.number)
			}
		}
	} else if !hasApprovedLabel {
		if err := ghc.AddLabel(pr.org, pr.repo, pr.number, labels.Approved); err != nil {
			log.WithError(err).Errorf("Failed to add %q label to %s/%s#%d.", labels.Approved, pr.org, pr.repo, pr.number)
		}
	}
	log.WithField("duration", time.Since(start).String()).Debug("Completed adding/deleting approval labels in handle")
	return nil
}

func humanAddedApproved(ghc githubClient, log *logrus.Entry, org, repo string, number int, botName string, hasLabel bool) func() bool {
	findOut := func() bool {
		if !hasLabel {
			return false
		}
		events, err := ghc.ListIssueEvents(org, repo, number)
		if err != nil {
			log.WithError(err).Errorf("Failed to list issue events for %s/%s#%d.", org, repo, number)
			return false
		}
		var lastAdded github.ListedIssueEvent
		for _, event := range events {
			// Only consider "approved" label added events.
			if event.Event != github.IssueActionLabeled || event.Label.Name != labels.Approved {
				continue
			}
			lastAdded = event
		}

		if lastAdded.Actor.Login == "" || lastAdded.Actor.Login == botName {
			return false
		}
		return true
	}

	var cache *bool
	return func() bool {
		if cache == nil {
			val := findOut()
			cache = &val
		}
		return *cache
	}
}

func approvalMatcher(botName string, reviewActsAsApprove bool) func(*comment) bool {
	return func(c *comment) bool {
		return isApprovalCommand(botName, c) || isApprovalState(botName, reviewActsAsApprove, c)
	}
}

func isApprovalCommand(botName string, c *comment) bool {
	if c.Author == botName {
		return false
	}

	for _, match := range commandRegex.FindAllStringSubmatch(c.Body, -1) {
		cmd := strings.ToUpper(match[1])
		if cmd == approveCommand {
			return true
		}
	}
	return false
}

func isApprovalState(botName string, reviewActsAsApprove bool, c *comment) bool {
	if c.Author == botName {
		return false
	}

	// The review webhook returns state as lowercase, while the review API
	// returns state as uppercase. Uppercase the value here so it always
	// matches the constant.
	reviewState := github.ReviewState(strings.ToUpper(string(c.ReviewState)))

	// ReviewStateApproved = /approve
	// ReviewStateChangesRequested = /approve cancel
	// ReviewStateDismissed = remove previous approval or disapproval
	// (Reviews can go from Approved or ChangesRequested to Dismissed
	// state if the Dismiss action is used)
	if reviewActsAsApprove && (reviewState == github.ReviewStateApproved ||
		reviewState == github.ReviewStateChangesRequested ||
		reviewState == github.ReviewStateDismissed) {
		return true
	}
	return false
}

func notificationMatcher(botName string) func(*comment) bool {
	return func(c *comment) bool {
		if c.Author != botName {
			return false
		}
		match := notificationRegex.FindStringSubmatch(c.Body)
		return len(match) > 0
	}
}

func updateNotification(linkURL *url.URL, commandHelpLink, prProcessLink, org, repo, branch string, latestNotification *comment, approversHandler approvers.Approvers) *string {
	message := approvers.GetMessage(approversHandler, linkURL, commandHelpLink, prProcessLink, org, repo, branch)
	if message == nil || (latestNotification != nil && strings.Contains(latestNotification.Body, *message)) {
		return nil
	}
	return message
}

// addApprovers iterates through the list of comments on a PR
// and identifies all of the people that have said /approve and adds
// them to the Approvers.  The function uses the latest approve or cancel comment
// to determine the Users intention. A review in requested changes state is
// considered a cancel.
func addApprovers(approversHandler *approvers.Approvers, approveComments []*comment, author string, reviewActsAsApprove bool) {
	for _, c := range approveComments {
		if c.Author == "" {
			continue
		}

		if reviewActsAsApprove && c.ReviewState == github.ReviewStateApproved {
			approversHandler.AddApprover(
				c.Author,
				c.HTMLURL,
				false,
			)
		}
		if reviewActsAsApprove && c.ReviewState == github.ReviewStateChangesRequested {
			approversHandler.RemoveApprover(c.Author)
		}

		for _, match := range commandRegex.FindAllStringSubmatch(c.Body, -1) {
			name := strings.ToUpper(match[1])
			if name != approveCommand {
				continue
			}
			args := strings.ToLower(strings.TrimSpace(match[2]))
			if strings.Contains(args, cancelArgument) {
				approversHandler.RemoveApprover(c.Author)
				continue
			}

			if c.Author == author {
				approversHandler.AddAuthorSelfApprover(
					c.Author,
					c.HTMLURL,
					args == noIssueArgument,
				)
			} else {
				approversHandler.AddApprover(
					c.Author,
					c.HTMLURL,
					args == noIssueArgument,
				)
			}
		}
	}
}

type comment struct {
	Body        string
	Author      string
	CreatedAt   time.Time
	HTMLURL     string
	ID          int
	ReviewState github.ReviewState
}

func commentFromIssueComment(ic *github.IssueComment) *comment {
	if ic == nil {
		return nil
	}
	return &comment{
		Body:      ic.Body,
		Author:    ic.User.Login,
		CreatedAt: ic.CreatedAt,
		HTMLURL:   ic.HTMLURL,
		ID:        ic.ID,
	}
}

func commentsFromIssueComments(ics []github.IssueComment) []*comment {
	comments := make([]*comment, 0, len(ics))
	for i := range ics {
		comments = append(comments, commentFromIssueComment(&ics[i]))
	}
	return comments
}

func commentFromReviewComment(rc *github.ReviewComment) *comment {
	if rc == nil {
		return nil
	}
	return &comment{
		Body:      rc.Body,
		Author:    rc.User.Login,
		CreatedAt: rc.CreatedAt,
		HTMLURL:   rc.HTMLURL,
		ID:        rc.ID,
	}
}

func commentsFromReviewComments(rcs []github.ReviewComment) []*comment {
	comments := make([]*comment, 0, len(rcs))
	for i := range rcs {
		comments = append(comments, commentFromReviewComment(&rcs[i]))
	}
	return comments
}

func commentFromReview(review *github.Review) *comment {
	if review == nil {
		return nil
	}
	return &comment{
		Body:        review.Body,
		Author:      review.User.Login,
		CreatedAt:   review.SubmittedAt,
		HTMLURL:     review.HTMLURL,
		ID:          review.ID,
		ReviewState: review.State,
	}
}

func commentsFromReviews(reviews []github.Review) []*comment {
	comments := make([]*comment, 0, len(reviews))
	for i := range reviews {
		comments = append(comments, commentFromReview(&reviews[i]))
	}
	return comments
}

func filterComments(comments []*comment, filter func(*comment) bool) []*comment {
	filtered := make([]*comment, 0, len(comments))
	for _, c := range comments {
		if filter(c) {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

func getLast(cs []*comment) *comment {
	if len(cs) == 0 {
		return nil
	}
	return cs[len(cs)-1]
}
