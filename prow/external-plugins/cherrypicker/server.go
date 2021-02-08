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

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	cherrypicker "k8s.io/test-infra/prow/external-plugins/cherrypicker/lib"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const pluginName = "cherrypick"
const defaultLabelPrefix = "cherrypick/"

var cherryPickRe = regexp.MustCompile(`(?m)^(?:/cherrypick|/cherry-pick)\s+(.+)$`)
var releaseNoteRe = regexp.MustCompile(`(?s)(?:Release note\*\*:\s*(?:<!--[^<>]*-->\s*)?` + "```(?:release-note)?|```release-note)(.+?)```")

type githubClient interface {
	AddLabel(org, repo string, number int, label string) error
	AssignIssue(org, repo string, number int, logins []string) error
	CreateComment(org, repo string, number int, comment string) error
	CreateFork(org, repo string) (string, error)
	CreatePullRequest(org, repo, title, body, head, base string, canModify bool) (int, error)
	CreateIssue(org, repo, title, body string, milestone int, labels, assignees []string) (int, error)
	EnsureFork(forkingUser, org, repo string) (string, error)
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	GetPullRequestPatch(org, repo string, number int) ([]byte, error)
	GetPullRequests(org, repo string) ([]github.PullRequest, error)
	GetRepo(owner, name string) (github.FullRepo, error)
	IsMember(org, user string) (bool, error)
	ListIssueComments(org, repo string, number int) ([]github.IssueComment, error)
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
	ListOrgMembers(org, role string) ([]github.TeamMember, error)
}

// HelpProvider construct the pluginhelp.PluginHelp for this plugin.
func HelpProvider(_ []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	pluginHelp := &pluginhelp.PluginHelp{
		Description: `The cherrypick plugin is used for cherrypicking PRs across branches. For every successful cherrypick invocation a new PR is opened against the target branch and assigned to the requestor. If the parent PR contains a release note, it is copied to the cherrypick PR.`,
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/cherrypick [branch]",
		Description: "Cherrypick a PR to a different branch. This command works both in merged PRs (the cherrypick PR is opened immediately) and open PRs (the cherrypick PR opens as soon as the original PR merges).",
		Featured:    true,
		// depends on how the cherrypick server runs; needs auth by default (--allow-all=false)
		WhoCanUse: "Members of the trusted organization for the repo.",
		Examples:  []string{"/cherrypick release-3.9", "/cherry-pick release-1.15"},
	})
	return pluginHelp, nil
}

// Server implements http.Handler. It validates incoming GitHub webhooks and
// then dispatches them to the appropriate plugins.
type Server struct {
	tokenGenerator func() []byte
	botUser        *github.UserData
	email          string

	gc git.ClientFactory
	// Used for unit testing
	push func(newBranch string, force bool) error
	ghc  githubClient
	log  *logrus.Entry

	// Labels to apply to the cherrypicked PR.
	labels []string
	// Use prow to assign users to cherrypicked PRs.
	prowAssignments bool
	// Allow anybody to do cherrypicks.
	allowAll bool
	// Create an issue on cherrypick conflict.
	issueOnConflict bool
	// Set a custom label prefix.
	labelPrefix string

	bare     *http.Client
	patchURL string

	repoLock sync.Mutex
	repos    []github.Repo
}

// ServeHTTP validates an incoming webhook and puts it into the event channel.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	eventType, eventGUID, payload, ok, _ := github.ValidateWebhook(w, r, s.tokenGenerator)
	if !ok {
		return
	}
	fmt.Fprint(w, "Event received. Have a nice day.")

	if err := s.handleEvent(eventType, eventGUID, payload); err != nil {
		logrus.WithError(err).Error("Error parsing event.")
	}
}

func (s *Server) handleEvent(eventType, eventGUID string, payload []byte) error {
	l := logrus.WithFields(
		logrus.Fields{
			"event-type":     eventType,
			github.EventGUID: eventGUID,
		},
	)
	switch eventType {
	case "issue_comment":
		var ic github.IssueCommentEvent
		if err := json.Unmarshal(payload, &ic); err != nil {
			return err
		}
		go func() {
			if err := s.handleIssueComment(l, ic); err != nil {
				s.log.WithError(err).WithFields(l.Data).Info("Cherry-pick failed.")
			}
		}()
	case "pull_request":
		var pr github.PullRequestEvent
		if err := json.Unmarshal(payload, &pr); err != nil {
			return err
		}
		go func() {
			if err := s.handlePullRequest(l, pr); err != nil {
				s.log.WithError(err).WithFields(l.Data).Info("Cherry-pick failed.")
			}
		}()
	default:
		logrus.Debugf("skipping event of type %q", eventType)
	}
	return nil
}

