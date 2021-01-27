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
	"errors"
	"flag"

	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/github"
	utilpointer "k8s.io/utils/pointer"
)

// GitOptions holds options for interacting with git.
type GitOptions struct {
	host          string
	user          string
	email         string
	tokenPath     string
	useSSH        bool
	useGitHubUser bool
}

// AddFlags injects Git options into the given FlagSet.
func (o *GitOptions) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.host, "git-host", "github.com", "host to contact for git operations.")
	fs.StringVar(&o.user, "git-user", "", "User for git commits, optional. Can be derived from GitHub credentials.")
	fs.StringVar(&o.email, "git-email", "", "Email for git commits, optional. Can be derived from GitHub credentials.")
	fs.StringVar(&o.tokenPath, "git-token-path", "", "Path to the file containing the git token for HTTPS operations, optional. Can be derived from GitHub credentials.")
	fs.BoolVar(&o.useSSH, "git-over-ssh", false, "Use SSH when pushing and pulling instead of HTTPS. SSH credentials should be present at ~/.ssh")
	fs.BoolVar(&o.useGitHubUser, "git-user-from-github", true, "Use GitHub credentials and user identity for git operations.")
}

// Validate validates Git options.
func (o *GitOptions) Validate(dryRun bool) error {
	if o.host == "" {
		return errors.New("--git-host is required")
	}

	if !o.useGitHubUser {
		switch {
		case o.user == "":
			return errors.New("--git-user is required, or may be loaded by setting --git-user-from-github")
		case o.email == "":
			return errors.New("--git-email is required, or may be loaded by setting --git-user-from-github")
		}
	}

	if !o.useSSH && !o.useGitHubUser && o.tokenPath == "" {
		return errors.New("--git-token-path must be provided or defaulted by setting --git-user-from-github or --git-over-ssh")
	}
	return nil
}

// GitClient creates a new git client.
func (o *GitOptions) GitClient(userClient github.UserClient, token func() []byte, censor func(content []byte) []byte, dryRun bool) (git.ClientFactory, error) {
	gitUser := func() (name, email string, err error) {
		name, email = o.user, o.email
		if o.useGitHubUser {
			user, err := userClient.BotUser()
			if err != nil {
				return "", "", err
			}
			name = user.Name
			email = user.Email
		}
		return name, email, nil
	}
	username := func() (login string, err error) {
		user, err := userClient.BotUser()
		if err != nil {
			return "", err
		}
		return user.Login, nil
	}
	opts := git.ClientFactoryOpts{
		Host:     o.host,
		UseSSH:   utilpointer.BoolPtr(o.useSSH),
		Username: username,
		Token:    token,
		GitUser:  gitUser,
		Censor:   censor,
	}
	return git.NewClientFactory(opts.Apply)
}
