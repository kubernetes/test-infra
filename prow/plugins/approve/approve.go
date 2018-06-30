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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/approve/approvers"
	"k8s.io/test-infra/prow/repoowners"
)

const (
	pluginName = "approve"

	approveCommand  = "APPROVE"
	approvedLabel   = "approved"
	cancelArgument  = "cancel"
	lgtmCommand     = "LGTM"
	noIssueArgument = "no-issue"
)

var (
	associatedIssueRegex = regexp.MustCompile(`(?:kubernetes/[^/]+/issues/|#)(\d+)`)
	commandRegex         = regexp.MustCompile(`(?m)^/([^\s]+)[\t ]*([^\n\r]*)`)
	notificationRegex    = regexp.MustCompile(`(?is)^\[` + approvers.ApprovalNotificationName + `\] *?([^\n]*)(?:\n\n(.*))?`)

	// deprecatedBotNames are the names of the bots that previously handled approvals.
	// Each can be removed once every PR approved by the old bot has been merged or unapproved.
	deprecatedBotNames = []string{"k8s-merge-robot", "openshift-merge-robot"}

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
	LoadRepoOwners(org, repo, base string) (repoowners.RepoOwnerInterface, error)
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

	repoOptions *plugins.Approve
}

func init() {
	plugins.RegisterGenericCommentHandler(pluginName, handleGenericCommentEvent, helpProvider)
	plugins.RegisterReviewEventHandler(pluginName, handleReviewEvent, helpProvider)
	plugins.RegisterPullRequestHandler(pluginName, handlePullRequestEvent, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
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
		parts := strings.Split(repo, "/")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid repo in enabledRepos: %q", repo)
		}
		opts := optionsForRepo(config, parts[0], parts[1])
		approveConfig[repo] = fmt.Sprintf("Pull requests %s require an associated issue.<br>Pull request authors %s implicitly approve their own PRs.<br>The /lgtm [cancel] command(s) %s act as approval.<br>A GitHub approved or changes requested review %s act as approval or cancel respectively.", doNot(opts.IssueRequired), doNot(opts.ImplicitSelfApprove), willNot(opts.LgtmActsAsApprove), willNot(opts.ReviewActsAsApprove))
	}
	pluginHelp := &pluginhelp.PluginHelp{
		Description: `The approve plugin implements a pull request approval process that manages the '` + approvedLabel + `' label and an approval notification comment. Approval is achieved when the set of users that have approved the PR is capable of approving every file changed by the PR. A user is able to approve a file if their username or an alias they belong to is listed in the 'approvers' section of an OWNERS file in the directory of the file or higher in the directory tree.
<br>
<br>Per-repo configuration may be used to require that PRs link to an associated issue before approval is granted. It may also be used to specify that the PR authors implicitly approve their own PRs.
<br>For more information see <a href="https://git.k8s.io/test-infra/prow/plugins/approve/approvers/README.md">here</a>.`,
		Config: approveConfig,
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

func handleGenericCommentEvent(pc plugins.PluginClient, ce github.GenericCommentEvent) error {
	return handleGenericComment(
		pc.Logger,
		pc.GitHubClient,
		pc.OwnersClient,
		pc.PluginConfig,
		&ce,
	)
}

func handleGenericComment(log *logrus.Entry, ghc githubClient, oc ownersClient, config *plugins.Configuration, ce *github.GenericCommentEvent) error {
	if ce.Action != github.GenericCommentActionCreated || !ce.IsPR || ce.IssueState == "closed" {
		return nil
	}

	botName, err := ghc.BotName()
	if err != nil {
		return err
	}

	opts := optionsForRepo(config, ce.Repo.Owner.Login, ce.Repo.Name)
	if !isApprovalCommand(botName, opts.LgtmActsAsApprove, &comment{Body: ce.Body, Author: ce.User.Login}) {
		return nil
	}

	pr, err := ghc.GetPullRequest(ce.Repo.Owner.Login, ce.Repo.Name, ce.Number)
	if err != nil {
		return err
	}

	repo, err := oc.LoadRepoOwners(ce.Repo.Owner.Login, ce.Repo.Name, pr.Base.Ref)
	if err != nil {
		return err
	}

	return handleFunc(
		log,
		ghc,
		repo,
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
func handleReviewEvent(pc plugins.PluginClient, re github.ReviewEvent) error {
	return handleReview(
		pc.Logger,
		pc.GitHubClient,
		pc.OwnersClient,
		pc.PluginConfig,
		&re,
	)
}

func handleReview(log *logrus.Entry, ghc githubClient, oc ownersClient, config *plugins.Configuration, re *github.ReviewEvent) error {
	if re.Action != github.ReviewActionSubmitted {
		return nil
	}

	botName, err := ghc.BotName()
	if err != nil {
		return err
	}

	opts := optionsForRepo(config, re.Repo.Owner.Login, re.Repo.Name)

	// Check for an approval command is in the body. If one exists, let the
	// genericCommentEventHandler handle this event. Approval commands override
	// review state.
	if isApprovalCommand(botName, opts.LgtmActsAsApprove, &comment{Body: re.Review.Body, Author: re.Review.User.Login}) {
		return nil
	}

	// Check for an approval command via review state. If none exists, don't
	// handle this event.
	if !isApprovalState(botName, opts.ReviewActsAsApprove, &comment{Author: re.Review.User.Login, ReviewState: re.Review.State}) {
		return nil
	}

	// This is a valid review state command. Get the pull request and handle it.
	pr, err := ghc.GetPullRequest(re.Repo.Owner.Login, re.Repo.Name, re.PullRequest.Number)
	if err != nil {
		log.Error(err)
		return err
	}

	repo, err := oc.LoadRepoOwners(re.Repo.Owner.Login, re.Repo.Name, pr.Base.Ref)
	if err != nil {
		return err
	}

	return handleFunc(
		log,
		ghc,
		repo,
		optionsForRepo(config, re.Repo.Owner.Login, re.Repo.Name),
		&state{
			org:       re.Repo.Owner.Login,
			repo:      re.Repo.Name,
			number:    re.PullRequest.Number,
			body:      re.Review.Body,
			author:    re.Review.User.Login,
			assignees: re.PullRequest.Assignees,
			htmlURL:   re.PullRequest.HTMLURL,
		},
	)

}

func handlePullRequestEvent(pc plugins.PluginClient, pre github.PullRequestEvent) error {
	return handlePullRequest(
		pc.Logger,
		pc.GitHubClient,
		pc.OwnersClient,
		pc.PluginConfig,
		&pre,
	)
}

func handlePullRequest(log *logrus.Entry, ghc githubClient, oc ownersClient, config *plugins.Configuration, pre *github.PullRequestEvent) error {
	if pre.Action != github.PullRequestActionOpened &&
		pre.Action != github.PullRequestActionReopened &&
		pre.Action != github.PullRequestActionSynchronize &&
		pre.Action != github.PullRequestActionLabeled {
		return nil
	}
	botName, err := ghc.BotName()
	if err != nil {
		return err
	}
	if pre.Action == github.PullRequestActionLabeled &&
		(pre.Label.Name != approvedLabel || pre.Sender.Login == botName || pre.PullRequest.State == "closed") {
		return nil
	}

	repo, err := oc.LoadRepoOwners(pre.Repo.Owner.Login, pre.Repo.Name, pre.PullRequest.Base.Ref)
	if err != nil {
		return err
	}

	return handleFunc(
		log,
		ghc,
		repo,
		optionsForRepo(config, pre.Repo.Owner.Login, pre.Repo.Name),
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
func findAssociatedIssue(body string) int {
	match := associatedIssueRegex.FindStringSubmatch(body)
	if len(match) == 0 {
		return 0
	}
	v, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}
	return v
}

// handle is the workhorse the will actually make updates to the PR.
// The algorithm goes as:
// - Initially, we build an approverSet
//   - Go through all comments in order of creation.
//     - (Issue/PR comments, PR review comments, and PR review bodies are considered as comments)
//   - If anyone said "/approve", add them to approverSet.
//   - If anyone said "/lgtm" AND LgtmActsAsApprove is enabled, add them to approverSet.
//   - If anyone created an approved review AND ReviewActsAsApprove is enabled, add them to approverSet.
// - Then, for each file, we see if any approver of this file is in approverSet and keep track of files without approval
//   - An approver of a file is defined as:
//     - Someone listed as an "approver" in an OWNERS file in the files directory OR
//     - in one of the file's parent directories
// - Iff all files have been approved, the bot will add the "approved" label.
// - Iff a cancel command is found, that reviewer will be removed from the approverSet
// 	and the munger will remove the approved label if it has been applied
func handle(log *logrus.Entry, ghc githubClient, repo approvers.RepoInterface, opts *plugins.Approve, pr *state) error {
	fetchErr := func(context string, err error) error {
		return fmt.Errorf("failed to get %s for %s/%s#%d: %v", context, pr.org, pr.repo, pr.number, err)
	}

	changes, err := ghc.GetPullRequestChanges(pr.org, pr.repo, pr.number)
	if err != nil {
		return fetchErr("PR file changes", err)
	}
	var filenames []string
	for _, change := range changes {
		filenames = append(filenames, change.Filename)
	}
	labels, err := ghc.GetIssueLabels(pr.org, pr.repo, pr.number)
	if err != nil {
		return fetchErr("issue labels", err)
	}
	hasApprovedLabel := false
	for _, label := range labels {
		if label.Name == approvedLabel {
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

	approversHandler := approvers.NewApprovers(
		approvers.NewOwners(
			log,
			filenames,
			repo,
			int64(pr.number),
		),
	)
	approversHandler.AssociatedIssue = findAssociatedIssue(pr.body)
	approversHandler.RequireIssue = opts.IssueRequired
	approversHandler.ManuallyApproved = humanAddedApproved(ghc, log, pr.org, pr.repo, pr.number, botName, hasApprovedLabel)

	// Author implicitly approves their own PR if config allows it
	if opts.ImplicitSelfApprove {
		approversHandler.AddAuthorSelfApprover(pr.author, pr.htmlURL+"#", false)
	} else {
		// Treat the author as an assignee, and suggest them if possible
		approversHandler.AddAssignees(pr.author)
	}

	commentsFromIssueComments := commentsFromIssueComments(issueComments)
	comments := append(commentsFromReviewComments(reviewComments), commentsFromIssueComments...)
	comments = append(comments, commentsFromReviews(reviews)...)
	sort.SliceStable(comments, func(i, j int) bool {
		return comments[i].CreatedAt.Before(comments[j].CreatedAt)
	})
	approveComments := filterComments(comments, approvalMatcher(botName, opts.LgtmActsAsApprove, opts.ReviewActsAsApprove))
	addApprovers(&approversHandler, approveComments, pr.author, opts.ReviewActsAsApprove)

	for _, user := range pr.assignees {
		approversHandler.AddAssignees(user.Login)
	}

	notifications := filterComments(commentsFromIssueComments, notificationMatcher(botName))
	latestNotification := getLast(notifications)
	newMessage := updateNotification(pr.org, pr.repo, pr.branch, latestNotification, approversHandler)
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

	if !approversHandler.IsApproved() {
		if hasApprovedLabel {
			if err := ghc.RemoveLabel(pr.org, pr.repo, pr.number, approvedLabel); err != nil {
				log.WithError(err).Errorf("Failed to remove %q label from %s/%s#%d.", approvedLabel, pr.org, pr.repo, pr.number)
			}
		}
	} else if !hasApprovedLabel {
		if err := ghc.AddLabel(pr.org, pr.repo, pr.number, approvedLabel); err != nil {
			log.WithError(err).Errorf("Failed to add %q label to %s/%s#%d.", approvedLabel, pr.org, pr.repo, pr.number)
		}
	}
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
			if event.Event != github.IssueActionLabeled || event.Label.Name != approvedLabel {
				continue
			}
			lastAdded = event
		}

		if lastAdded.Actor.Login == "" || lastAdded.Actor.Login == botName || isDeprecatedBot(lastAdded.Actor.Login) {
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

func approvalMatcher(botName string, lgtmActsAsApprove, reviewActsAsApprove bool) func(*comment) bool {
	return func(c *comment) bool {
		return isApprovalCommand(botName, lgtmActsAsApprove, c) || isApprovalState(botName, reviewActsAsApprove, c)
	}
}

func isApprovalCommand(botName string, lgtmActsAsApprove bool, c *comment) bool {
	if c.Author == botName || isDeprecatedBot(c.Author) {
		return false
	}

	for _, match := range commandRegex.FindAllStringSubmatch(c.Body, -1) {
		cmd := strings.ToUpper(match[1])
		if (cmd == lgtmCommand && lgtmActsAsApprove) || cmd == approveCommand {
			return true
		}
	}
	return false
}

func isApprovalState(botName string, reviewActsAsApprove bool, c *comment) bool {
	if c.Author == botName || isDeprecatedBot(c.Author) {
		return false
	}

	// consider reviews in either approved OR requested changes states as
	// approval commands. Reviews in requested changes states will be
	// interpreted as cancelled approvals.
	if reviewActsAsApprove && (c.ReviewState == github.ReviewStateApproved || c.ReviewState == github.ReviewStateChangesRequested) {
		return true
	}
	return false
}

func notificationMatcher(botName string) func(*comment) bool {
	return func(c *comment) bool {
		if c.Author != botName && !isDeprecatedBot(c.Author) {
			return false
		}
		match := notificationRegex.FindStringSubmatch(c.Body)
		return len(match) > 0
	}
}

func updateNotification(org, project, branch string, latestNotification *comment, approversHandler approvers.Approvers) *string {
	message := approvers.GetMessage(approversHandler, org, project, branch)
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
			if name != approveCommand && name != lgtmCommand {
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
			}

			if name == approveCommand {
				approversHandler.AddApprover(
					c.Author,
					c.HTMLURL,
					args == noIssueArgument,
				)
			} else {
				approversHandler.AddLGTMer(
					c.Author,
					c.HTMLURL,
					args == noIssueArgument,
				)
			}

		}
	}
}

// optionsForRepo gets the plugins.Approve struct that is applicable to the indicated repo.
func optionsForRepo(config *plugins.Configuration, org, repo string) *plugins.Approve {
	fullName := fmt.Sprintf("%s/%s", org, repo)
	for i := range config.Approve {
		if !strInSlice(org, config.Approve[i].Repos) && !strInSlice(fullName, config.Approve[i].Repos) {
			continue
		}
		return &config.Approve[i]
	}
	// Default to no issue required and no implicit self approval.
	return &plugins.Approve{}
}

func strInSlice(str string, slice []string) bool {
	for _, elem := range slice {
		if elem == str {
			return true
		}
	}
	return false
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
	comments := []*comment{}
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
	comments := []*comment{}
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
	comments := []*comment{}
	for i := range reviews {
		comments = append(comments, commentFromReview(&reviews[i]))
	}
	return comments
}

func filterComments(comments []*comment, filter func(*comment) bool) []*comment {
	var filtered []*comment
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

func isDeprecatedBot(login string) bool {
	for _, deprecated := range deprecatedBotNames {
		if deprecated == login {
			return true
		}
	}
	return false
}
