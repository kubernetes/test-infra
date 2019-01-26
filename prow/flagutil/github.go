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

package flagutil

import (
	"flag"
	"fmt"
	"net/url"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config/secret"
	"k8s.io/test-infra/prow/git"
	"k8s.io/test-infra/prow/github"
)

// GitHubOptions holds options for interacting with GitHub.
type GitHubOptions struct {
	endpoint            Strings
	TokenPath           string
	deprecatedTokenFile string
}

// AddFlags injects GitHub options into the given FlagSet.
func (o *GitHubOptions) AddFlags(fs *flag.FlagSet) {
	o.endpoint = NewStrings("https://api.github.com")
	fs.Var(&o.endpoint, "github-endpoint", "GitHub's API endpoint (may differ for enterprise).")
	fs.StringVar(&o.TokenPath, "github-token-path", "/etc/github/oauth", "Path to the file containing the GitHub OAuth secret.")
	fs.StringVar(&o.deprecatedTokenFile, "github-token-file", "", "DEPRECATED: use -github-token-path instead.  -github-token-file may be removed anytime after 2019-01-01.")
}

// Validate validates GitHub options.
func (o *GitHubOptions) Validate(dryRun bool) error {
	for _, uri := range o.endpoint.Strings() {
		if _, err := url.ParseRequestURI(uri); err != nil {
			return fmt.Errorf("invalid -github-endpoint URI: %q", uri)
		}
	}

	if o.deprecatedTokenFile != "" {
		o.TokenPath = o.deprecatedTokenFile
		logrus.Error("-github-token-file is deprecated and may be removed anytime after 2019-01-01.  Use -github-token-path instead.")
	}

	if o.TokenPath == "" {
		logrus.Warn("empty -github-token-path, will use anonymous github client")
	}

	return nil
}

// GitHubClient returns a GitHub client.
func (o *GitHubOptions) GitHubClient(secretAgent *secret.Agent, dryRun bool) (client *github.Client, err error) {
	var generator *func() []byte
	if o.TokenPath == "" {
		generatorFunc := func() []byte {
			return []byte{}
		}
		generator = &generatorFunc
	} else {
		if secretAgent == nil {
			return nil, fmt.Errorf("cannot store token from %q without a secret agent", o.TokenPath)
		}
		generatorFunc := secretAgent.GetTokenGenerator(o.TokenPath)
		generator = &generatorFunc
	}

	if dryRun {
		return github.NewDryRunClient(*generator, o.endpoint.Strings()...), nil
	}
	return github.NewClient(*generator, o.endpoint.Strings()...), nil
}

// GitClient returns a Git client.
func (o *GitHubOptions) GitClient(secretAgent *secret.Agent, dryRun bool) (client *git.Client, err error) {
	client, err = git.NewClient()
	if err != nil {
		return nil, err
	}

	// We must capture the value of client here to prevent issues related
	// to the use of named return values when an error is encountered.
	// Without this, we risk a nil pointer dereference.
	defer func(client *git.Client) {
		if err != nil {
			client.Clean()
		}
	}(client)

	// Get the bot's name in order to set credentials for the Git client.
	githubClient, err := o.GitHubClient(secretAgent, dryRun)
	if err != nil {
		return nil, fmt.Errorf("error getting GitHub client: %v", err)
	}
	botName, err := githubClient.BotName()
	if err != nil {
		return nil, fmt.Errorf("error getting bot name: %v", err)
	}
	client.SetCredentials(botName, secretAgent.GetTokenGenerator(o.TokenPath))

	return client, nil
}
