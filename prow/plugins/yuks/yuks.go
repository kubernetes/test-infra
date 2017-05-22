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
	// Previously: https://tambal.azurewebsites.net/joke/random
	jokeUrl    = realJoke("https://icanhazdadjoke.com")
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

var client = http.Client{}

type jokeResult struct {
	Joke string `json:"joke"`
}

func (url realJoke) readJoke() (string, error) {
	req, err := http.NewRequest("GET", string(url), nil)
	if err != nil {
		return "", fmt.Errorf("Could not create request %s: %v", url, err)
	}
	req.Header.Add("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Could not read joke from %s: %v", url, err)
	}
	defer resp.Body.Close()
	var a jokeResult
	if err = json.NewDecoder(resp.Body).Decode(&a); err != nil {
		return "", err
	}
	if a.Joke == "" {
		return "", fmt.Errorf("result from %s did not contain a joke", url)
	}
	return a.Joke, nil
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
	return gc.CreateComment(org, repo, number, plugins.FormatICResponse(ic.Comment, resp))
}
