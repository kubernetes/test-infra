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
	"fmt"
	"net/url"
)

// RemoteResolverFactory knows how to construct remote resolvers for
// authoritative central remotes (to pull from) and publish remotes
// (to push to) for a repository. These resolvers
type RemoteResolverFactory interface {
	CentralRemote(org, repo string) RemoteResolver
	PublishRemote(org, repo string) RemoteResolver
}

// RemoteResolver knows how to construct a remote URL for git calls
type RemoteResolver func() (string, error)

type sshRemoteResolverFactory struct {
	host     string
	username func() (login string, err error)
}

// CentralRemote creates a remote resolver that refers to an authoritative remote
// for the repository.
func (f *sshRemoteResolverFactory) CentralRemote(org, repo string) RemoteResolver {
	remote := fmt.Sprintf("git@%s:%s/%s.git", f.host, org, repo)
	return func() (string, error) {
		return remote, nil
	}
}

// PublishRemote creates a remote resolver that refers to a gitUser's remote
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

type simpleAuthResolverFactory struct {
	host     string
	username func() (login string, err error)
	token    func() []byte
}

// CentralRemote creates a remote resolver that refers to an authoritative remote
// for the repository.
func (f *simpleAuthResolverFactory) CentralRemote(org, repo string) RemoteResolver {
	return simpleAuthResolver(func() (*url.URL, error) {
		return &url.URL{Host: f.host, Path: fmt.Sprintf("%s/%s", org, repo)}, nil
	}, f.username, f.token)
}

// PublishRemote creates a remote resolver that refers to a gitUser's remote
// for the repository that can be published to.
func (f *simpleAuthResolverFactory) PublishRemote(_, repo string) RemoteResolver {
	return simpleAuthResolver(func() (*url.URL, error) {
		o, err := f.username()
		if err != nil {
			return nil, err
		}
		return &url.URL{Host: f.host, Path: fmt.Sprintf("%s/%s", o, repo)}, nil
	}, f.username, f.token)
}

// simpleAuthResolver builds URLs with simple auth credentials, resolved dynamically.
func simpleAuthResolver(remote func() (*url.URL, error), username func() (string, error), token func() []byte) RemoteResolver {
	return func() (string, error) {
		remote, err := remote()
		if err != nil {
			return "", fmt.Errorf("could not resolve remote: %v", err)
		}
		name, err := username()
		if err != nil {
			return "", fmt.Errorf("could not resolve username: %v", err)
		}
		remote.User = url.UserPassword(name, string(token()))
		return remote.String(), nil
	}
}
