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
	"bytes"
	"crypto/rsa"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"strings"

	jwt "github.com/dgrijalva/jwt-go/v4"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config/secret"
	"k8s.io/test-infra/prow/git"
	"k8s.io/test-infra/prow/github"
)

// GitHubOptions holds options for interacting with GitHub.
//
// Set AllowAnonymous to be true if you want to allow anonymous github access.
// Set AllowDirectAccess to be true if you want to suppress warnings on direct github access (without ghproxy).
type GitHubOptions struct {
	Host              string
	endpoint          Strings
	graphqlEndpoint   string
	TokenPath         string
	AllowAnonymous    bool
	AllowDirectAccess bool
	AppID             string
	AppPrivateKeyPath string

	// These two will only be set after a github client was retrieved for the first time
	client             github.Client
	appsTokenGenerator github.GitHubAppTokenGenerator
}

const DefaultGitHubTokenPath = "/etc/github/oauth" // Exported for testing purposes

// AddFlags injects GitHub options into the given FlagSet.
func (o *GitHubOptions) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.Host, "github-host", github.DefaultHost, "GitHub's default host (may differ for enterprise)")
	o.endpoint = NewStrings(github.DefaultAPIEndpoint)
	fs.Var(&o.endpoint, "github-endpoint", "GitHub's API endpoint (may differ for enterprise).")
	fs.StringVar(&o.graphqlEndpoint, "github-graphql-endpoint", github.DefaultGraphQLEndpoint, "GitHub GraphQL API endpoint (may differ for enterprise).")
	fs.StringVar(&o.TokenPath, "github-token-path", "", "Path to the file containing the GitHub OAuth secret.")
	fs.StringVar(&o.AppID, "github-app-id", "", "ID of the GitHub app. If set, requires --github-app-private-key-path to be set and --github-token-path to be unset.")
	fs.StringVar(&o.AppPrivateKeyPath, "github-app-private-key-path", "", "Path to the private key of the github app. If set, requires --github-app-id to bet set and --github-token-path to be unset")
}

// Validate validates GitHub options. Note that validate updates the GitHubOptions
// to add default valiues for TokenPath and graphqlEndpoint.
func (o *GitHubOptions) Validate(bool) error {
	endpoints := o.endpoint.Strings()
	for i, uri := range endpoints {
		if uri == "" {
			endpoints[i] = github.DefaultAPIEndpoint
		} else if _, err := url.ParseRequestURI(uri); err != nil {
			return fmt.Errorf("invalid -github-endpoint URI: %q", uri)
		}
	}

	if o.TokenPath != "" && (o.AppID != "" || o.AppPrivateKeyPath != "") {
		return fmt.Errorf("--token-path is mutually exclusive with --app-id and --app-private-key-path")
	}
	if o.AppID == "" != (o.AppPrivateKeyPath == "") {
		return errors.New("--app-id and --app-private-key-path must be set together")
	}

	if o.TokenPath == "" && o.AppID == "" && !o.AllowAnonymous {
		// TODO(fejta): just return error after May 2020
		logrus.Warnf("missing required flag: please set to --github-token-path=%s before June 2020", DefaultGitHubTokenPath)
		o.TokenPath = DefaultGitHubTokenPath
	}

	if o.TokenPath != "" && len(endpoints) == 1 && endpoints[0] == github.DefaultAPIEndpoint && !o.AllowDirectAccess {
		logrus.Warn("It doesn't look like you are using ghproxy to cache API calls to GitHub! This has become a required component of Prow and other components will soon be allowed to add features that may rapidly consume API ratelimit without caching. Starting May 1, 2020 use Prow components without ghproxy at your own risk! https://github.com/kubernetes/test-infra/tree/master/ghproxy#ghproxy")
	}

	if o.graphqlEndpoint == "" {
		o.graphqlEndpoint = github.DefaultGraphQLEndpoint
	} else if _, err := url.Parse(o.graphqlEndpoint); err != nil {
		return fmt.Errorf("invalid -github-graphql-endpoint URI: %q", o.graphqlEndpoint)
	}

	return nil
}

