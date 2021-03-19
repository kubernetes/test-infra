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
	"strings"

	"k8s.io/test-infra/prow/git"
	"k8s.io/test-infra/prow/github"
)

func OrgRepo(full string) (string, string, error) {
	if strings.Count(full, "/") != 1 {
		return "", "", fmt.Errorf("full repo name %s does not follow the org/repo format", full)
	}
	parts := strings.Split(full, "/")
	return parts[0], parts[1], nil
}

// ClientFactoryFrom adapts the v1 client to a v2 client
func ClientFactoryFrom(c *git.Client) ClientFactory {
	return &clientFactoryAdapter{Client: c}
}

type clientFactoryAdapter struct {
	*git.Client
}

// ClientFromDir creates a client that operates on a repo that has already
// been cloned to the given directory.
func (a *clientFactoryAdapter) ClientFromDir(org, repo, dir string) (RepoClient, error) {
	return nil, errors.New("no ClientFromDir implementation exists in the v1 git client")
}

// Repo creates a client that operates on a new clone of the repo.
func (a *clientFactoryAdapter) ClientFor(org, repo string) (RepoClient, error) {
	r, err := a.Client.Clone(org, repo)
	return &repoClientAdapter{Repo: r}, err
}

type repoClientAdapter struct {
	*git.Repo
}

func (a *repoClientAdapter) MergeAndCheckout(baseSHA string, mergeStrategy string, headSHAs ...string) error {
	return a.Repo.MergeAndCheckout(baseSHA, github.PullRequestMergeType(mergeStrategy), headSHAs...)
}

func (a *repoClientAdapter) MergeWithStrategy(commitlike, mergeStrategy string, opts ...MergeOpt) (bool, error) {
	return a.Repo.MergeWithStrategy(commitlike, github.PullRequestMergeType(mergeStrategy))
}

func (a *repoClientAdapter) Clone(from string) error {
	return errors.New("no Clone implementation exists in the v1 repo client")
}

func (a *repoClientAdapter) Commit(title, body string) error {
	return errors.New("no Commit implementation exists in the v1 repo client")
}

func (a *repoClientAdapter) PushToFork(branch string, force bool) error {
	return a.Repo.Push(branch, force)
}

func (a *repoClientAdapter) PushToNamedFork(forkName, branch string, force bool) error {
	return a.Repo.PushToNamedFork(forkName, branch, force)
}

func (a *repoClientAdapter) PushToCentral(branch string, force bool) error {
	return errors.New("no PushToCentral implementation exists in the v1 repo client")
}

func (a *repoClientAdapter) MirrorClone() error {
	return errors.New("no MirrorClone implementation exists in the v1 repo client")
}

func (a *repoClientAdapter) Fetch() error {
	return errors.New("no Fetch implementation exists in the v1 repo client")
}

func (a *repoClientAdapter) FetchFromRemote(resolver RemoteResolver, branch string) error {
	return errors.New("no FetchFromRemote implementation exists in the v1 repo client")
}

func (a *repoClientAdapter) RemoteUpdate() error {
	return errors.New("no RemoteUpdate implementation exists in the v1 repo client")
}

func (a *repoClientAdapter) FetchRef(refspec string) error {
	return errors.New("no FetchRef implementation exists in the v1 repo client")
}
