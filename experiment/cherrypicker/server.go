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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sync"
	"time"

	uuid "github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/git"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/hook"
	"k8s.io/test-infra/prow/plugins"
)

const pluginName = "cherrypick"

var cherryPickRe = regexp.MustCompile(`(?m)^/cherrypick\s+(.+)$`)

type githubClient interface {
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	CreateComment(org, repo string, number int, comment string) error
	IsMember(org, user string) (bool, error)
	CreatePullRequest(org, repo, title, body, head, base string, canModify bool) (int, error)
	ListIssueComments(org, repo string, number int) ([]github.IssueComment, error)
	CreateFork(org, repo string) error
}

// Server implements http.Handler. It validates incoming GitHub webhooks and
// then dispatches them to the appropriate plugins.
type Server struct {
	hmacSecret  []byte
	credentials string
	botName     string

	gc *git.Client
	// Used for unit testing
	push func(botName, credentials, repo, newBranch string) error
	ghc  githubClient
	log  *logrus.Entry

	bare     *http.Client
	patchURL string

	repoLock sync.Mutex
	repos    []github.Repo
}

func NewServer(name, creds string, hmac []byte, gc *git.Client, ghc *github.Client, repos []github.Repo) *Server {
	return &Server{
		hmacSecret:  hmac,
		credentials: creds,
		botName:     name,

		gc:  gc,
		ghc: ghc,
		log: logrus.StandardLogger().WithField("client", "cherrypicker"),

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
	switch eventType {
	case "issue_comment":
		var ic github.IssueCommentEvent
		if err := json.Unmarshal(payload, &ic); err != nil {
			return err
		}
		go func() {
			if err := s.handleIssueComment(ic); err != nil {
				s.log.WithError(err).Info("Cherry-pick failed.")
			}
		}()
	case "pull_request":
		var pr github.PullRequestEvent
		if err := json.Unmarshal(payload, &pr); err != nil {
			return err
		}
		go func() {
			if err := s.handlePullRequest(pr); err != nil {
				s.log.WithError(err).Info("Cherry-pick failed.")
			}
		}()
	default:
		return fmt.Errorf("received an event of type %q but didn't ask for it", eventType)
	}
	return nil
}

func (s *Server) handleIssueComment(ic github.IssueCommentEvent) error {
	// Only consider new comments in PRs.
	if !ic.Issue.IsPullRequest() || ic.Action != github.IssueCommentActionCreated {
		return nil
	}

	org := ic.Repo.Owner.Login
	repo := ic.Repo.Name
	num := ic.Issue.Number
	commentAuthor := ic.Comment.User.Login

	cherryPickMatches := cherryPickRe.FindAllStringSubmatch(ic.Comment.Body, -1)
	if len(cherryPickMatches) == 0 || len(cherryPickMatches[0]) != 2 {
		return nil
	}
	targetBranch := cherryPickMatches[0][1]

	if targetBranch == "master" {
		return s.ghc.CreateComment(org, repo, num, plugins.FormatICResponse(ic.Comment, "I don't cherrypick pull requests onto master."))
	}

	if ic.Issue.State != "closed" {
		// Only members should be able to do cherry-picks.
		ok, err := s.ghc.IsMember(org, commentAuthor)
		if err != nil {
			return err
		}
		if !ok {
			resp := fmt.Sprintf("Only [%s](https://github.com/orgs/%s/people) org members may request cherry picks. You can still do the cherry-pick manually.", org, org)
			s.log.Info(resp)
			return s.ghc.CreateComment(org, repo, num, plugins.FormatICResponse(ic.Comment, resp))
		}
		resp := fmt.Sprintf("@%s once the present PR merges, I will cherry-pick it on top of %s in a new PR and assign it to you.", commentAuthor, targetBranch)
		return s.ghc.CreateComment(org, repo, num, plugins.FormatICResponse(ic.Comment, resp))
	}

	pr, err := s.ghc.GetPullRequest(org, repo, num)
	if err != nil {
		return err
	}
	baseBranch := pr.Base.Ref

	// Cherry-pick only merged PRs.
	if !pr.Merged {
		resp := "cannot cherry-pick an unmerged PR"
		s.log.Info(resp)
		return s.ghc.CreateComment(org, repo, num, plugins.FormatICResponse(ic.Comment, resp))
	}

	if baseBranch != "master" {
		resp := "The base branch needs to be master in order to cherry-pick to a different branch."
		s.log.Info(resp)
		return s.ghc.CreateComment(org, repo, num, plugins.FormatICResponse(ic.Comment, resp))
	}

	return s.handle(ic.Comment, org, repo, baseBranch, targetBranch, num)
}

func (s *Server) handlePullRequest(pre github.PullRequestEvent) error {
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

	if baseBranch != "master" {
		// We can't know at this point whether there was actually a /cherrypick comment in the PR.
		return nil
	}

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
	return s.handle(*ic, org, repo, baseBranch, targetBranch, num)
}

var cherryPickBranchFmt = "cherry-pick-%d-to-%s"

func (s *Server) handle(comment github.IssueComment, org, repo, baseBranch, targetBranch string, num int) error {
	// TODO: Use a whitelist for allowed base and target branches.
	if baseBranch == targetBranch {
		resp := fmt.Sprintf("Base branch (%s) needs to differ from target branch (%s)", baseBranch, targetBranch)
		s.log.Info(resp)
		return s.ghc.CreateComment(org, repo, num, plugins.FormatICResponse(comment, resp))
	}

	// Only members should be able to do cherry-picks.
	commentAuthor := comment.User.Login
	ok, err := s.ghc.IsMember(org, commentAuthor)
	if err != nil {
		return err
	}
	if !ok {
		resp := fmt.Sprintf("Only [%s](https://github.com/orgs/%s/people) org members may request cherry picks. You can still do the cherry-pick manually.", org, org)
		s.log.Info(resp)
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
			s.log.WithError(err).Error("Error cleaning up repo.")
		}
	}()
	if err := r.Checkout(targetBranch); err != nil {
		return err
	}
	s.log.WithField("duration", time.Since(startClone)).Info("Cloned and checked out target branch.")

	// Fetch the patch from Github
	localPath, err := s.getPatch(org, repo, num)
	if err != nil {
		return err
	}

	if err := r.Config("user.name", "cherrypicker"); err != nil {
		return err
	}
	if err := r.Config("user.email", "cherrypicker@localhost"); err != nil {
		return err
	}

	// Checkout a new branch for the cherry-pick.
	newBranch := fmt.Sprintf(cherryPickBranchFmt, num, targetBranch)
	if err := r.CheckoutNewBranch(newBranch); err != nil {
		return err
	}

	// Apply the patch.
	if err := r.Apply(localPath); err != nil {
		resp := fmt.Sprintf("PR %d failed to apply on top of branch %q: %v", num, targetBranch, err)
		return s.ghc.CreateComment(org, repo, num, plugins.FormatICResponse(comment, resp))
	}

	push := r.Push
	if s.push != nil {
		push = s.push
	}
	// Push the new branch in the bot's fork.
	if err := push(s.botName, s.credentials, repo, newBranch); err != nil {
		resp := fmt.Sprintf("Failed to push cherry-picked changes in Github: %v", err)
		return s.ghc.CreateComment(org, repo, num, plugins.FormatICResponse(comment, resp))
	}

	// Open a PR in Github.
	title := fmt.Sprintf("Automated cherry-pick of #%d on %s", num, targetBranch)
	// TODO: Make this a configurable template
	body := fmt.Sprintf("This is an automated cherry-pick of #%d\n\n/assign %s", num, commentAuthor)
	head := fmt.Sprintf("%s:%s", s.botName, newBranch)
	createdNum, err := s.ghc.CreatePullRequest(org, repo, title, body, head, targetBranch, true)
	if err != nil {
		resp := fmt.Sprintf("New pull request could not be created: %v", err)
		return s.ghc.CreateComment(org, repo, num, plugins.FormatICResponse(comment, resp))
	}
	resp := fmt.Sprintf("New pull request created: #%d", createdNum)
	return s.ghc.CreateComment(org, repo, num, plugins.FormatICResponse(comment, resp))
}

