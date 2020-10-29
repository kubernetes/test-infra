/*
Copyright 2020 The Kubernetes Authors.

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

// Package trickortreat adds halloween images to an issue or PR in response to a /trick-or-treat comment
package trickortreat

import (
	"errors"
	"math/rand"
	"regexp"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

var (
	match        = regexp.MustCompile(`(?mi)^/trick(\-)?or(\-)?treat(?: (.+))?\s*$`)
	trickOrTreat realSnicker
)

const (
	pluginName = "trick-or-treat"
)

func init() {
	plugins.RegisterGenericCommentHandler(pluginName, handleGenericComment, helpProvider)
}

func helpProvider(config *plugins.Configuration, _ []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The trick-or-treat plugin adds a candy image to an issue or PR in response to the `/trick-or-treat` command.",
		Config:      map[string]string{},
		Snippet:     "",
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/trick(-)or(-)treat",
		Description: "Add a candy image to the issue or PR",
		Featured:    false,
		WhoCanUse:   "Anyone",
		Examples:    []string{"/trick-or-treat", "/trickortreat"},
	})
	return pluginHelp, nil
}

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
}

type snicker interface {
	readImage(*logrus.Entry) (string, error)
}

type realSnicker struct {
}

func (c *realSnicker) readImage(log *logrus.Entry) (string, error) {
	var imgURL string
	var err error
	var tooBig bool
	for i := 0; i < 3; i++ {
		imgIndex := rand.Intn(len(candiesImgs))
		imgURL = candiesImgs[imgIndex]
		// checking size, GitHub doesn't support big images
		tooBig, err = github.ImageTooBig(imgURL)
		if err == nil {
			if !tooBig {
				return imgURL, nil
			}
			err = errors.New("image too big")
		}
		log.WithError(err).Debugf("Failed to read image %q", imgURL)
	}
	return "", err
}

func handleGenericComment(pc plugins.Agent, e github.GenericCommentEvent) error {
	return handle(
		pc.GitHubClient,
		pc.Logger,
		&e,
		&trickOrTreat,
	)
}

func handle(gc githubClient, log *logrus.Entry, e *github.GenericCommentEvent, c snicker) error {
	// Only consider new comments.
	if e.Action != github.GenericCommentActionCreated {
		return nil
	}
	// Make sure they are requesting a cat
	if mat := match.FindStringSubmatch(e.Body); mat == nil {
		return nil
	}

	org := e.Repo.Owner.Login
	repo := e.Repo.Name
	number := e.Number
	interval := 200 * time.Microsecond
	for i := 0; i < 3; i++ {
		imgURL, err := c.readImage(log)
		if err != nil {
			log.WithError(err).Error("Failed to get img")
			time.Sleep(interval)
			continue
		}
		return gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, imgURL))
	}

	return errors.New("could not find a valid candy image")
}
