/*
Copyright 2019 The Kubernetes Authors.

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

// Package goose adds goose images to an issue or PR in response to a /honk comment
package goose

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

var (
	match = regexp.MustCompile(`(?mi)^/(honk)\s*$`)
	honk  = &realGaggle{
		url: "https://api.unsplash.com/photos/random?query=goose",
	}
)

const (
	pluginName = "goose"
)

func init() {
	plugins.RegisterGenericCommentHandler(pluginName, handleGenericComment, helpProvider)
}

func helpProvider(config *plugins.Configuration, _ []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	yamlSnippet, err := plugins.CommentMap.GenYaml(&plugins.Configuration{
		Goose: plugins.Goose{
			KeyPath: "/etc/unsplash-api/honk.txt",
		},
	})
	if err != nil {
		logrus.WithError(err).Warnf("cannot generate comments for %s plugin", pluginName)
	}
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The goose plugin adds a goose image to an issue or PR in response to the `/honk` command.",
		Config: map[string]string{
			"": fmt.Sprintf("The goose plugin uses an api key for unsplash.com stored in %s.", config.Goose.KeyPath),
		},
		Snippet: yamlSnippet,
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/honk",
		Description: "Add a goose image to the issue or PR",
		Featured:    false,
		WhoCanUse:   "Anyone",
		Examples:    []string{"/honk"},
	})
	return pluginHelp, nil
}

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
}

type gaggle interface {
	readGoose() (string, error)
}

type realGaggle struct {
	url     string
	lock    sync.RWMutex
	update  time.Time
	key     string
	keyPath string
}

func (g *realGaggle) setKey(keyPath string, log *logrus.Entry) {
	g.lock.Lock()
	defer g.lock.Unlock()
	if !time.Now().After(g.update) {
		return
	}
	g.update = time.Now().Add(1 * time.Minute)
	if keyPath == "" {
		g.key = ""
		return
	}
	b, err := ioutil.ReadFile(keyPath)
	if err == nil {
		g.key = strings.TrimSpace(string(b))
		return
	}
	log.WithError(err).Errorf("failed to read key at %s", keyPath)
	g.key = ""
}

type gooseResult struct {
	ID     string   `json:"id"`
	Images imageSet `json:"urls"`
}

type imageSet struct {
	Raw     string `json:"raw"`
	Full    string `json:"full"`
	Regular string `json:"regular"`
	Small   string `json:"small"`
	Thumb   string `json:"thumb"`
}

func (gr gooseResult) Format() (string, error) {
	if gr.Images.Small == "" {
		return "", errors.New("empty image url")
	}
	img, err := url.Parse(gr.Images.Small)
	if err != nil {
		return "", fmt.Errorf("invalid image url %s: %v", gr.Images.Small, err)
	}

	return fmt.Sprintf("\n![goose image](%s)", img), nil
}

func (g *realGaggle) URL() string {
	g.lock.RLock()
	defer g.lock.RUnlock()
	uri := string(g.url)
	if g.key != "" {
		uri += "&client_id=" + url.QueryEscape(g.key)
	}
	return uri
}

func (g *realGaggle) readGoose() (string, error) {
	geese := make([]gooseResult, 1)
	uri := g.URL()
	resp, err := http.Get(uri)
	if err != nil {
		return "", fmt.Errorf("could not read goose from %s: %v", uri, err)
	}
	defer resp.Body.Close()
	if sc := resp.StatusCode; sc > 299 || sc < 200 {
		return "", fmt.Errorf("failing %d response from %s", sc, uri)
	}
	if err = json.NewDecoder(resp.Body).Decode(&geese[0]); err != nil {
		return "", err
	}
	if len(geese) < 1 {
		return "", fmt.Errorf("no geese in response from %s", uri)
	}
	a := geese[0]
	if a.Images.Small == "" {
		return "", fmt.Errorf("no image url in response from %s", uri)
	}
	// checking size, GitHub doesn't support big images
	toobig, err := github.ImageTooBig(a.Images.Small)
	if err != nil {
		return "", fmt.Errorf("could not validate image size %s: %v", a.Images.Small, err)
	} else if toobig {
		return "", fmt.Errorf("long goose is too long: %s", a.Images.Small)
	}
	return a.Format()
}

func handleGenericComment(pc plugins.Agent, e github.GenericCommentEvent) error {
	return handle(
		pc.GitHubClient,
		pc.Logger,
		&e,
		honk,
		func() { honk.setKey(pc.PluginConfig.Goose.KeyPath, pc.Logger) },
	)
}

func handle(gc githubClient, log *logrus.Entry, e *github.GenericCommentEvent, g gaggle, setKey func()) error {
	// Only consider new comments.
	if e.Action != github.GenericCommentActionCreated {
		return nil
	}
	// Make sure they are requesting a goose
	mat := match.FindStringSubmatch(e.Body)
	if mat == nil {
		return nil
	}

	// Now that we know this is a relevant event we can set the key.
	setKey()

	org := e.Repo.Owner.Login
	repo := e.Repo.Name
	number := e.Number

	for i := 0; i < 3; i++ {
		resp, err := g.readGoose()
		if err != nil {
			log.WithError(err).Error("Failed to get goose img")
			continue
		}
		return gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, resp))
	}

	msg := "Unable to find goose. Have you checked the garden?"
	if err := gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, msg)); err != nil {
		log.WithError(err).Error("Failed to leave comment")
	}

	return errors.New("could not find a valid goose image")
}
