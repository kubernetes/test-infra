/*
Copyright 2018 The Kubernetes Authors.

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

// Package dog adds dog images to issues in response to a /woof comment
package dog

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

var (
	match     = regexp.MustCompile(`(?mi)^/(woof|bark)\s*$`)
	filetypes = regexp.MustCompile(`(?i)\.(jpg|gif|png)$`)
)

const (
	dogURL     = realPack("https://random.dog/woof.json")
	pluginName = "dog"
)

func init() {
	plugins.RegisterGenericCommentHandler(pluginName, handleGenericComment, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	// The Config field is omitted because this plugin is not configurable.
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The dog plugin adds a dog image to an issue in response to the `/woof` command.",
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/(woof|bark)",
		Description: "Add a dog image to the issue",
		Featured:    false,
		WhoCanUse:   "Anyone",
		Examples:    []string{"/woof", "/bark"},
	})
	return pluginHelp, nil
}

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
}

type pack interface {
	readDog() (string, error)
}

type realPack string

var client = http.Client{}

type dogResult struct {
	URL string `json:"url"`
}

func (dr dogResult) Format() (string, error) {
	if dr.URL == "" {
		return "", errors.New("empty url")
	}
	src, err := url.ParseRequestURI(dr.URL)
	if err != nil {
		return "", fmt.Errorf("invalid url %s: %v", dr.URL, err)
	}
	return fmt.Sprintf("[![dog image](%s)](%s)", src, src), nil
}

func (u realPack) readDog() (string, error) {
	uri := string(u)
	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return "", fmt.Errorf("could not create request %s: %v", uri, err)
	}
	req.Header.Add("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not read dog from %s: %v", uri, err)
	}
	defer resp.Body.Close()
	var a dogResult
	if err = json.NewDecoder(resp.Body).Decode(&a); err != nil {
		return "", err
	}
	// GitHub doesn't support videos :(
	if !filetypes.MatchString(a.URL) {
		return "", errors.New("unsupported doggo :( unknown filetype: " + a.URL)
	}
	return a.Format()
}

func handleGenericComment(pc plugins.PluginClient, e github.GenericCommentEvent) error {
	return handle(pc.GitHubClient, pc.Logger, &e, dogURL)
}

func handle(gc githubClient, log *logrus.Entry, e *github.GenericCommentEvent, p pack) error {
	// Only consider new comments.
	if e.Action != github.GenericCommentActionCreated {
		return nil
	}
	// Make sure they are requesting a dog
	mat := match.FindStringSubmatch(e.Body)
	if mat == nil {
		return nil
	}

	org := e.Repo.Owner.Login
	repo := e.Repo.Name
	number := e.Number

	for i := 0; i < 5; i++ {
		resp, err := p.readDog()
		if err != nil {
			log.WithError(err).Println("Failed to get dog img")
			continue
		}
		return gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, resp))
	}

	return errors.New("could not find a valid dog image")
}
