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

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"

	"github.com/sirupsen/logrus"
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
	agent := config.SecretAgent{}
	if err := agent.Start([]string{o.TokenPath}); err != nil {
		return nil, fmt.Errorf("start %s: %v", o.TokenPath, err)
	}
	return o.GitHubClient(&agent, !o.confirm)
}

func getOptions(fs *flag.FlagSet, args []string) (*options, error) {
	o := options{}
	o.AddFlags(fs)
	o.events = flagutil.NewStrings(github.AllHookEvents...)
	fs.Var(&o.events, "events", "Receive hooks for the following events, defaults to * for all events")
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
			return &h.Id
		}
	}
	return nil
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

	for _, orgRepo := range o.repo.Strings() {
		parts := strings.SplitN(orgRepo, "/", 2)
		var hooks []github.Hook
		if len(parts) == 1 {
			hooks, err = client.ListOrgHooks(orgRepo)
		} else {
			hooks, err = client.ListRepoHooks(parts[0], parts[1])
		}
		if err != nil {
			logrus.Fatalf("Failed to list %s hooks: %v", orgRepo, err)
		}
		id := findHook(hooks, o.hookURL)
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
		switch {
		case id == nil && len(parts) == 1:
			_, err = client.CreateOrgHook(parts[0], req)
		case id == nil:
			_, err = client.CreateRepoHook(parts[0], parts[1], req)
		case len(parts) == 1:
			err = client.EditOrgHook(parts[0], *id, req)
		default:
			err = client.EditRepoHook(parts[0], parts[1], *id, req)
		}
		if err != nil {
			logrus.Fatalf("Could not apply %s hook: %v", orgRepo, err)
		}
	}
}
