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

// Package cat adds cat images to issues in response to a /meow comment
package cat

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

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

var (
	match = regexp.MustCompile(`(?mi)^/meow(vie)?(?: (.+))?\s*$`)
	meow  = &realClowder{
		url: "https://api.thecatapi.com/api/images/get?format=json&results_per_page=1",
	}
)

const (
	pluginName = "cat"
)

func init() {
	plugins.RegisterGenericCommentHandler(pluginName, handleGenericComment, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	// The Config field is omitted because this plugin is not configurable.
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The cat plugin adds a cat image to an issue in response to the `/meow` command.",
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/meow(vie) [CATegory]",
		Description: "Add a cat image to the issue",
		Featured:    false,
		WhoCanUse:   "Anyone",
		Examples:    []string{"/meow", "/meow caturday", "/meowvie clothes"},
	})
	return pluginHelp, nil
}

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
}

type clowder interface {
	readCat(string, bool) (string, error)
}

type realClowder struct {
	url     string
	lock    sync.RWMutex
	update  time.Time
	key     string
	keyPath string
}

func (c *realClowder) setKey(keyPath string, log *logrus.Entry) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if !time.Now().After(c.update) {
		return
	}
	c.update = time.Now().Add(1 * time.Minute)
	if keyPath == "" {
		c.key = ""
		return
	}
	b, err := ioutil.ReadFile(keyPath)
	if err == nil {
		c.key = strings.TrimSpace(string(b))
		return
	}
	log.WithError(err).Errorf("failed to read key at %s", keyPath)
	c.key = ""
}

type catResult struct {
	Source string `json:"source_url"`
	Image  string `json:"url"`
}

func (cr catResult) Format() (string, error) {
	if cr.Source == "" {
		return "", errors.New("empty source_url")
	}
	if cr.Image == "" {
		return "", errors.New("empty image url")
	}
	src, err := url.Parse(cr.Source)
	if err != nil {
		return "", fmt.Errorf("invalid source_url %s: %v", cr.Source, err)
	}
	img, err := url.Parse(cr.Image)
	if err != nil {
		return "", fmt.Errorf("invalid image url %s: %v", cr.Image, err)
	}

	return fmt.Sprintf("[![cat image](%s)](%s)", img, src), nil
}

func (r *realClowder) Url(category string, movieCat bool) string {
	r.lock.RLock()
	defer r.lock.RUnlock()
	uri := string(r.url)
	if category != "" {
		uri += "&category=" + url.QueryEscape(category)
	}
	if r.key != "" {
		uri += "&api_key=" + url.QueryEscape(r.key)
	}
	if movieCat {
		uri += "&mime_types=gif"
	}
	return uri
}

func (r *realClowder) readCat(category string, movieCat bool) (string, error) {
	uri := r.Url(category, movieCat)
	resp, err := http.Get(uri)
	if err != nil {
		return "", fmt.Errorf("could not read cat from %s: %v", uri, err)
	}
	defer resp.Body.Close()
	if sc := resp.StatusCode; sc > 299 || sc < 200 {
		return "", fmt.Errorf("failing %d response from %s", sc, uri)
	}
	cats := make([]catResult, 0)
	if err = json.NewDecoder(resp.Body).Decode(&cats); err != nil {
		return "", err
	}
	if len(cats) < 1 {
		return "", fmt.Errorf("no cats in response from %s", uri)
	}
	a := cats[0]
	if a.Image == "" {
		return "", fmt.Errorf("no image url in response from %s", uri)
	}
	// checking size, GitHub doesn't support big images
	toobig, err := github.ImageTooBig(a.Image)
	if err != nil {
		return "", fmt.Errorf("could not validate image size %s: %v", a.Image, err)
	} else if toobig {
		return "", fmt.Errorf("longcat is too long: %s", a.Image)
	}
	return a.Format()
}

func handleGenericComment(pc plugins.Agent, e github.GenericCommentEvent) error {
	return handle(
		pc.GitHubClient,
		pc.Logger,
		&e,
		meow,
		func() { meow.setKey(pc.PluginConfig.Cat.KeyPath, pc.Logger) },
	)
}

func handle(gc githubClient, log *logrus.Entry, e *github.GenericCommentEvent, c clowder, setKey func()) error {
	// Only consider new comments.
	if e.Action != github.GenericCommentActionCreated {
		return nil
	}
	// Make sure they are requesting a cat
	mat := match.FindStringSubmatch(e.Body)
	if mat == nil {
		return nil
	}

	category, movieCat, err := parseMatch(mat)
	if err != nil {
		return err
	}

	// Now that we know this is a relevant event we can set the key.
	setKey()

	org := e.Repo.Owner.Login
	repo := e.Repo.Name
	number := e.Number

	for i := 0; i < 3; i++ {
		resp, err := c.readCat(category, movieCat)
		if err != nil {
			log.WithError(err).Error("Failed to get cat img")
			continue
		}
		return gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, resp))
	}

	var msg string
	if category != "" {
		msg = "Bad category. Please see https://api.thecatapi.com/api/categories/list"
	} else {
		msg = "https://thecatapi.com appears to be down"
	}
	if err := gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, msg)); err != nil {
		log.WithError(err).Error("Failed to leave comment")
	}

	return errors.New("could not find a valid cat image")
}

func parseMatch(mat []string) (string, bool, error) {
	if len(mat) != 3 {
		err := fmt.Errorf("expected 3 capture groups in regexp match, but got %d", len(mat))
		return "", false, err
	}
	category := strings.TrimSpace(mat[2])
	movieCat := len(mat[1]) > 0 // "vie" suffix is present.
	return category, movieCat, nil
}
