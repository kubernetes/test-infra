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
	"bytes"
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
	"k8s.io/test-infra/prow/plugins"
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

type ActionConfig struct {
	Actions []Action `json:"actions"`
}

type Action struct {
	Prefix     string `json:"prefix"`
	MakeTarget string `json:"action"`
}

type result struct {
	action Action
	output string
	err    error
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

	actionConfig ActionConfig
}

// NewServer returns new server
func NewServer(name, creds string, hmac []byte, gc *git.Client, ghc *github.Client, config ActionConfig) *Server {
	return &Server{
		hmacSecret:  hmac,
		credentials: creds,
		botName:     name,

		gc:  gc,
		ghc: ghc,
		log: logrus.StandardLogger().WithField("client", "jenkins-config-updater"),

		actionConfig: config,
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
		logrus.WithError(err).Error("Error handling event.")
	}
}

func (s *Server) handleEvent(eventType, eventGUID string, payload []byte) error {
	s.log.WithField("eventType", eventType).WithField("eventGUID", eventGUID).Info("Received webhook")
	if eventType != "pull_request" {
		return fmt.Errorf("received an event of type %q but didn't ask for it", eventType)
	}

	var pre github.PullRequestEvent
	if err := json.Unmarshal(payload, &pre); err != nil {
		return err
	}
	s.log = s.log.WithFields(map[string]interface{}{
		"org":    pre.Repo.Owner.Login,
		"repo":   pre.Repo.Name,
		"pr":     pre.Number,
		"author": pre.PullRequest.User.Login,
		"url":    pre.PullRequest.HTMLURL,
	})

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

	completedTasks := []result{}
	failedTasks := []result{}
	for _, action := range s.actionConfig.Actions {
		logger := s.log.WithField("target", action.MakeTarget)
		found := false
		for _, change := range changes {
			logger.Debug("considering change to file: " + change.Filename)
			if strings.HasPrefix(change.Filename, action.Prefix) {
				logger.Debug("file looks applicable")
				found = true
				break
			} else {
				logger.Debug("file does not look applicable")
			}
		}
		if !found {
			logger.Debug("did not find applicable changed file")
			return nil
		}

		logger.Info("Running action")
		startAction := time.Now()
		cmd := exec.Command("/usr/bin/make", action.MakeTarget)
		cmd.Dir = r.Dir
		out, err := cmd.CombinedOutput()
		logger.Info("output: " + string(out[:]))
		logger.WithField("duration", time.Since(startAction)).Info("Ran action")
		taskResult := result{action, string(out), err}

		if err != nil {
			failedTasks = append(failedTasks, taskResult)
		} else {
			completedTasks = append(completedTasks, taskResult)
		}
	}

	if len(completedTasks) == 0 && len(failedTasks) == 0 {
		return nil
	}

	var commentBuffer bytes.Buffer
	if len(completedTasks) > 0 {
		commentBuffer.WriteString("The following updates succeeded:\n")
		commentBuffer.WriteString("<ul>\n")
		for _, task := range completedTasks {
			commentBuffer.WriteString(formatDetails(task))
		}
		commentBuffer.WriteString("</ul>\n")
	}

	if len(failedTasks) > 0 {
		commentBuffer.WriteString("The following updates failed:\n")
		commentBuffer.WriteString("<ul>\n")
		for _, task := range failedTasks {
			commentBuffer.WriteString(formatDetails(task))
		}
		commentBuffer.WriteString("</ul>\n")
	}

	return s.ghc.CreateComment(
		org, repo, num,
		plugins.FormatResponseRaw(
			pre.PullRequest.Body,
			pre.PullRequest.HTMLURL,
			pre.PullRequest.User.Login,
			commentBuffer.String(),
		),
	)
}

func formatDetails(taskResult result) string {
	return fmt.Sprintf(`  <li>
    <details>
    <summary><code>make %s</code><summary>

    <pre><code>
    $ make %s
    %s
    %v
    </pre></code>

    </details>
  </li>`, taskResult.action.MakeTarget, taskResult.action.MakeTarget, taskResult.output, taskResult.err)
}
