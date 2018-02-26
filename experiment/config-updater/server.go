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

// config-updater watches for merged PRs which update a set of files
// and update the corresponding files in a given deployment
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"

	"k8s.io/test-infra/prow/git"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/hook"
	"k8s.io/test-infra/prow/plugins"
)

const pluginName = "config-updater"

type githubClient interface {
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
	CreateComment(org, repo string, number int, comment string) error
}

type UpdateConfig struct {
	Targets  []string  `json:"targets"`
	Matchers []Matcher `json:"matchers"`
}

type Matcher struct {
	Regex  regexp.Regexp `json:"regex"`
	Target string        `json:"target"`
}

// Load loads and parses the config at path.
func Load(path string) (*UpdateConfig, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading %s: %v", path, err)
	}
	nc := &UpdateConfig{}
	if err := yaml.Unmarshal(b, nc); err != nil {
		return nil, fmt.Errorf("error unmarshaling %s: %v", path, err)
	}
	return nc, nil
}

type result struct {
	command []string
	output  string
	err     error
}

// Server implements http.Handler. It validates incoming GitHub webhooks and
// then dispatches them to the appropriate plugins.
type Server struct {
	hmacSecret []byte

	gc  *git.Client
	ghc githubClient
	log *logrus.Entry

	configAgent *Agent
}

// NewServer returns new server
func NewServer(hmac []byte, gc *git.Client, ghc *github.Client, configAgent *Agent) *Server {
	return &Server{
		hmacSecret: hmac,

		gc:  gc,
		ghc: ghc,
		log: logrus.StandardLogger().WithField("plugin", pluginName),

		configAgent: configAgent,
	}
}

// ServeHTTP validates an incoming webhook and puts it into the event channel.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	eventType, eventGUID, payload, ok := hook.ValidateWebhook(w, r, s.hmacSecret)
	if !ok {
		s.log.Error("Failed to validate payload")
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
		s.log.Debugf("received an event of type %q but didn't ask for it", eventType)
		return nil
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
		return fmt.Errorf("error getting pull request changes: %v", err)
	}

	startClone := time.Now()
	s.log.Info("cloning " + org + "/" + repo)
	r, err := s.gc.Clone(org + "/" + repo)
	if err != nil {
		return fmt.Errorf("error cloning: %v", err)
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

	results := results{}
	tasks := [][]string{}

	updateConfig := s.configAgent.Config()
	for _, target := range updateConfig.Targets {
		for _, change := range changes {
			if change.Filename == target {
				args, err := determineTargetForConfig(r.Dir, change.Filename)
				if err != nil {
					results.internal = append(results.internal, err)
				} else {
					tasks = append(tasks, args)
				}
			}
		}
	}
	for _, matcher := range updateConfig.Matchers {
		for _, change := range changes {
			if matcher.Regex.MatchString(change.Filename) {
				tasks = append(tasks, []string{"/usr/bin/make", matcher.Target})
				break
			}
		}
	}

	for _, task := range tasks {
		startAction := time.Now()
		cmd := exec.Command(task[0], task[1:]...)
		cmd.Dir = r.Dir
		out, err := cmd.CombinedOutput()
		s.log.WithFields(map[string]interface{}{
			"duration":  time.Since(startAction),
			"args":      task,
			"output":    out,
			"succeeded": err == nil,
		}).Info("Ran command")
		taskResult := result{task, string(out), err}

		if err != nil {
			results.failed = append(results.failed, taskResult)
		} else {
			results.succeeded = append(results.succeeded, taskResult)
		}
	}

	if len(results.succeeded) == 0 && len(results.failed) == 0 && len(results.internal) == 0 {
		return nil
	}

	return s.ghc.CreateComment(
		org, repo, num,
		plugins.FormatResponseRaw(
			pre.PullRequest.Body,
			pre.PullRequest.HTMLURL,
			pre.PullRequest.User.Login,
			results.formatResults(),
		),
	)
}

func determineTargetForConfig(dir, config string) ([]string, error) {
	configFile := filepath.Join(dir, config)
	content, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("cannot read object YAML/JSON from %v", config)
	}
	object := map[interface{}]interface{}{}
	err = yaml.Unmarshal(content, &object)
	if err != nil {
		return nil, fmt.Errorf("cannot parse object YAML/JSON from %v", config)
	}
	objectType, ok := object["kind"]
	if !ok {
		return nil, fmt.Errorf("cannot access object kind from %v", config)
	}

	var makeTarget string
	switch objectType {
	case "Template":
		makeTarget = "applyTemplate"
	default:
		makeTarget = "apply"
	}
	return []string{"/usr/bin/make", makeTarget, fmt.Sprintf("WHAT=%s", config)}, nil
}

type results struct {
	succeeded []result
	failed    []result
	internal  []error
}

func (r *results) formatResults() string {
	var commentBuffer bytes.Buffer
	if len(r.succeeded) > 0 {
		commentBuffer.WriteString("The following updates succeeded:\n")
		commentBuffer.WriteString("<ul>")
		for _, task := range r.succeeded {
			commentBuffer.WriteString(formatDetails(task))
		}
		commentBuffer.WriteString("</ul>\n")
	}

	if len(r.failed) > 0 {
		commentBuffer.WriteString("The following updates failed:\n")
		commentBuffer.WriteString("<ul>")
		for _, task := range r.failed {
			commentBuffer.WriteString(formatDetails(task))
		}
		commentBuffer.WriteString("</ul>\n")
	}

	if len(r.internal) > 0 {
		commentBuffer.WriteString("The following internal errors occurred:\n")
		commentBuffer.WriteString("<ul>")
		for _, err := range r.internal {
			commentBuffer.WriteString(fmt.Sprintf(`<li>%v</li>`, err))
		}
		commentBuffer.WriteString("</ul>\n")
	}

	return commentBuffer.String()
}

func formatDetails(taskResult result) string {
	output := taskResult.output
	if taskResult.err != nil {
		output = fmt.Sprintf("%s\n%v", taskResult.output, taskResult.err)
	}

	args := strings.Join(taskResult.command, " ")

	return fmt.Sprintf(`<li><details><summary><code>%s</code></summary><pre><code>
$ %s
%s
</pre></code></details></li>`, args, args, output)
}
