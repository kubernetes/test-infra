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
	"sync"

	"github.com/sirupsen/logrus"
)

// ClientFactory knows how to create clientFactory for repos
type ClientFactory interface {
	// ClientFromDir creates a client that operates on a repo that has already
	// been cloned to the given directory.
	ClientFromDir(org, repo, dir string) (RepoClient, error)
	// ClientFor creates a client that operates on a new clone of the repo.
	ClientFor(org, repo string) (RepoClient, error)

	// Clean removes the caches used to generate clients
	Clean() error
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

// NewClientFactory allows for the creation of repository clients
func NewClientFactory(host string, useSSH bool, username LoginGetter, token TokenGetter, gitUser GitUserGetter, censor Censor) (ClientFactory, error) {
	cacheDir, err := ioutil.TempDir("", "gitcache")
	if err != nil {
		return nil, err
	}
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
		cacheDir:   cacheDir,
		remotes:    remotes,
		gitUser:    gitUser,
		censor:     censor,
		masterLock: &sync.Mutex{},
		repoLocks:  map[string]*sync.Mutex{},
		logger:     logrus.WithField("client", "git"),
	}, nil
}

// NewLocalClientFactory allows for the creation of repository clients
// based on a local filepath remote for testing
func NewLocalClientFactory(baseDir string, gitUser GitUserGetter, censor Censor) (ClientFactory, error) {
	cacheDir, err := ioutil.TempDir("", "gitcache")
	if err != nil {
		return nil, err
	}
	return &clientFactory{
		cacheDir:   cacheDir,
		remotes:    &pathResolverFactory{baseDir: baseDir},
		gitUser:    gitUser,
		censor:     censor,
		masterLock: &sync.Mutex{},
		repoLocks:  map[string]*sync.Mutex{},
		logger:     logrus.WithField("client", "git"),
	}, nil
}

type clientFactory struct {
	remotes RemoteResolverFactory
	gitUser GitUserGetter
	censor  Censor
	logger  *logrus.Entry

	// cacheDir is the root under which cached clones of repos are created
	cacheDir string
	// masterLock guards mutations to the repoLocks records
	masterLock *sync.Mutex
	// repoLocks guard mutating access to subdirectories under the cacheDir
	repoLocks map[string]*sync.Mutex
}

// bootstrapClients returns a repository client and cloner for a dir.
func (c *clientFactory) bootstrapClients(org, repo, dir string) (cacher, cloner, RepoClient, error) {
	if dir == "" {
		workdir, err := os.Getwd()
		if err != nil {
			return nil, nil, nil, err
		}
		dir = workdir
	}
	logger := c.logger.WithFields(logrus.Fields{"org": org, "repo": repo})
	logger.WithField("dir", dir).Debug("Creating a pre-initialized client.")
	executor, err := NewCensoringExecutor(dir, c.censor, logger)
	if err != nil {
		return nil, nil, nil, err
	}
	client := &repoClient{
		publisher: publisher{
			remote:   c.remotes.PublishRemote(org, repo),
			executor: executor,
			info:     c.gitUser,
		},
		interactor: interactor{
			dir:      dir,
			remote:   c.remotes.CentralRemote(org, repo),
			executor: executor,
			logger:   logger,
		},
	}
	return client, client, client, nil
}

// ClientFromDir returns a repository client for a directory that's already initialized with content.
// If the directory isn't specified, the current working directory is used.
func (c *clientFactory) ClientFromDir(org, repo, dir string) (RepoClient, error) {
	_, _, client, err := c.bootstrapClients(org, repo, dir)
	return client, err
}

// ClientFor returns a repository client for the specified repository.
// This function may take a long time if it is the first time cloning the repo.
// In that case, it must do a full git mirror clone. For large repos, this can
// take a while. Once that is done, it will do a git fetch instead of a clone,
// which will usually take at most a few seconds.
func (c *clientFactory) ClientFor(org, repo string) (RepoClient, error) {
	cacheDir := path.Join(c.cacheDir, org, repo)
	c.logger.WithFields(logrus.Fields{"org": org, "repo": repo, "dir": cacheDir}).Debug("Creating a client from the cache.")
	cacheClientCacher, _, _, err := c.bootstrapClients(org, repo, cacheDir)
	if err != nil {
		return nil, err
	}

	repoDir, err := ioutil.TempDir("", "gitrepo")
	if err != nil {
		return nil, err
	}
	_, repoClientCloner, repoClient, err := c.bootstrapClients(org, repo, repoDir)
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
		if err := os.MkdirAll(cacheDir, os.ModePerm); err != nil && !os.IsExist(err) {
			return nil, err
		}
		if err := cacheClientCacher.MirrorClone(); err != nil {
			return nil, err
		}
	} else if err != nil {
		// something unexpected happened
		return nil, err
	} else {
		// we have cloned the repo previously, but will refresh it
		if err := cacheClientCacher.RemoteUpdate(); err != nil {
			return nil, err
		}
	}

	// initialize the new derivative repo from the cache
	if err := repoClientCloner.Clone(cacheDir); err != nil {
		return nil, err
	}

	return repoClient, nil
}

// Clean removes the caches used to generate clients
func (c *clientFactory) Clean() error {
	return os.RemoveAll(c.cacheDir)
}
