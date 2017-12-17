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

	"k8s.io/test-infra/prow/git"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/hook"
	"k8s.io/test-infra/prow/plugins"
)

const pluginName = "cherrypick"

var cherryPickRe = regexp.MustCompile(`(?m)^/cherrypick\s+(.+)$`)

type githubClient interface {
	AssignIssue(org, repo string, number int, logins []string) error
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	GetPullRequestPatch(org, repo string, number int) ([]byte, error)
	CreateComment(org, repo string, number int, comment string) error
	IsMember(org, user string) (bool, error)
	CreatePullRequest(org, repo, title, body, head, base string, canModify bool) (int, error)
	ListIssueComments(org, repo string, number int) ([]github.IssueComment, error)
	CreateFork(org, repo string) error
	GetRepo(owner, name string) (github.Repo, error)
}

// Server implements http.Handler. It validates incoming GitHub webhooks and
// then dispatches them to the appropriate plugins.
type Server struct {
	hmacSecret []byte
	botName    string
	email      string

	gc *git.Client
	// Used for unit testing
	push func(repo, newBranch string) error
	ghc  githubClient
	log  *logrus.Entry

	// Use prow to assign users to cherrypicked PRs.
	prowAssignments bool

	bare     *http.Client
	patchURL string

	repoLock sync.Mutex
	repos    []github.Repo
}

func NewServer(name, email string, hmac []byte, gc *git.Client, ghc *github.Client, repos []github.Repo, prowAssignments bool) *Server {
	return &Server{
		hmacSecret: hmac,
		botName:    name,
		email:      email,

		gc:  gc,
		ghc: ghc,
		log: logrus.StandardLogger().WithField("plugin", pluginName),

		prowAssignments: prowAssignments,

		bare:     &http.Client{},
		patchURL: "https://patch-diff.githubusercontent.com",

		repos: repos,
	}
}

// ServeHTTP validates an incoming webhook and puts it into the event channel.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	eventType, eventGUID, payload, ok := hook.ValidateWebhook(w, r, s.hmacSecret)
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

	l = l.WithFields(logrus.Fields{
		github.OrgLogField:  org,
		github.RepoLogField: repo,
		github.PrLogField:   num,
	})

	cherryPickMatches := cherryPickRe.FindAllStringSubmatch(ic.Comment.Body, -1)
	if len(cherryPickMatches) == 0 || len(cherryPickMatches[0]) != 2 {
		return nil
	}
	targetBranch := cherryPickMatches[0][1]

	if ic.Issue.State != "closed" {
		// Only members should be able to do cherry-picks.
		ok, err := s.ghc.IsMember(org, commentAuthor)
		if err != nil {
			return err
		}
		if !ok {
			resp := fmt.Sprintf("only [%s](https://github.com/orgs/%s/people) org members may request cherry picks. You can still do the cherry-pick manually.", org, org)
			s.log.WithFields(l.Data).Info(resp)
			return s.ghc.CreateComment(org, repo, num, plugins.FormatICResponse(ic.Comment, resp))
		}
		resp := fmt.Sprintf("once the present PR merges, I will cherry-pick it on top of %s in a new PR and assign it to you.", targetBranch)
		s.log.WithFields(l.Data).Info(resp)
		return s.ghc.CreateComment(org, repo, num, plugins.FormatICResponse(ic.Comment, resp))
	}

	pr, err := s.ghc.GetPullRequest(org, repo, num)
	if err != nil {
		return err
	}
	baseBranch := pr.Base.Ref
	title := pr.Title

	// Cherry-pick only merged PRs.
	if !pr.Merged {
		resp := "cannot cherry-pick an unmerged PR"
		s.log.WithFields(l.Data).Info(resp)
		return s.ghc.CreateComment(org, repo, num, plugins.FormatICResponse(ic.Comment, resp))
	}

	return s.handle(l, ic.Comment, org, repo, baseBranch, targetBranch, title, num)
}