// ensureForkExists ensures a fork of org/repo exists for the bot.
func (s *Server) ensureForkExists(org, repo string) error {
	s.repoLock.Lock()
	defer s.repoLock.Unlock()

	var exists bool
	fork := s.botName + "/" + repo
	for _, r := range s.repos {
		if !r.Fork {
			continue
		}
		if r.FullName == fork {
			exists = true
			break
		}
	}
	// Fork repo if it doesn't exist.
	if !exists {
		if err := s.ghc.CreateFork(org, repo); err != nil {
			return fmt.Errorf("cannot fork %s/%s: %v", org, repo, err)
		}
		if err := waitFork(fmt.Sprintf("https://github.com/%s/%s", s.botName, repo)); err != nil {
			return fmt.Errorf("fork of %s/%s cannot show up on Github: %v", org, repo, err)
		}
		s.repos = append(s.repos, github.Repo{FullName: fork, Fork: true})
	}
	return nil
}

func waitFork(forkURL string) error {
	// Wait for at most 5 minutes for the fork to appear on Github.
	after := time.After(5 * time.Minute)
	tick := time.Tick(5 * time.Second)

	for {
		select {
		case <-tick:
			resp, err := http.Get(forkURL)
			if err == nil && resp.StatusCode == 200 {
				resp.Body.Close()
				return nil
			}
			if err == nil {
				resp.Body.Close()
			}
		case <-after:
			return fmt.Errorf("timed out waiting for %s to appear on Github", forkURL)
		}
	}
}

// getPatch gets the patch for the provided PR and creates a local
// copy of it. It returns its location in the filesystem and any
// encountered error.
func (s *Server) getPatch(org, repo string, num int) (string, error) {
	url := fmt.Sprintf(s.patchURL+"/raw/%s/%s/pull/%d.patch", org, repo, num)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	// TODO: Add retries
	resp, err := s.bare.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("cannot get Github patch for PR %d: %s", num, resp.Status)
	}
	localPath := fmt.Sprintf("/tmp/%d-%s.patch", num, uuid.NewV1().String())
	out, err := os.Create(localPath)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, resp.Body); err != nil {
		return "", err
	}
	return localPath, nil
}
