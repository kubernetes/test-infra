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

package git

import (
	"errors"
	"fmt"
	"net/url"
	"path"

	gerritsource "k8s.io/test-infra/prow/gerrit/source"
)

// RemoteResolverFactory knows how to construct remote resolvers for
// authoritative central remotes (to pull from) and publish remotes
// (to push to) for a repository. These resolvers are called at run-time
// to determine remotes for git commands.
type RemoteResolverFactory interface {
	// CentralRemote returns a resolver for a remote server with an
	// authoritative version of the repository. This type of remote
	// is useful for fetching refs and cloning.
	CentralRemote(org, repo string) RemoteResolver
	// PublishRemote returns a resolver for a remote server with a
	// personal fork of the central repository. This type of remote is most
	// useful for publishing local changes.
	PublishRemote(org, centralRepo string) ForkRemoteResolver
}

// RemoteResolver knows how to construct a remote URL for git calls
type RemoteResolver func() (string, error)

// ForkRemoteResolver knows how to construct a remote URL for git calls
// It accepts a fork name since this may be different than the parent
// repo name. If the forkName is "", the parent repo name is assumed.
type ForkRemoteResolver func(forkName string) (string, error)

// LoginGetter fetches a GitHub login on-demand
type LoginGetter func() (login string, err error)

// TokenGetter fetches a GitHub OAuth token on-demand
type TokenGetter func(org string) (string, error)

type sshRemoteResolverFactory struct {
	host     string
	username LoginGetter
}

// CentralRemote creates a remote resolver that refers to an authoritative remote
// for the repository.
func (f *sshRemoteResolverFactory) CentralRemote(org, repo string) RemoteResolver {
	remote := fmt.Sprintf("git@%s:%s/%s.git", f.host, org, repo)
	return func() (string, error) {
		return remote, nil
	}
}

// PublishRemote creates a remote resolver that refers to a user's remote
// for the repository that can be published to.
func (f *sshRemoteResolverFactory) PublishRemote(_, centralRepo string) ForkRemoteResolver {
	return func(forkName string) (string, error) {
		repo := centralRepo
		if forkName != "" {
			repo = forkName
		}
		org, err := f.username()
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("git@%s:%s/%s.git", f.host, org, repo), nil
	}
}

type httpResolverFactory struct {
	// Whether to use HTTP.
	http bool
	host string
	// Optional, either both or none must be set
	username LoginGetter
	token    TokenGetter
}

// CentralRemote creates a remote resolver that refers to an authoritative remote
// for the repository.
func (f *httpResolverFactory) CentralRemote(org, repo string) RemoteResolver {
	return func() (string, error) {
		return f.resolve(org, repo)
	}
}

// PublishRemote creates a remote resolver that refers to a user's remote
// for the repository that can be published to.
func (f *httpResolverFactory) PublishRemote(_, centralRepo string) ForkRemoteResolver {
	return func(forkName string) (string, error) {
		// For the publsh remote we use:
		// - the user login rather than the central org
		// - the forkName rather than the central repo name, if specified.
		repo := centralRepo
		if forkName != "" {
			repo = forkName
		}
		if f.username == nil {
			return "", errors.New("username not configured, no publish repo available")
		}
		org, err := f.username()
		if err != nil {
			return "", fmt.Errorf("could not resolve username: %w", err)
		}
		remote, err := f.resolve(org, repo)
		if err != nil {
			err = fmt.Errorf("could not resolve remote: %w", err)
		}
		return remote, err
	}
}

// resolve builds the URL string for the given org/repo remote identifier, it
// respects the configured scheme, and the dynamic username and credentials.
func (f *httpResolverFactory) resolve(org, repo string) (string, error) {
	scheme := "https"
	if f.http {
		scheme = "http"
	}
	remote := &url.URL{Scheme: scheme, Host: f.host, Path: fmt.Sprintf("%s/%s", org, repo)}

	if f.username != nil {
		name, err := f.username()
		if err != nil {
			return "", fmt.Errorf("could not resolve username: %w", err)
		}
		token, err := f.token(org)
		if err != nil {
			return "", fmt.Errorf("could not resolve token: %w", err)
		}
		remote.User = url.UserPassword(name, token)
	}

	return remote.String(), nil
}

// pathResolverFactory generates resolvers for local path-based repositories,
// used in local integration testing only
type pathResolverFactory struct {
	baseDir string
}

// CentralRemote creates a remote resolver that refers to an authoritative remote
// for the repository.
func (f *pathResolverFactory) CentralRemote(org, repo string) RemoteResolver {
	return func() (string, error) {
		return path.Join(f.baseDir, org, repo), nil
	}
}

// PublishRemote creates a remote resolver that refers to a user's remote
// for the repository that can be published to.
func (f *pathResolverFactory) PublishRemote(org, centralRepo string) ForkRemoteResolver {
	return func(_ string) (string, error) {
		return path.Join(f.baseDir, org, centralRepo), nil
	}
}

// gerritResolverFactory is meant to be used by Gerrit only. It's so different
// from GitHub that there is no way any of the remotes logic can be shared
// between these two providers. The resulting CentralRemote and PublishRemote
// are both the clone URI.
type gerritResolverFactory struct{}

func (f *gerritResolverFactory) CentralRemote(org, repo string) RemoteResolver {
	return func() (string, error) {
		return gerritsource.CloneURIFromOrgRepo(org, repo), nil
	}
}

func (f *gerritResolverFactory) PublishRemote(org, repo string) ForkRemoteResolver {
	return func(_ string) (string, error) {
		return gerritsource.CloneURIFromOrgRepo(org, repo), nil
	}
}
