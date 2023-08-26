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
	"os"
	"os/exec"
	"path"
	"runtime"
	"sync"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	utilpointer "k8s.io/utils/pointer"
)

// ClientFactory knows how to create clientFactory for repos
type ClientFactory interface {
	// ClientFromDir creates a client that operates on a repo that has already
	// been cloned to the given directory.
	ClientFromDir(org, repo, dir string) (RepoClient, error)
	// ClientFor creates a client that operates on a new clone of the repo.
	ClientFor(org, repo string) (RepoClient, error)
	// ClientForWithRepoOpts is like ClientFor, but allows you to customize the
	// setup of the cloned repo (such as sparse checkouts instead of using the
	// default full clone).
	ClientForWithRepoOpts(org, repo string, repoOpts RepoOpts) (RepoClient, error)

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

type ClientFactoryOpts struct {
	// Host, defaults to "github.com" if unset
	Host string
	// UseSSH, defaults to false
	UseSSH *bool
	// The directory in which the cache should be
	// created. Defaults to the "/var/tmp" on
	// Linux and os.TempDir otherwise
	CacheDirBase *string
	// If unset, publishing action will error
	Username LoginGetter
	// If unset, publishing action will error
	Token TokenGetter
	// The git user to use.
	GitUser GitUserGetter
	// The censor to use. Not needed for anonymous
	// actions.
	Censor Censor
	// Path to the httpCookieFile that will be used to authenticate client
	CookieFilePath string
	// If set, cacheDir persist. Otherwise temp dir will be used for CacheDir
	Persist *bool
}

// These options are scoped to the repo, not the ClientFactory level. The reason
// for the separation is to allow a single process to have for example repos
// that are both sparsely checked out and non-sparsely checked out.
type RepoOpts struct {
	// sparseCheckoutDirs is the list of directories that the working tree
	// should have. If non-nil and empty, then the working tree only has files
	// reachable from the root. If non-nil and non-empty, then those additional
	// directories from the root are also checked out (populated) in the working
	// tree, recursively.
	SparseCheckoutDirs []string
	// This is the `--share` flag to `git clone`. For cloning from a local
	// source, it allows bypassing the copying of all objects.
	ShareObjectsWithSourceRepo bool
	// FetchCommits list only those commit SHAs which are needed. If the commit
	// already exists, it is not fetched to save network costs. If FetchCommits
	// is set, we do not call RemoteUpdate() for the primary clone (git cache).
	FetchCommits sets.Set[string]

	// NoFetchTags determines whether we disable fetching tag objects). Defaults
	// to false (tag objects are fetched).
	NoFetchTags bool
	// PrimaryCloneUpdateCommits are any additional SHAs we need to fetch into
	// the primary clone to bring it up to speed. This should at least be the
	// current base branch SHA. Needed when we're using shared objects because
	// otherwise the primary will slowly get stale with no updates to it after
	// its initial creation.
	PrimaryCloneUpdateCommits sets.Set[string]
}

// Apply allows to use a ClientFactoryOpts as Opt
func (cfo *ClientFactoryOpts) Apply(target *ClientFactoryOpts) {
	if cfo.Host != "" {
		target.Host = cfo.Host
	}
	if cfo.UseSSH != nil {
		target.UseSSH = cfo.UseSSH
	}
	if cfo.CacheDirBase != nil {
		target.CacheDirBase = cfo.CacheDirBase
	}
	if cfo.Token != nil {
		target.Token = cfo.Token
	}
	if cfo.GitUser != nil {
		target.GitUser = cfo.GitUser
	}
	if cfo.Censor != nil {
		target.Censor = cfo.Censor
	}
	if cfo.Username != nil {
		target.Username = cfo.Username
	}
	if cfo.CookieFilePath != "" {
		target.CookieFilePath = cfo.CookieFilePath
	}
	if cfo.Persist != nil {
		target.Persist = cfo.Persist
	}
}

func defaultTempDir() *string {
	switch runtime.GOOS {
	case "linux":
		return utilpointer.String("/var/tmp")
	default:
		return utilpointer.String("")
	}
}

// ClientFactoryOpts allows to manipulate the options for a ClientFactory
type ClientFactoryOpt func(*ClientFactoryOpts)

func defaultClientFactoryOpts(cfo *ClientFactoryOpts) {
	if cfo.Host == "" {
		cfo.Host = "github.com"
	}
	if cfo.CacheDirBase == nil {
		// If we do not have a place to put cache, put it in temp dir.
		cfo.CacheDirBase = defaultTempDir()
	}
	if cfo.Censor == nil {
		cfo.Censor = func(in []byte) []byte { return in }
	}
}

// NewClientFactory allows for the creation of repository clients. It uses github.com
// without authentication by default, if UseSSH then returns
// sshRemoteResolverFactory, and if CookieFilePath is provided then returns
// gerritResolverFactory(Assuming that git http.cookiefile is used only by
// Gerrit, this function needs to be updated if it turned out that this
// assumtpion is not correct.)
func NewClientFactory(opts ...ClientFactoryOpt) (ClientFactory, error) {
	o := ClientFactoryOpts{}
	defaultClientFactoryOpts(&o)
	for _, opt := range opts {
		opt(&o)
	}

	if o.CookieFilePath != "" {
		if output, err := exec.Command("git", "config", "--global", "http.cookiefile", o.CookieFilePath).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("unable to configure http.cookiefile.\nOutput: %s\nError: %w", string(output), err)
		}
	}

	var cacheDir string
	var err error
	// If we want to persist the Cache between runs, use the cacheDirBase as the cache. Otherwise make a temp dir.
	if o.Persist != nil && *o.Persist {
		cacheDir = *o.CacheDirBase
	} else if cacheDir, err = os.MkdirTemp(*o.CacheDirBase, "gitcache"); err != nil {
		return nil, err
	}

	var remote RemoteResolverFactory
	if o.UseSSH != nil && *o.UseSSH {
		remote = &sshRemoteResolverFactory{
			host:     o.Host,
			username: o.Username,
		}
	} else if o.CookieFilePath != "" {
		remote = &gerritResolverFactory{}
	} else {
		remote = &httpResolverFactory{
			host:     o.Host,
			username: o.Username,
			token:    o.Token,
		}
	}
	return &clientFactory{
		cacheDir:       cacheDir,
		cacheDirBase:   *o.CacheDirBase,
		remote:         remote,
		gitUser:        o.GitUser,
		censor:         o.Censor,
		masterLock:     &sync.Mutex{},
		repoLocks:      map[string]*sync.Mutex{},
		logger:         logrus.WithField("client", "git"),
		cookieFilePath: o.CookieFilePath,
	}, nil
}

