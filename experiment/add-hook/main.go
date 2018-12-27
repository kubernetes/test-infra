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

package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config/secret"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"
)

type options struct {
	flagutil.GitHubOptions
	repo     flagutil.Strings
	hookURL  string
	hmacPath string
	confirm  bool
	events   flagutil.Strings
}

func (o options) githubClient() (*github.Client, error) {
	agent := &secret.Agent{}
	if err := agent.Start([]string{o.TokenPath}); err != nil {
		return nil, fmt.Errorf("start %s: %v", o.TokenPath, err)
	}
	return o.GitHubClient(agent, !o.confirm)
}

func getOptions(fs *flag.FlagSet, args []string) (*options, error) {
	o := options{}
	o.AddFlags(fs)
	o.events = flagutil.NewStrings(github.AllHookEvents...)
	fs.Var(&o.events, "event", "Receive hooks for the following events, defaults to [\"*\"] (all events)")
	fs.Var(&o.repo, "repo", "Add hooks for this org or org/repo")
	fs.StringVar(&o.hookURL, "hook-url", "", "URL to send hooks")
	fs.StringVar(&o.hmacPath, "hmac-path", "", "Path to hmac secret")
	fs.BoolVar(&o.confirm, "confirm", false, "Apply changes to github")
	fs.Parse(args)
	if o.hmacPath == "" {
		return nil, errors.New("--hmac-path must be set")
	}
	if o.hookURL == "" {
		return nil, errors.New("--hook-url must be set")
	}
	if len(o.repo.Strings()) == 0 {
		return nil, errors.New("no --repos set")
	}
	if err := o.Validate(!o.confirm); err != nil {
		return nil, err
	}
	return &o, nil
}

func (o options) hmac() (string, error) {
	b, err := ioutil.ReadFile(o.hmacPath)
	if err != nil {
		return "", fmt.Errorf("read %s: %v", o.hmacPath, err)
	}
	return string(bytes.TrimSpace(b)), nil
}

func findHook(hooks []github.Hook, url string) *int {
	for _, h := range hooks {
		if h.Config.URL == url {
			return &h.ID
		}
	}
	return nil
}

type changer struct {
	lister  func(org string) ([]github.Hook, error)
	editor  func(org string, id int, req github.HookRequest) error
	creator func(org string, req github.HookRequest) (int, error)
}

func orgChanger(client *github.Client) changer {
	return changer{
		lister:  client.ListOrgHooks,
		editor:  client.EditOrgHook,
		creator: client.CreateOrgHook,
	}
}

func repoChanger(client *github.Client, repo string) changer {
	return changer{
		lister: func(org string) ([]github.Hook, error) {
			return client.ListRepoHooks(org, repo)
		},
		editor: func(org string, id int, req github.HookRequest) error {
			return client.EditRepoHook(org, repo, id, req)
		},
		creator: func(org string, req github.HookRequest) (int, error) {
			return client.CreateRepoHook(org, repo, req)
		},
	}
}

func main() {
	o, err := getOptions(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Args[1:])
	if err != nil {
		logrus.Fatalf("Bad flags: %v", err)
	}

	client, err := o.githubClient()
	if err != nil {
		logrus.Fatalf("Could not create github client: %v", err)
	}

	hmac, err := o.hmac()
	if err != nil {
		logrus.Fatalf("Could not load hmac secret: %v", err)
	}

	yes := true
	j := "json"
	req := github.HookRequest{
		Name:   "web",
		Active: &yes,
		Config: &github.HookConfig{
			URL:         o.hookURL,
			ContentType: &j,
			Secret:      &hmac,
		},
		Events: o.events.Strings(),
	}
	for _, orgRepo := range o.repo.Strings() {
		parts := strings.SplitN(orgRepo, "/", 2)
		var ch changer
		if len(parts) == 1 {
			ch = orgChanger(client)
		} else {
			ch = repoChanger(client, parts[1])
		}

		org := parts[0]
		if err := reconcileHook(ch, org, req); err != nil {
			logrus.Fatalf("Could not apply hook to %s: %v", orgRepo, err)
		}
	}
}

func reconcileHook(ch changer, org string, req github.HookRequest) error {
	hooks, err := ch.lister(org)
	if err != nil {
		return fmt.Errorf("list: %v", err)
	}
	id := findHook(hooks, req.Config.URL)
	if id == nil {
		_, err := ch.creator(org, req)
		return err
	}
	return ch.editor(org, *id, req)
}
