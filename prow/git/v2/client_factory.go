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
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	utilpointer "k8s.io/utils/pointer"
)

var gitMetrics = struct {
	ensureFreshPrimaryDuration *prometheus.HistogramVec
	fetchByShaDuration         *prometheus.HistogramVec
	secondaryCloneDuration     *prometheus.HistogramVec
	sparseCheckoutDuration     prometheus.Histogram
}{
	ensureFreshPrimaryDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "git_ensure_fresh_primary_duration",
		Help:    "Histogram of seconds spent ensuring that the primary is fresh, by org and repo.",
		Buckets: []float64{0.5, 1, 2, 5, 10, 20, 30, 45, 60, 90, 120, 180, 300, 450, 600, 750, 900, 1050, 1200},
	}, []string{
		"org", "repo",
	}),
	fetchByShaDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "git_fetch_by_sha_duration",
		Help:    "Histogram of seconds spent fetching commit SHAs, by org and repo.",
		Buckets: []float64{0.5, 1, 2, 5, 10, 20, 30, 45, 60, 90, 120, 180, 300, 450, 600, 750, 900, 1050, 1200},
	}, []string{
		"org", "repo",
	}),
	secondaryCloneDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "git_secondary_clone_duration",
		Help:    "Histogram of seconds spent creating the secondary clone, by org and repo.",
		Buckets: []float64{0.5, 1, 2, 5, 10, 20, 30, 45, 60, 90},
	}, []string{
		"org", "repo",
	}),
	sparseCheckoutDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "sparse_checkout_duration",
		Help:    "Histogram of seconds spent performing sparse checkout for a repository",
		Buckets: []float64{0.5, 1, 2, 5, 10, 20, 30, 45, 60, 90},
	}),
}

func init() {
	prometheus.MustRegister(gitMetrics.ensureFreshPrimaryDuration)
	prometheus.MustRegister(gitMetrics.fetchByShaDuration)
	prometheus.MustRegister(gitMetrics.secondaryCloneDuration)
	prometheus.MustRegister(gitMetrics.sparseCheckoutDuration)
}

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
	// Whether to use HTTP. By default, HTTPS is used (overrides UseSSH).
	//
	// TODO (listx): Combine HTTPS, HTTP, and SSH schemes into a single enum.
	UseInsecureHTTP *bool
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
	// source, it allows bypassing the copying of all objects. If this is true,
	// you must also set NeededCommits to a non-empty value; otherwise, when the
	// primary is updated with RemoteUpdate() the `--prune` flag may end up
	// deleting objects in the primary (which could adversely affect the
	// secondary).
	ShareObjectsWithPrimaryClone bool
	// NeededCommits list only those commit SHAs which are needed. If the commit
	// already exists, it is not fetched to save network costs. If NeededCommits
	// is set, we do not call RemoteUpdate() for the primary clone (git cache).
	NeededCommits sets.Set[string]
	// BranchesToRetarget contains a map of branch names mapped to SHAs. These
	// branch name and SHA pairs will be fed into RetargetBranch in the git v2
	// client, to update the current HEAD of each branch.
	BranchesToRetarget map[string]string
}