// NewLocalClientFactory allows for the creation of repository clients
// based on a local filepath remote for testing
func NewLocalClientFactory(baseDir string, gitUser GitUserGetter, censor Censor) (ClientFactory, error) {
	cacheDir, err := os.MkdirTemp("", "gitcache")
	if err != nil {
		return nil, err
	}
	return &clientFactory{
		cacheDir:   cacheDir,
		remote:     &pathResolverFactory{baseDir: baseDir},
		gitUser:    gitUser,
		censor:     censor,
		masterLock: &sync.Mutex{},
		repoLocks:  map[string]*sync.Mutex{},
		logger:     logrus.WithField("client", "git"),
	}, nil
}

type clientFactory struct {
	remote         RemoteResolverFactory
	gitUser        GitUserGetter
	censor         Censor
	logger         *logrus.Entry
	cookieFilePath string

	// cacheDir is the root under which cached clones of repos are created
	cacheDir string
	// cacheDirBase is the basedir under which create tempdirs
	cacheDirBase string
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
	var remote RemoteResolverFactory
	remote = c.remote
	client := &repoClient{
		publisher: publisher{
			remotes: remotes{
				publishRemote: remote.PublishRemote(org, repo),
				centralRemote: remote.CentralRemote(org, repo),
			},
			executor: executor,
			info:     c.gitUser,
			logger:   logger,
		},
		interactor: interactor{
			dir:      dir,
			remote:   remote.CentralRemote(org, repo),
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

// ClientFor wraps around ClientForWithRepoOpts using the default RepoOpts{}
// (empty value). Originally, ClientFor was not a wrapper at all and did the
// work inside ClientForWithRepoOpts itself, but it did this without RepoOpts.
// When RepoOpts was created, we made ClientFor wrap around
// ClientForWithRepoOpts to preserve behavior of existing callers of ClientFor.
func (c *clientFactory) ClientFor(org, repo string) (RepoClient, error) {
	return c.ClientForWithRepoOpts(org, repo, RepoOpts{})
}

// ClientForWithRepoOpts returns a repository client for the specified repository.
// This function may take a long time if it is the first time cloning the repo.
// In that case, it must do a full git mirror clone. For large repos, this can
// take a while. Once that is done, it will do a git remote update (essentially
// git fetch) for the mirror clone, which will usually take at most a few
// seconds, before creating a secondary clone from this (updated) mirror.
//
// org and repo are used for determining where the repo is cloned, cloneURI
// overrides org/repo for cloning.
func (c *clientFactory) ClientForWithRepoOpts(org, repo string, repoOpts RepoOpts) (RepoClient, error) {
	cacheDir := path.Join(c.cacheDir, org, repo)
	c.logger.WithFields(logrus.Fields{"org": org, "repo": repo, "dir": cacheDir}).Debug("Creating a client from the cache.")
	cacheClientCacher, _, _, err := c.bootstrapClients(org, repo, cacheDir)
	if err != nil {
		return nil, err
	}

	// Put copies of the repo in temp dir.
	repoDir, err := os.MkdirTemp(*defaultTempDir(), "gitrepo")
	if err != nil {
		return nil, err
	}
	_, repoClientCloner, repoClient, err := c.bootstrapClients(org, repo, repoDir)
	if err != nil {
		return nil, err
	}

	// First create or update the primary clone (in "cacheDir").
	c.ensureFreshPrimary(cacheDir, cacheClientCacher, repoOpts)

	// Initialize the new derivative repo (secondary clone) from the primary
	// clone. This is a local clone operation.
	if err := repoClientCloner.CloneWithRepoOpts(cacheDir, repoOpts); err != nil {
		return nil, err
	}

	// Here we do a targeted fetch, if the repoOpts for the secondary clone
	// asked for it.
	//
	// TODO(listx): Change ensureCommits() so that it takes a "cacher" interface
	// instead of a generic RepoClient. However, if ShareObjectsWithSourceRepo
	// was not set, then we cannot run ensureCommits() with the cacher because
	// the cacher will run on the primary, not secondary (and without --shared,
	// this will have no effect on the secondary); we have to handle that case
	// gracefully (perhaps with some logging also).
	if repoOpts.FetchCommits.Len() > 0 {
		// Targeted fetch. Only fetch those commits which we want, and only
		// if they are missing.
		if err := repoClientCloner.FetchCommits(repoOpts.NoFetchTags, repoOpts.FetchCommits.UnsortedList()); err != nil {
			return nil, err
		}
	}

	return repoClient, nil
}

func (c *clientFactory) ensureFreshPrimary(cacheDir string, cacheClientCacher cacher, repoOpts RepoOpts) error {
	// Protect access to the shared repoLocks map.
	var repoLock *sync.Mutex
	c.masterLock.Lock()
	if _, exists := c.repoLocks[cacheDir]; exists {
		repoLock = c.repoLocks[cacheDir]
	} else {
		repoLock = &sync.Mutex{}
		c.repoLocks[cacheDir] = repoLock
	}
	c.masterLock.Unlock()

	repoLock.Lock()
	defer repoLock.Unlock()
	if _, err := os.Stat(path.Join(cacheDir, "HEAD")); os.IsNotExist(err) {
		// we have not yet cloned this repo, we need to do a full clone
		if err := os.MkdirAll(cacheDir, os.ModePerm); err != nil && !os.IsExist(err) {
			return err
		}
		if err := cacheClientCacher.MirrorClone(); err != nil {
			return err
		}
	} else if err != nil {
		// something unexpected happened
		return err
	} else {
		// We have cloned the repo previously, but will refresh it. By default
		// we refresh all refs with a call to `git remote update`.
		//
		// This is the default behavior if FetchCommits is empty or nil (i.e.,
		// when we don't define a targeted list of commits to fetch directly).
		if repoOpts.FetchCommits.Len() == 0 {
			if err := cacheClientCacher.RemoteUpdate(); err != nil {
				return err
			}
		} else if repoOpts.PrimaryCloneUpdateCommits.Len() > 0 {
			if err := cacheClientCacher.FetchCommits(repoOpts.NoFetchTags, repoOpts.PrimaryCloneUpdateCommits.UnsortedList()); err != nil {
				return err
			}
		}
	}

	return nil
}

// Clean removes the caches used to generate clients
func (c *clientFactory) Clean() error {
	return os.RemoveAll(c.cacheDir)
}
