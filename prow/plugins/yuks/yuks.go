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

package yuks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

var (
	match  = regexp.MustCompile(`(?m)^@k8s-(ci|ro)?bot tell me a joke[,.?!]*\r?$`)
	simple = regexp.MustCompile(`^[\w?'!., ]+$`)
)

const (
	jokeUrl    = realJoke("https://tambal.azurewebsites.net/joke/random")
	pluginName = "yuks"
)

func init() {
	plugins.RegisterIssueCommentHandler(pluginName, handleIssueComment)
}

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
}

type joker interface {
	readJoke() (string, error)
}

type realJoke string

func (url realJoke) readJoke() (string, error) {
	resp, err := http.Get(string(url))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var a map[string]string
	if err = json.NewDecoder(resp.Body).Decode(&a); err != nil {
		return "", err
	}
	j, ok := a["joke"]
	if !ok {
		return "", fmt.Errorf("result does not contain a joke key: %v", a)
	}
	return j, nil
}

func handleIssueComment(pc plugins.PluginClient, ic github.IssueCommentEvent) error {
	return handle(pc.GitHubClient, pc.Logger, ic, jokeUrl)
}

func handle(gc githubClient, log *logrus.Entry, ic github.IssueCommentEvent, j joker) error {
	// Only consider new comments.
	if ic.Action != "created" {
		return nil
	}
	// Make sure they are requesting a joke
	if !match.MatchString(ic.Comment.Body) {
		return nil
	}

	org := ic.Repo.Owner.Login
	repo := ic.Repo.Name
	number := ic.Issue.Number

	resp, err := j.readJoke()
	if err != nil {
		return err
	}
	if !simple.MatchString(resp) {
		return fmt.Errorf("joke contains invalid characters: %v", resp)
	}
	log.Infof("Commenting with \"%s\".", resp)
	return gc.CreateComment(org, repo, number, plugins.FormatResponse(ic.Comment, resp))
}
