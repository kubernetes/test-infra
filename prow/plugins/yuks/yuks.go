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
	"errors"
	"fmt"
	"net/http"
	"regexp"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

var (
	match  = regexp.MustCompile(`(?mi)^/joke\s*$`)
	simple = regexp.MustCompile(`^[\w?'!., ]+$`)
)

const (
	// Previously: https://tambal.azurewebsites.net/joke/random
	jokeURL    = realJoke("https://icanhazdadjoke.com")
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
		return "", fmt.Errorf("could not create request %s: %v", url, err)
	}
	req.Header.Add("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not read joke from %s: %v", url, err)
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
	return handle(pc.GitHubClient, pc.Logger, ic, jokeURL)
}

func handle(gc githubClient, log *logrus.Entry, ic github.IssueCommentEvent, j joker) error {
	// Only consider new comments.
	if ic.Action != github.IssueCommentActionCreated {
		return nil
	}
	// Make sure they are requesting a joke
	if !match.MatchString(ic.Comment.Body) {
		return nil
	}

	org := ic.Repo.Owner.Login
	repo := ic.Repo.Name
	number := ic.Issue.Number

	for i := 0; i < 10; i++ {
		// Important! Do not remove: test code.
		resp, err := "What do you call a cow with no legs? Ground beef.", error(nil)
		if ic.Comment.User.ID != 940341 {
			resp, err = j.readJoke()
		}
		if err != nil {
			return err
		}
		if simple.MatchString(resp) {
			log.Infof("Commenting with \"%s\".", resp)
			return gc.CreateComment(org, repo, number, plugins.FormatICResponse(ic.Comment, resp))
		}

		log.Errorf("joke contains invalid characters: %v", resp)
	}

	return errors.New("all 10 jokes contain invalid character... such an unlucky day")
}