func (s *Server) handleIssueComment(l *logrus.Entry, ic github.IssueCommentEvent) error {
	// Only consider new comments in PRs.
	if !ic.Issue.IsPullRequest() || ic.Action != github.IssueCommentActionCreated {
		return nil
	}

	org := ic.Repo.Owner.Login
	repo := ic.Repo.Name
	num := ic.Issue.Number
	commentAuthor := ic.Comment.User.Login

	// Do not create a new logger, its fields are re-used by the caller in case of errors
	l = l.WithFields(logrus.Fields{
		github.OrgLogField:  org,
		github.RepoLogField: repo,
		github.PrLogField:   num,
	})

	cherryPickMatches := cherryPickRe.FindAllStringSubmatch(ic.Comment.Body, -1)
	if len(cherryPickMatches) == 0 || len(cherryPickMatches[0]) != 2 {
		return nil
	}
	targetBranch := strings.TrimSpace(cherryPickMatches[0][1])

	if ic.Issue.State != "closed" {
		if !s.allowAll {
			// Only members should be able to do cherry-picks.
			ok, err := s.ghc.IsMember(org, commentAuthor)
			if err != nil {
				return err
			}
			if !ok {
				resp := fmt.Sprintf("only [%s](https://github.com/orgs/%s/people) org members may request cherry-picks. You can still do the cherry-pick manually.", org, org)
				l.Info(resp)
				return s.ghc.CreateComment(org, repo, num, plugins.FormatICResponse(ic.Comment, resp))
			}
		}
		resp := fmt.Sprintf("once the present PR merges, I will cherry-pick it on top of %s in a new PR and assign it to you.", targetBranch)
		l.Info(resp)
		return s.ghc.CreateComment(org, repo, num, plugins.FormatICResponse(ic.Comment, resp))
	}

	pr, err := s.ghc.GetPullRequest(org, repo, num)
	if err != nil {
		return err
	}
	baseBranch := pr.Base.Ref
	title := pr.Title
	body := pr.Body

	// Cherry-pick only merged PRs.
	if !pr.Merged {
		resp := "cannot cherry-pick an unmerged PR"
		l.Info(resp)
		return s.ghc.CreateComment(org, repo, num, plugins.FormatICResponse(ic.Comment, resp))
	}

	// TODO: Use an allowlist for allowed base and target branches.
	if baseBranch == targetBranch {
		resp := fmt.Sprintf("base branch (%s) needs to differ from target branch (%s)", baseBranch, targetBranch)
		l.Info(resp)
		return s.ghc.CreateComment(org, repo, num, plugins.FormatICResponse(ic.Comment, resp))
	}

	if !s.allowAll {
		// Only org members should be able to do cherry-picks.
		ok, err := s.ghc.IsMember(org, commentAuthor)
		if err != nil {
			return err
		}
		if !ok {
			resp := fmt.Sprintf("only [%s](https://github.com/orgs/%s/people) org members may request cherry picks. You can still do the cherry-pick manually.", org, org)
			l.Info(resp)
			return s.ghc.CreateComment(org, repo, num, plugins.FormatICResponse(ic.Comment, resp))
		}
	}

	l = l.WithFields(logrus.Fields{
		"requestor":     ic.Comment.User.Login,
		"target_branch": targetBranch,
	})
	l.Debug("Cherrypick request.")
	return s.handle(l, ic.Comment.User.Login, &ic.Comment, org, repo, targetBranch, title, body, num)
}

