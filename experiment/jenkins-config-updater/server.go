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

// jenkins-config-updater watches for merged PRs which update a set of files
// and update the corresponding files in a given deployment
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/git"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/hook"
)

const pluginName = "jenkins-config-updater"

type githubClient interface {
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
	CreateComment(org, repo string, number int, comment string) error
	IsMember(org, user string) (bool, error)
	CreatePullRequest(org, repo, title, body, head, base string, canModify bool) (int, error)
	ListPullRequestComments(org, repo string, number int) ([]github.ReviewComment, error)
	CreateFork(org, repo string) error
}

// Server implements http.Handler. It validates incoming GitHub webhooks and
// then dispatches them to the appropriate plugins.
type Server struct {
	hmacSecret  []byte
	credentials string
	botName     string

	gc  *git.Client
	ghc githubClient
	log *logrus.Entry

	repos []github.Repo
}

// NewServer returns new server
func NewServer(name, creds string, hmac []byte, gc *git.Client, ghc *github.Client, repos []github.Repo) *Server {
	return &Server{
		hmacSecret:  hmac,
		credentials: creds,
		botName:     name,

		gc:  gc,
		ghc: ghc,
		log: logrus.StandardLogger().WithField("client", "jenkins-config-updater"),

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
	s.log.Debug("received webhook")
	if eventType != "pull_request" {
		return fmt.Errorf("received an event of type %q but didn't ask for it", eventType)
	}

	var pre github.PullRequestEvent
	if err := json.Unmarshal(payload, &pre); err != nil {
		return err
	}

	if pre.Action != github.PullRequestActionClosed {
		return nil
	}

	pr := pre.PullRequest
	if !pr.Merged || pr.MergeSHA == nil {
		return nil
	}

	org := pr.Base.Repo.Owner.Login
	repo := pr.Base.Repo.Name
	num := pr.Number

	changes, err := s.ghc.GetPullRequestChanges(org, repo, num)
	if err != nil {
		s.log.Info("error getting pull request changes")
		return nil
	}

	found := false
	for _, change := range changes {
		s.log.Debug("considering change to file: " + change.Filename)
		if strings.HasPrefix(change.Filename, "cluster/ci/origin/jjb/") {
			s.log.Debug("file looks applicable")
			found = true
			break
		} else {
			s.log.Debug("file does not look applicable")
		}
	}
	if !found {
		s.log.Debug("did not find applicable changed file")
		return nil
	}

	startClone := time.Now()
	s.log.Info("cloning " + org + "/" + repo)
	r, err := s.gc.Clone(org + "/" + repo)
	if err != nil {
		s.log.Info("error cloning")
		return err
	}
	defer func() {
		if err := r.Clean(); err != nil {
			s.log.WithError(err).Error("Error cleaning up repo.")
		}
	}()

	s.log.Info("checking out " + pr.Head.SHA)
	if err = r.Checkout(pr.Head.SHA); err != nil {
		return err
	}
	s.log.WithField("duration", time.Since(startClone)).Info("Cloned and checked out target branch.")

	startMake := time.Now()

	cmd := exec.Command("/bin/bash", r.Dir+"/cluster/ci/origin/jjb/make.sh")
	s.log.Info("Running: /bin/bash " + r.Dir + "/cluster/ci/origin/jjb/make.sh")
	out, err := cmd.CombinedOutput()
	s.log.Info("output: " + string(out[:]))

	if err != nil {
		return err
	}
	s.log.WithField("duration", time.Since(startMake)).Info("Ran make.sh")

	return nil
}
