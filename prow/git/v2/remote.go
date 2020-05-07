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
	// personal fork of the repository. This type of remote is most
	// useful for publishing local changes.
	PublishRemote(org, repo string) RemoteResolver
}

// RemoteResolver knows how to construct a remote URL for git calls
type RemoteResolver func() (string, error)

// LoginGetter fetches a GitHub login on-demand
type LoginGetter func() (login string, err error)

// TokenGetter fetches a GitHub OAuth token on-demand
type TokenGetter func() []byte

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
func (f *sshRemoteResolverFactory) PublishRemote(_, repo string) RemoteResolver {
	return func() (string, error) {
		org, err := f.username()
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("git@%s:%s/%s.git", f.host, org, repo), nil
	}
}

type httpResolverFactory struct {
	host string
	// Optional, either both or none must be set
	username LoginGetter
	token    TokenGetter
}

// CentralRemote creates a remote resolver that refers to an authoritative remote
// for the repository.
func (f *httpResolverFactory) CentralRemote(org, repo string) RemoteResolver {
	return httpResolver(func() (*url.URL, error) {
		return &url.URL{Scheme: "https", Host: f.host, Path: fmt.Sprintf("%s/%s", org, repo)}, nil
	}, f.username, f.token)
}

// PublishRemote creates a remote resolver that refers to a user's remote
// for the repository that can be published to.
func (f *httpResolverFactory) PublishRemote(_, repo string) RemoteResolver {
	return httpResolver(func() (*url.URL, error) {
		if f.username == nil {
			return nil, errors.New("username not configured, no publish repo available")
		}
		o, err := f.username()
		if err != nil {
			return nil, err
		}
		return &url.URL{Scheme: "https", Host: f.host, Path: fmt.Sprintf("%s/%s", o, repo)}, nil
	}, f.username, f.token)
}

// httpResolverbuilds http URLs that may optionally contain simple auth credentials, resolved dynamically.
func httpResolver(remote func() (*url.URL, error), username LoginGetter, token TokenGetter) RemoteResolver {
	return func() (string, error) {
		remote, err := remote()
		if err != nil {
			return "", fmt.Errorf("could not resolve remote: %v", err)
		}

		if username != nil {
			name, err := username()
			if err != nil {
				return "", fmt.Errorf("could not resolve username: %v", err)
			}
			remote.User = url.UserPassword(name, string(token()))
		}

		return remote.String(), nil
	}
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
func (f *pathResolverFactory) PublishRemote(org, repo string) RemoteResolver {
	return func() (string, error) {
		return path.Join(f.baseDir, org, repo), nil
	}
}