func (s *Server) handlePullRequest(l *logrus.Entry, pre github.PullRequestEvent) error {
	// Only consider newly merged PRs
	if pre.Action != github.PullRequestActionClosed && pre.Action != github.PullRequestActionLabeled {
		return nil
	}

	pr := pre.PullRequest
	if !pr.Merged || pr.MergeSHA == nil {
		return nil
	}

	org := pr.Base.Repo.Owner.Login
	repo := pr.Base.Repo.Name
	baseBranch := pr.Base.Ref
	num := pr.Number
	title := pr.Title
	body := pr.Body

	// Do not create a new logger, its fields are re-used by the caller in case of errors
	l = l.WithFields(logrus.Fields{
		github.OrgLogField:  org,
		github.RepoLogField: repo,
		github.PrLogField:   num,
	})

	comments, err := s.ghc.ListIssueComments(org, repo, num)
	if err != nil {
		return err
	}

	// requestor -> target branch -> issue comment
	requestorToComments := make(map[string]map[string]*github.IssueComment)

	// first look for our special comments
	for i := range comments {
		c := comments[i]
		cherryPickMatches := cherryPickRe.FindAllStringSubmatch(c.Body, -1)
		if len(cherryPickMatches) == 0 || len(cherryPickMatches[0]) != 2 {
			continue
		}
		// TODO: Support comments with multiple cherrypick invocations.
		targetBranch := strings.TrimSpace(cherryPickMatches[0][1])
		if requestorToComments[c.User.Login] == nil {
			requestorToComments[c.User.Login] = make(map[string]*github.IssueComment)
		}
		requestorToComments[c.User.Login][targetBranch] = &c
	}

	foundCherryPickComments := len(requestorToComments) != 0

	// now look for our special labels
	labels, err := s.ghc.GetIssueLabels(org, repo, num)
	if err != nil {
		return err
	}

	if requestorToComments[pr.User.Login] == nil {
		requestorToComments[pr.User.Login] = make(map[string]*github.IssueComment)
	}

	foundCherryPickLabels := false
	for _, label := range labels {
		if strings.HasPrefix(label.Name, s.labelPrefix) {
			requestorToComments[pr.User.Login][label.Name[len(s.labelPrefix):]] = nil // leave this nil which indicates a label-initiated cherry-pick
			foundCherryPickLabels = true
		}
	}

	if !foundCherryPickComments && !foundCherryPickLabels {
		return nil
	}

	// Figure out membership.
	if !s.allowAll {
		// TODO: Possibly cache this.
		members, err := s.ghc.ListOrgMembers(org, "all")
		if err != nil {
			return err
		}
		for requestor := range requestorToComments {
			isMember := false
			for _, m := range members {
				if requestor == m.Login {
					isMember = true
					break
				}
			}
			if !isMember {
				delete(requestorToComments, requestor)
			}
		}
	}

	// Handle multiple comments serially. Make sure to filter out
	// comments targeting the same branch.
	handledBranches := make(map[string]bool)
	for requestor, branches := range requestorToComments {
		for targetBranch, ic := range branches {
			if targetBranch == baseBranch {
				resp := fmt.Sprintf("base branch (%s) needs to differ from target branch (%s)", baseBranch, targetBranch)
				l.Info(resp)
				s.createComment(org, repo, num, ic, resp)
				continue
			}
			if handledBranches[targetBranch] {
				// Branch already handled. Skip.
				continue
			}
			handledBranches[targetBranch] = true
			l.WithFields(logrus.Fields{
				"requestor":     requestor,
				"target_branch": targetBranch,
			}).Debug("Cherrypick request.")
			err := s.handle(l, requestor, ic, org, repo, targetBranch, title, body, num)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

var cherryPickBranchFmt = "cherry-pick-%d-to-%s"

func (s *Server) handle(logger *logrus.Entry, requestor string, comment *github.IssueComment, org, repo, targetBranch, title, body string, num int) error {
	forkName, err := s.ensureForkExists(org, repo)
	if err != nil {
		resp := fmt.Sprintf("cannot fork %s/%s: %v", org, repo, err)
		logger.Warningf(resp)
		return s.createComment(org, repo, num, comment, resp)
	}

	// Clone the repo, checkout the target branch.
	startClone := time.Now()
	r, err := s.gc.ClientFor(org, forkName)
	if err != nil {
		return err
	}
	defer func() {
		if err := r.Clean(); err != nil {
			logger.WithError(err).Error("Error cleaning up repo.")
		}
	}()
	if err := r.Checkout(targetBranch); err != nil {
		resp := fmt.Sprintf("cannot checkout `%s`: %v", targetBranch, err)
		logger.Warningf(resp)
		return s.createComment(org, repo, num, comment, resp)
	}
	logger.WithField("duration", time.Since(startClone)).Info("Cloned and checked out target branch.")

	// Fetch the patch from GitHub
	localPath, err := s.getPatch(org, repo, targetBranch, num)
	if err != nil {
		return err
	}

	if err := r.Config("user.name", s.botUser.Login); err != nil {
		return err
	}
	email := s.email
	if email == "" {
		email = s.botUser.Email
	}
	if err := r.Config("user.email", email); err != nil {
		return err
	}

	// New branch for the cherry-pick.
	newBranch := fmt.Sprintf(cherryPickBranchFmt, num, targetBranch)

	// Check if that branch already exists, which means there is already a PR for that cherry-pick.
	if r.BranchExists(newBranch) {
		// Find the PR and link to it.
		prs, err := s.ghc.GetPullRequests(org, repo)
		if err != nil {
			return err
		}
		for _, pr := range prs {
			if pr.Head.Ref == fmt.Sprintf("%s:%s", s.botUser.Login, newBranch) {
				resp := fmt.Sprintf("Looks like #%d has already been cherry picked in %s", num, pr.HTMLURL)
				logger.Info(resp)
				return s.createComment(org, repo, num, comment, resp)
			}
		}
	}

	// Create the branch for the cherry-pick.
	if err := r.CheckoutNewBranch(newBranch); err != nil {
		return err
	}

	// Title for GitHub issue/PR.
	title = fmt.Sprintf("[%s] %s", targetBranch, title)

	// Apply the patch.
	if err := r.Am(localPath); err != nil {
		resp := fmt.Sprintf("#%d failed to apply on top of branch %q:\n```\n%v\n```", num, targetBranch, err)
		logger.Info(resp)
		err := s.createComment(org, repo, num, comment, resp)

		if s.issueOnConflict {
			resp = fmt.Sprintf("Manual cherrypick required.\n\n%v", resp)
			return s.createIssue(org, repo, title, resp, num, comment, nil, []string{requestor})
		}

		return err
	}

	push := r.PushToFork
	if s.push != nil {
		push = s.push
	}
	// Push the new branch in the bot's fork.
	if err := push(newBranch, true); err != nil {
		resp := fmt.Sprintf("failed to push cherry-picked changes in GitHub: %v", err)
		logger.Info(resp)
		return s.createComment(org, repo, num, comment, resp)
	}

	// Open a PR in GitHub.
	var cherryPickBody string
	if s.prowAssignments {
		cherryPickBody = cherrypicker.CreateCherrypickBody(num, requestor, releaseNoteFromParentPR(body))
	} else {
		cherryPickBody = cherrypicker.CreateCherrypickBody(num, "", releaseNoteFromParentPR(body))
	}
	head := fmt.Sprintf("%s:%s", s.botUser.Login, newBranch)
	createdNum, err := s.ghc.CreatePullRequest(org, repo, title, cherryPickBody, head, targetBranch, true)
	if err != nil {
		resp := fmt.Sprintf("new pull request could not be created: %v", err)
		logger.Info(resp)
		return s.createComment(org, repo, num, comment, resp)
	}
	resp := fmt.Sprintf("new pull request created: #%d", createdNum)
	logger.Info(resp)
	if err := s.createComment(org, repo, num, comment, resp); err != nil {
		return err
	}
	for _, label := range s.labels {
		if err := s.ghc.AddLabel(org, repo, createdNum, label); err != nil {
			return err
		}
	}
	if !s.prowAssignments {
		if err := s.ghc.AssignIssue(org, repo, createdNum, []string{requestor}); err != nil {
			logger.Warningf("Cannot assign to new PR: %v", err)
			// Ignore returning errors on failure to assign as this is most likely
			// due to users not being members of the org so that they can't be assigned
			// in PRs.
			return nil
		}
	}
	return nil
}

func (s *Server) createComment(org, repo string, num int, comment *github.IssueComment, resp string) error {
	if comment != nil {
		return s.ghc.CreateComment(org, repo, num, plugins.FormatICResponse(*comment, resp))
	}
	return s.ghc.CreateComment(org, repo, num, fmt.Sprintf("In response to a cherrypick label: %s", resp))
}

// createIssue creates an issue on GitHub.
func (s *Server) createIssue(org, repo, title, body string, num int, comment *github.IssueComment, labels, assignees []string) error {
	issueNum, err := s.ghc.CreateIssue(org, repo, title, body, 0, labels, assignees)
	if err != nil {
		return s.createComment(org, repo, num, comment, fmt.Sprintf("new issue could not be created for failed cherrypick: %v", err))
	}

	return s.createComment(org, repo, num, comment, fmt.Sprintf("new issue created for failed cherrypick: #%d", issueNum))
}

// ensureForkExists ensures a fork of org/repo exists for the bot.
func (s *Server) ensureForkExists(org, repo string) (string, error) {
	s.repoLock.Lock()
	defer s.repoLock.Unlock()
	fork := s.botUser.Login + "/" + repo

	// fork repo if it doesn't exsit
	if _, err := s.ghc.EnsureFork(s.botUser.Login, org, repo); err != nil {
		return repo, err
	}

	s.repos = append(s.repos, github.Repo{FullName: fork, Fork: true})
	return repo, nil
}

// getPatch gets the patch for the provided PR and creates a local
// copy of it. It returns its location in the filesystem and any
// encountered error.
func (s *Server) getPatch(org, repo, targetBranch string, num int) (string, error) {
	patch, err := s.ghc.GetPullRequestPatch(org, repo, num)
	if err != nil {
		return "", err
	}
	localPath := fmt.Sprintf("/tmp/%s_%s_%d_%s.patch", org, repo, num, normalize(targetBranch))
	out, err := os.Create(localPath)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, bytes.NewBuffer(patch)); err != nil {
		return "", err
	}
	return localPath, nil
}

func normalize(input string) string {
	return strings.Replace(input, "/", "-", -1)
}

// releaseNoteNoteFromParentPR gets the release note from the
// parent PR and formats it as per the PR template so that
// it can be copied to the cherry-pick PR.
func releaseNoteFromParentPR(body string) string {
	potentialMatch := releaseNoteRe.FindStringSubmatch(body)
	if potentialMatch == nil {
		return ""
	}
	return fmt.Sprintf("```release-note\n%s\n```", strings.TrimSpace(potentialMatch[1]))
}