func (s *Server) handlePullRequest(l *logrus.Entry, pre github.PullRequestEvent) error {
	// Only consider newly merged PRs
	if pre.Action != github.PullRequestActionClosed {
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

	l = l.WithFields(logrus.Fields{
		github.OrgLogField:  org,
		github.RepoLogField: repo,
		github.PrLogField:   num,
	})

	comments, err := s.ghc.ListIssueComments(org, repo, num)
	if err != nil {
		return err
	}

	var targetBranch string
	var ic *github.IssueComment
	for _, c := range comments {
		cherryPickMatches := cherryPickRe.FindAllStringSubmatch(c.Body, -1)
		if len(cherryPickMatches) == 0 || len(cherryPickMatches[0]) != 2 {
			continue
		}
		// TODO: Collect all "/cherrypick" comments and figure out if any
		// comes from an org member?
		targetBranch = cherryPickMatches[0][1]
		ic = &c
		break
	}
	if targetBranch == "" || ic == nil {
		return nil
	}
	return s.handle(l, *ic, org, repo, baseBranch, targetBranch, title, num)
}

var cherryPickBranchFmt = "cherry-pick-%d-to-%s"

func (s *Server) handle(l *logrus.Entry, comment github.IssueComment, org, repo, baseBranch, targetBranch, title string, num int) error {
	// TODO: Use a whitelist for allowed base and target branches.
	if baseBranch == targetBranch {
		resp := fmt.Sprintf("base branch (%s) needs to differ from target branch (%s)", baseBranch, targetBranch)
		s.log.WithFields(l.Data).Info(resp)
		return s.ghc.CreateComment(org, repo, num, plugins.FormatICResponse(comment, resp))
	}

	// Only members should be able to do cherry-picks.
	commentAuthor := comment.User.Login
	ok, err := s.ghc.IsMember(org, commentAuthor)
	if err != nil {
		return err
	}
	if !ok {
		resp := fmt.Sprintf("only [%s](https://github.com/orgs/%s/people) org members may request cherry picks. You can still do the cherry-pick manually.", org, org)
		s.log.WithFields(l.Data).Info(resp)
		return s.ghc.CreateComment(org, repo, num, plugins.FormatICResponse(comment, resp))
	}

	if err := s.ensureForkExists(org, repo); err != nil {
		return err
	}

	// Clone the repo, checkout the target branch.
	startClone := time.Now()
	r, err := s.gc.Clone(org + "/" + repo)
	if err != nil {
		return err
	}
	defer func() {
		if err := r.Clean(); err != nil {
			s.log.WithError(err).WithFields(l.Data).Error("Error cleaning up repo.")
		}
	}()
	if err := r.Checkout(targetBranch); err != nil {
		resp := fmt.Sprintf("cannot checkout %s: %v", targetBranch, err)
		s.log.WithFields(l.Data).Info(resp)
		return s.ghc.CreateComment(org, repo, num, plugins.FormatICResponse(comment, resp))
	}
	s.log.WithFields(l.Data).WithField("duration", time.Since(startClone)).Info("Cloned and checked out target branch.")

	// Fetch the patch from Github
	localPath, err := s.getPatch(org, repo, targetBranch, num)
	if err != nil {
		return err
	}

	if err := r.Config("user.name", s.botName); err != nil {
		return err
	}
	email := s.email
	if email == "" {
		email = fmt.Sprintf("%s@localhost", s.botName)
	}
	if err := r.Config("user.email", email); err != nil {
		return err
	}

	// Checkout a new branch for the cherry-pick.
	newBranch := fmt.Sprintf(cherryPickBranchFmt, num, targetBranch)
	if err := r.CheckoutNewBranch(newBranch); err != nil {
		return err
	}

	// Apply the patch.
	if err := r.Am(localPath); err != nil {
		resp := fmt.Sprintf("#%d failed to apply on top of branch %q:\n```%v\n```", num, targetBranch, err)
		s.log.WithFields(l.Data).Info(resp)
		return s.ghc.CreateComment(org, repo, num, plugins.FormatICResponse(comment, resp))
	}

	push := r.Push
	if s.push != nil {
		push = s.push
	}
	// Push the new branch in the bot's fork.
	if err := push(repo, newBranch); err != nil {
		resp := fmt.Sprintf("failed to push cherry-picked changes in Github: %v", err)
		s.log.WithFields(l.Data).Info(resp)
		return s.ghc.CreateComment(org, repo, num, plugins.FormatICResponse(comment, resp))
	}

	// Open a PR in Github.
	title = fmt.Sprintf("[%s] %s", targetBranch, title)
	body := fmt.Sprintf("This is an automated cherry-pick of #%d", num)
	if s.prowAssignments {
		body = fmt.Sprintf("%s\n\n/assign %s", body, commentAuthor)
	}
	head := fmt.Sprintf("%s:%s", s.botName, newBranch)
	createdNum, err := s.ghc.CreatePullRequest(org, repo, title, body, head, targetBranch, true)
	if err != nil {
		resp := fmt.Sprintf("new pull request could not be created: %v", err)
		s.log.WithFields(l.Data).Info(resp)
		return s.ghc.CreateComment(org, repo, num, plugins.FormatICResponse(comment, resp))
	}
	resp := fmt.Sprintf("new pull request created: #%d", createdNum)
	s.log.WithFields(l.Data).Info(resp)
	if err := s.ghc.CreateComment(org, repo, num, plugins.FormatICResponse(comment, resp)); err != nil {
		return err
	}
	if !s.prowAssignments {
		if err := s.ghc.AssignIssue(org, repo, createdNum, []string{commentAuthor}); err != nil {
			return err
		}
	}
	return nil
}

// ensureForkExists ensures a fork of org/repo exists for the bot.
func (s *Server) ensureForkExists(org, repo string) error {
	s.repoLock.Lock()
	defer s.repoLock.Unlock()

	// Fork repo if it doesn't exist.
	fork := s.botName + "/" + repo
	if !repoExists(fork, s.repos) {
		if err := s.ghc.CreateFork(org, repo); err != nil {
			return fmt.Errorf("cannot fork %s/%s: %v", org, repo, err)
		}
		if err := waitForRepo(s.botName, repo, s.ghc); err != nil {
			return fmt.Errorf("fork of %s/%s cannot show up on Github: %v", org, repo, err)
		}
		s.repos = append(s.repos, github.Repo{FullName: fork, Fork: true})
	}
	return nil
}

func waitForRepo(owner, name string, ghc githubClient) error {
	// Wait for at most 5 minutes for the fork to appear on Github.
	after := time.After(5 * time.Minute)
	tick := time.Tick(5 * time.Second)

	var ghErr string
	for {
		select {
		case <-tick:
			repo, err := ghc.GetRepo(owner, name)
			if err != nil {
				ghErr = fmt.Sprintf(": %v", err)
				logrus.WithError(err).Warn("Error getting bot repository.")
				continue
			}
			ghErr = ""
			if repoExists(owner+"/"+name, []github.Repo{repo}) {
				return nil
			}
		case <-after:
			return fmt.Errorf("timed out waiting for %s to appear on Github%s", owner+"/"+name, ghErr)
		}
	}
}

func repoExists(repo string, repos []github.Repo) bool {
	for _, r := range repos {
		if !r.Fork {
			continue
		}
		if r.FullName == repo {
			return true
		}
	}
	return false
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
