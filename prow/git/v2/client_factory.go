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
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sync"

	"github.com/sirupsen/logrus"
)

// ClientFactory knows how to create clientFactory for repos
type ClientFactory interface {
	// RepoFromDir creates a client that operates on a repo that has already
	// been cloned to the given directory.
	RepoFromDir(org, repo, dir string) (RepoClient, error)
	// Repo creates a client that operates on a new clone of the repo.
	Repo(org, repo string) (RepoClient, error)
}

// RepoClient exposes interactions with a git repo
type RepoClient interface {
	Publisher
	Interactor
}

type repoClient struct {
	publisher
	interactor
}

func NewClientFactory(host string, useSSH bool, username func() (login string, err error), token func() []byte, gitUser func() (name, email string, err error), censor func(content []byte) []byte) ClientFactory {
	var remotes RemoteResolverFactory
	if useSSH {
		remotes = &sshRemoteResolverFactory{
			host:     host,
			username: username,
		}
	} else {
		remotes = &simpleAuthResolverFactory{
			host:     host,
			username: username,
			token:    token,
		}
	}
	return &clientFactory{
		remotes: remotes,
		gitUser: gitUser,
		censor:  censor,
		logger:  logrus.WithField("client", "git"),
	}
}

type clientFactory struct {
	remotes RemoteResolverFactory
	gitUser func() (name, email string, err error)
	censor  func(content []byte) []byte
	logger  *logrus.Entry

	// cacheDir is the root under which cached clones of repos are created
	cacheDir string
	// masterLock guards mutations to the repoLocks records
	masterLock *sync.Mutex
	// repoLocks guard mutating access to subdirectories under the cacheDir
	repoLocks map[string]*sync.Mutex
}

// RepoFromDir returns a repository client for a directory that's already initialized with content.
// If the directory isn't specified, the current working directory is used.
func (c *clientFactory) RepoFromDir(org, repo, dir string) (RepoClient, error) {
	if dir == "" {
		workdir, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		dir = workdir
	}
	logger := c.logger.WithFields(logrus.Fields{"org": org, "repo": repo})
	executor, err := NewCensoringExecutor(dir, c.censor, logger)
	if err != nil {
		return nil, err
	}
	return &repoClient{
		publisher: publisher{
			remote:   c.remotes.PublishRemote(org, repo),
			executor: executor,
			info:     c.gitUser,
		},
		interactor: interactor{
			remote:   c.remotes.CentralRemote(org, repo),
			executor: executor,
			logger:   logger,
		},
	}, nil
}

// Repo returns a repository client for the specified repository.
// This function may take a long time if it is the first time cloning the repo.
// In that case, it must do a full git mirror clone. For large repos, this can
// take a while. Once that is done, it will do a git fetch instead of a clone,
// which will usually take at most a few seconds.
func (c *clientFactory) Repo(org, repo string) (RepoClient, error) {
	cacheDir := path.Join(c.cacheDir, org, repo)
	cacheClient, err := c.RepoFromDir(org, repo, cacheDir)
	if err != nil {
		return nil, err
	}

	repoDir, err := ioutil.TempDir("", "git")
	if err != nil {
		return nil, err
	}
	repoClient, err := c.RepoFromDir(org, repo, repoDir)
	if err != nil {
		return nil, err
	}
	c.masterLock.Lock()
	if _, exists := c.repoLocks[cacheDir]; !exists {
		c.repoLocks[cacheDir] = &sync.Mutex{}
	}
	c.masterLock.Unlock()
	c.repoLocks[cacheDir].Lock()
	defer c.repoLocks[cacheDir].Unlock()
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		// we have not yet cloned this repo, we need to do a full clone
		if err := os.MkdirAll(filepath.Dir(cacheDir), os.ModePerm); err != nil && !os.IsExist(err) {
			return nil, err
		}
		if err := cacheClient.MirrorClone(); err != nil {
			return nil, err
		}
	} else if err != nil {
		// something unexpected happened
		return nil, err
	} else {
		// we have cloned the repo previously, but will refresh it by fetching
		if err := cacheClient.Fetch(); err != nil {
			return nil, err
		}
	}

	// initialize the new derivative repo from the cache
	if err := repoClient.Clone(cacheDir); err != nil {
		return nil, err
	}

	return repoClient, nil
}
