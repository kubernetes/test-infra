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
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
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
	plugins.RegisterGenericCommentHandler(pluginName, handleGenericComment, helpProvider)
}

func helpProvider(config *plugins.Configuration, _ []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	// The Config field is omitted because this plugin is not configurable.
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The yuks plugin comments with jokes in response to the `/joke` command.",
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/joke",
		Description: "Tells a joke.",
		Featured:    false,
		WhoCanUse:   "Anyone can use the `/joke` command.",
		Examples:    []string{"/joke"},
	})
	return pluginHelp, nil
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

func handleGenericComment(pc plugins.Agent, e github.GenericCommentEvent) error {
	return handle(pc.GitHubClient, pc.Logger, &e, jokeURL)
}

// escapeMarkdown takes a string and returns a serialized version of it such that all the symbols
// are treated as text instead of Markdown syntax. It escapes the symbols using numeric character
// references with the decimal notation. See https://www.w3.org/TR/html401/charset.html#h-5.3.1
func escapeMarkdown(s string) string {
	var b bytes.Buffer
	for _, r := range s {
		// Check for simple characters as they are considered safe, otherwise we escape the rune.
		c := string(r)
		if simple.MatchString(c) {
			b.WriteString(c)
		} else {
			b.WriteString(fmt.Sprintf("&#%d;", r))
		}
	}
	return b.String()
}

func handle(gc githubClient, log *logrus.Entry, e *github.GenericCommentEvent, j joker) error {
	// Only consider new comments.
	if e.Action != github.GenericCommentActionCreated {
		return nil
	}
	// Make sure they are requesting a joke
	if !match.MatchString(e.Body) {
		return nil
	}

	org := e.Repo.Owner.Login
	repo := e.Repo.Name
	number := e.Number

	errorBudget := 5
	for i := 1; i <= errorBudget; i++ {
		resp, err := j.readJoke()
		if err != nil {
			log.WithError(err).Infof("failed to get joke. Retrying (attempt %d/%d)", i, errorBudget)
			continue
		}
		if resp == "" {
			log.Infof("joke is empty. Retrying (attempt %d/%d)", i, errorBudget)
			continue
		}

		sanitizedJoke := escapeMarkdown(resp)
		log.Infof("commenting with \"%s\".", sanitizedJoke)
		return gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, sanitizedJoke))
	}

	return fmt.Errorf("failed to get joke after %d attempts", errorBudget)
}