// Apply allows to use a ClientFactoryOpts as Opt
func (cfo *ClientFactoryOpts) Apply(target *ClientFactoryOpts) {
	if cfo.Host != "" {
		target.Host = cfo.Host
	}
	if cfo.UseInsecureHTTP != nil {
		target.UseInsecureHTTP = cfo.UseInsecureHTTP
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
			http:     o.UseInsecureHTTP != nil && *o.UseInsecureHTTP,
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
	if repoOpts.ShareObjectsWithPrimaryClone && repoOpts.NeededCommits.Len() == 0 {
		return nil, fmt.Errorf("programmer error: cannot share objects between primary and secondary without targeted fetches (NeededCommits)")
	}

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
	timeBeforeEnsureFreshPrimary := time.Now()
	err = c.ensureFreshPrimary(cacheDir, cacheClientCacher, repoOpts, org, repo)
	if err != nil {
		c.logger.WithFields(logrus.Fields{"org": org, "repo": repo, "dir": cacheDir}).Errorf("Error encountered while refreshing primary clone: %s", err.Error())
	} else {
		gitMetrics.ensureFreshPrimaryDuration.WithLabelValues(org, repo).Observe(time.Since(timeBeforeEnsureFreshPrimary).Seconds())
	}

	// Initialize the new derivative repo (secondary clone) from the primary
	// clone. This is a local clone operation.
	timeBeforeSecondaryClone := time.Now()
	if err = repoClientCloner.CloneWithRepoOpts(cacheDir, repoOpts); err != nil {
		return nil, err
	}
	gitMetrics.secondaryCloneDuration.WithLabelValues(org, repo).Observe(time.Since(timeBeforeSecondaryClone).Seconds())

	return repoClient, nil
}

func (c *clientFactory) ensureFreshPrimary(
	cacheDir string,
	cacheClientCacher cacher,
	repoOpts RepoOpts,
	org string,
	repo string,
) error {
	if err := c.maybeCloneAndUpdatePrimary(cacheDir, cacheClientCacher, repoOpts); err != nil {
		return err
	}
	// For targeted fetches by SHA objects, there's no need to hold a lock on
	// the primary because it's safe to do so (git will first write to a
	// temporary file and replace the file being written to, so if another git
	// process already wrote to it, the worst case is that it will overwrite the
	// file with the same data).  Targeted fetch. Only fetch those commits which
	// we want, and only if they are missing.
	if repoOpts.NeededCommits.Len() > 0 {
		// Targeted fetch. Only fetch those commits which we want, and only if
		// they are missing.
		timeBeforeFetchBySha := time.Now()
		if err := cacheClientCacher.FetchCommits(repoOpts.NeededCommits.UnsortedList()); err != nil {
			return err
		}
		gitMetrics.fetchByShaDuration.WithLabelValues(org, repo).Observe(time.Since(timeBeforeFetchBySha).Seconds())

		// Retarget branches. That is, make them point to a new SHA, so that the
		// branches can get updated, even though we only fetch by SHA above.
		//
		// Because the branches never get used directly here, it's OK if this
		// operation fails.
		for branch, sha := range repoOpts.BranchesToRetarget {
			if err := cacheClientCacher.RetargetBranch(branch, sha); err != nil {
				c.logger.WithFields(logrus.Fields{"org": org, "repo": repo, "dir": cacheDir, "branch": branch}).WithError(err).Debug("failed to retarget branch")
			}
		}
	}

	return nil
}

// maybeCloneAndUpdatePrimary clones the primary if it doesn't exist yet, and
// also runs a RemoteUpdate() against it if NeededCommits is empty. The
// operations in this function are protected by a lock so that only one thread
// can run at a given time for the same cacheDir (primary clone path).
func (c *clientFactory) maybeCloneAndUpdatePrimary(cacheDir string, cacheClientCacher cacher, repoOpts RepoOpts) error {
	// Protect access to the shared repoLocks map. The main point of all this
	// locking is to ensure that we only try to create the primary clone (if it
	// doesn't exist) in a serial manner.
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
	} else if repoOpts.NeededCommits.Len() == 0 {
		// We have cloned the repo previously, but will refresh it. By default
		// we refresh all refs with a call to `git remote update`.
		//
		// This is the default behavior if NeededCommits is empty or nil (i.e.,
		// when we don't define a targeted list of commits to fetch directly).
		//
		// This call to RemoteUpdate() still needs to be protected by a lock
		// because it updates possibly hundreds, if not thousands, of refs
		// (quite literally, files in .git/refs/*).
		if err := cacheClientCacher.RemoteUpdate(); err != nil {
			return err
		}
	}

	return nil
}

// Clean removes the caches used to generate clients
func (c *clientFactory) Clean() error {
	return os.RemoveAll(c.cacheDir)
}
