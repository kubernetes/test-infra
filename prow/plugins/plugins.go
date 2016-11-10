/*
Copyright 2016 The Kubernetes Authors.

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

package plugins

import (
	"fmt"
	"io/ioutil"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/ghodss/yaml"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/jobs"
	"k8s.io/test-infra/prow/kube"
)

var (
	allPlugins           = map[string]struct{}{}
	issueCommentHandlers = map[string]IssueCommentHandler{}
	pullRequestHandlers  = map[string]PullRequestHandler{}
	statusEventHandlers  = map[string]StatusEventHandler{}
)

type IssueCommentHandler func(PluginClient, github.IssueCommentEvent) error

func RegisterIssueCommentHandler(name string, fn IssueCommentHandler) {
	allPlugins[name] = struct{}{}
	issueCommentHandlers[name] = fn
}

type PullRequestHandler func(PluginClient, github.PullRequestEvent) error

func RegisterPullRequestHandler(name string, fn PullRequestHandler) {
	allPlugins[name] = struct{}{}
	pullRequestHandlers[name] = fn
}

type PluginClient struct {
	GitHubClient *github.Client
	KubeClient   *kube.Client
	JobAgent     *jobs.JobAgent
	Logger       *logrus.Entry
}

type StatusEventHandler func(*PluginAgent, github.StatusEvent) error

func RegisterStatusEventHandler(name string, fn StatusEventHandler) {
	allPlugins[name] = struct{}{}
	statusEventHandlers[name] = fn
}

type PluginAgent struct {
	PluginClient

	mut sync.Mutex
	// Repo (eg "k/k") -> list of handler names.
	ps map[string][]string
}

// Load attempts to load config from the path. It returns an error if either
// the file can't be read or it contains an unknown plugin.
func (pa *PluginAgent) Load(path string) error {
	pa.mut.Lock()
	defer pa.mut.Unlock()
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	np := map[string][]string{}
	if err := yaml.Unmarshal(b, &np); err != nil {
		return err
	}
	for _, v := range np {
		for _, p := range v {
			if _, ok := allPlugins[p]; !ok {
				return fmt.Errorf("unknown plugin: %s", p)
			}
		}
	}
	pa.ps = np
	return nil
}

// Start starts polling path for plugin config. If the first attempt fails,
// then start returns the error. Future errors will halt updates but not stop.
func (pa *PluginAgent) Start(path string) error {
	if err := pa.Load(path); err != nil {
		return err
	}
	ticker := time.Tick(1 * time.Minute)
	go func() {
		for range ticker {
			if err := pa.Load(path); err != nil {
				logrus.WithField("path", path).WithError(err).Error("Error loading plugin config.")
			}
		}
	}()
	return nil
}

// IssueCommentHandlers returns a map of plugin names to handlers for the repo.
func (pa *PluginAgent) IssueCommentHandlers(repo string) map[string]IssueCommentHandler {
	pa.mut.Lock()
	defer pa.mut.Unlock()

	hs := map[string]IssueCommentHandler{}
	for _, p := range pa.ps[repo] {
		if h, ok := issueCommentHandlers[p]; ok {
			hs[p] = h
		}
	}
	return hs
}

// PullRequestHandlers returns a map of plugin names to handlers for the repo.
func (pa *PluginAgent) PullRequestHandlers(repo string) map[string]PullRequestHandler {
	pa.mut.Lock()
	defer pa.mut.Unlock()

	hs := map[string]PullRequestHandler{}
	for _, p := range pa.ps[repo] {
		if h, ok := pullRequestHandlers[p]; ok {
			hs[p] = h
		}
	}
	return hs
}

// IssueCommentHandlers returns a map of plugin names to handlers for the repo.
func (pa *PluginAgent) StatusEventHandlers(repo string) map[string]StatusEventHandler {
	pa.mut.Lock()
	defer pa.mut.Unlock()

	hs := map[string]StatusEventHandler{}
	for _, p := range pa.ps[repo] {
		if h, ok := statusEventHandlers[p]; ok {
			hs[p] = h
		}
	}
	return hs
}
