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

package ghhook

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config/secret"

	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"
)

type Options struct {
	GitHubOptions    flagutil.GitHubOptions
	GitHubHookClient github.HookClient

	Repos        flagutil.Strings
	HookURL      string
	HMACValue    string
	HMACPath     string
	Events       flagutil.Strings
	ShouldDelete bool
	Confirm      bool
}

func (o *Options) Validate() error {
	if !o.ShouldDelete && o.HMACPath == "" && o.HMACValue == "" {
		return errors.New("either '--hmac-path' or '--hmac-value' must be specified (only one of them)")
	}
	if !o.ShouldDelete && o.HMACValue != "" && o.HMACPath != "" {
		return errors.New("both '--hmac-path' and '--hmac-value' can not be set at the same time")
	}
	if o.HookURL == "" {
		return errors.New("--hook-url must be set")
	}
	if len(o.Repos.Strings()) == 0 {
		return errors.New("no --repos set")
	}

	o.GitHubOptions.AllowDirectAccess = true
	var err error
	if err = o.GitHubOptions.Validate(!o.Confirm); err != nil {
		return err
	}

	return nil
}

func GetOptions(fs *flag.FlagSet, args []string) (*Options, error) {
	o := Options{}
	o.GitHubOptions.AddFlags(fs)
	o.Events = flagutil.NewStrings(github.AllHookEvents...)
	fs.Var(&o.Events, "event", "Receive hooks for the following events, defaults to [\"*\"] (all events)")
	fs.Var(&o.Repos, "repo", "Add hooks for this org or org/repos")
	fs.StringVar(&o.HookURL, "hook-url", "", "URL to send hooks")
	fs.StringVar(&o.HMACPath, "hmac-path", "", "Path to hmac secret")
	fs.StringVar(&o.HMACValue, "hmac-value", "", "hmac secret value")
	fs.BoolVar(&o.Confirm, "confirm", false, "Apply changes to github")
	fs.BoolVar(&o.ShouldDelete, "delete-webhook", false, "Webhook should be deleted")
	fs.Parse(args)

	var err error
	if err = o.Validate(); err != nil {
		return nil, err
	}

	agent := &secret.Agent{}
	// If it's not using the default github token path, start the secret agent.
	// TODO: check if the token path is empty instead, after the DefaultGitHubTokenPath is deprecated.
	if o.GitHubOptions.TokenPath != flagutil.DefaultGitHubTokenPath {
		if err := agent.Start([]string{o.GitHubOptions.TokenPath}); err != nil {
			return nil, fmt.Errorf("error starting secret agent %s: %v", o.GitHubOptions.TokenPath, err)
		}
	}
	o.GitHubHookClient, err = o.GitHubOptions.GitHubClient(agent, !o.Confirm)
	if err != nil {
		return nil, fmt.Errorf("error creating github client: %v", err)
	}

	return &o, nil
}

func (o *Options) hmacValueFromFile() (string, error) {
	b, err := ioutil.ReadFile(o.HMACPath)
	if err != nil {
		return "", fmt.Errorf("read %s: %v", o.HMACPath, err)
	}
	return string(bytes.TrimSpace(b)), nil
}

func (o *Options) HandleWebhookConfigChange() error {
	var hmac string
	var err error
	// hmac is only needed when we add or edit a webhook
	if !o.ShouldDelete {
		hmac, err = o.hmacValue()
		if err != nil {
			return fmt.Errorf("could not load hmac secret: %v", err)
		}
	}

	yes := true
	j := "json"
	req := github.HookRequest{
		Name:   "web",
		Active: &yes,
		Config: &github.HookConfig{
			URL:         o.HookURL,
			ContentType: &j,
			Secret:      &hmac,
		},
		Events: o.Events.Strings(),
	}
	for _, orgRepo := range o.Repos.Strings() {
		parts := strings.SplitN(orgRepo, "/", 2)
		var ch changer
		if len(parts) == 1 {
			ch = orgChanger(o.GitHubHookClient)
		} else {
			repo := parts[1]
			ch = repoChanger(o.GitHubHookClient, repo)
		}

		org := parts[0]
		if err := reconcileHook(ch, org, req, o); err != nil {
			return fmt.Errorf("could not apply hook to %s: %v", orgRepo, err)
		}
	}
	return nil
}

func reconcileHook(ch changer, org string, req github.HookRequest, o *Options) error {
	hooks, err := ch.lister(org)
	if err != nil {
		return fmt.Errorf("list: %v", err)
	}
	id := findHook(hooks, req.Config.URL)
	if id == nil {
		if o.ShouldDelete {
			logrus.Warnf("The webhook for %q does not exist, skip deletion", req.Config.URL)
			return nil
		}
		_, err := ch.creator(org, req)
		return err
	}
	if o.ShouldDelete {
		return ch.deletor(org, *id, req)
	}
	return ch.editor(org, *id, req)
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
	deletor func(org string, id int, req github.HookRequest) error
}

func orgChanger(client github.HookClient) changer {
	return changer{
		lister:  client.ListOrgHooks,
		editor:  client.EditOrgHook,
		creator: client.CreateOrgHook,
		deletor: client.DeleteOrgHook,
	}
}

func repoChanger(client github.HookClient, repo string) changer {
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
		deletor: func(org string, id int, req github.HookRequest) error {
			return client.DeleteRepoHook(org, repo, id, req)
		},
	}
}

func (o *Options) hmacValue() (string, error) {
	if o.HMACValue != "" {
		return o.HMACValue, nil
	}
	hmac, err := o.hmacValueFromFile()
	return hmac, err
}