// GitHubClientWithLogFields returns a GitHub client with extra logging fields
func (o *GitHubOptions) GitHubClientWithLogFields(secretAgent *secret.Agent, dryRun bool, fields logrus.Fields) (github.Client, error) {
	if o.client != nil {
		return o.client.WithFields(fields), nil
	}
	var generator *func() []byte
	if o.TokenPath == "" {
		logrus.Warn("empty -github-token-path, will use anonymous github client")
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

	var appsGenerator func() *rsa.PrivateKey
	if o.AppPrivateKeyPath != "" {
		if secretAgent == nil {
			return nil, fmt.Errorf("cannot store token from %q without a secret agent", o.AppPrivateKeyPath)
		}
		if err := secretAgent.Add(o.AppPrivateKeyPath); err != nil {
			return nil, fmt.Errorf("failed to add the the key from --app-private-key-path to scret agent: %w", err)
		}
		appsGenerator = func() *rsa.PrivateKey {
			raw := secretAgent.GetTokenGenerator(o.AppPrivateKeyPath)()
			privateKey, err := jwt.ParseRSAPrivateKeyFromPEM(raw)
			// TODO alvaroaleman: Add hooks to the SecretAgent
			if err != nil {
				panic(fmt.Sprintf("failed to parse private key: %v", err))
			}
			return privateKey
		}

	}

	if dryRun {
		if o.AppPrivateKeyPath != "" {
			appsTokenGenerator, client := github.NewAppsAuthDryRunClientWithFields(fields, secretAgent.Censor, o.AppID, appsGenerator, o.graphqlEndpoint, o.endpoint.Strings()...)
			o.client = client
			o.appsTokenGenerator = appsTokenGenerator
			return o.client, nil
		}
		o.client = github.NewDryRunClientWithFields(fields, *generator, secretAgent.Censor, o.graphqlEndpoint, o.endpoint.Strings()...)
		return o.client, nil
	}
	if o.AppPrivateKeyPath != "" {
		appsTokenGenerator, client := github.NewAppsAuthClientWithFields(fields, secretAgent.Censor, o.AppID, appsGenerator, o.graphqlEndpoint, o.endpoint.Strings()...)
		o.client = client
		o.appsTokenGenerator = appsTokenGenerator
		return o.client, nil

	}

	o.client = github.NewClientWithFields(fields, *generator, secretAgent.Censor, o.graphqlEndpoint, o.endpoint.Strings()...)
	return o.client, nil
}

// GitHubClient returns a GitHub client.
func (o *GitHubOptions) GitHubClient(secretAgent *secret.Agent, dryRun bool) (github.Client, error) {
	return o.GitHubClientWithLogFields(secretAgent, dryRun, logrus.Fields{})
}

// GitHubClientWithAccessToken creates a GitHub client from an access token.
func (o *GitHubOptions) GitHubClientWithAccessToken(token string) github.Client {
	return github.NewClient(func() []byte { return []byte(token) }, func(content []byte) []byte {
		trimmedToken := strings.TrimSpace(token)
		if trimmedToken != token {
			token = trimmedToken
		}
		if token == "" {
			return content
		}
		return bytes.ReplaceAll(content, []byte(token), []byte("CENSORED"))
	}, o.graphqlEndpoint, o.endpoint.Strings()...)
}

// GitClient returns a Git client.
func (o *GitHubOptions) GitClient(secretAgent *secret.Agent, dryRun bool) (client *git.Client, err error) {
	client, err = git.NewClientWithHost(o.Host)
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

	user, generator, err := o.getGitAuthentication(secretAgent, dryRun)
	if err != nil {
		return nil, fmt.Errorf("failed to get git authentication: %w", err)
	}
	client.SetCredentials(user, generator)

	return client, nil
}

func (o *GitHubOptions) getGitAuthentication(secretAgent *secret.Agent, dryRun bool) (string, git.GitTokenGenerator, error) {
	githubClient, err := o.GitHubClient(secretAgent, dryRun)
	if err != nil {
		return "", nil, fmt.Errorf("error getting GitHub client: %v", err)
	}

	// Use Personal Access token auth
	if o.appsTokenGenerator == nil {
		botUser, err := githubClient.BotUser()
		if err != nil {
			return "", nil, fmt.Errorf("error getting bot name: %v", err)
		}
		generator := func(_ string) (string, error) {
			return string(secretAgent.GetTokenGenerator(o.TokenPath)()), nil
		}

		return botUser.Login, generator, nil
	}

	// Use github apps auth
	// https://docs.github.com/en/free-pro-team@latest/developers/apps/authenticating-with-github-apps#http-based-git-access-by-an-installation
	return "x-access-token", git.GitTokenGenerator(o.appsTokenGenerator), nil
}
