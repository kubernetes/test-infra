/*
Copyright 2017 The Kubernetes Authors.

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

// Package git provides a client to plugins that can do git operations.
package git

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type gitExecutor interface {
	run(args ...string) ([]byte, error)
	runWithRetries(retries int, args ...string) ([]byte, error)
	remote() string
}

func newLiteralGitExecutor(dir, git string, remoteURL *url.URL) gitExecutor {
	return &literalGitExecutor{
		logger: logrus.WithFields(logrus.Fields{
			"client": "git",
			"dir":    dir,
		}),
		dir:       dir,
		git:       git,
		remoteURL: remoteURL,
	}
}

type literalGitExecutor struct {
	// logger will be used to log git operations
	logger *logrus.Entry
	// rootCacheDir is the location of the git cache for this repo.
	dir string
	// git is the path to the git binary.
	git string
	// base is the base path for git clone calls. For users it will be set to
	// GitHub, but for tests set it to a directory with git repos.
	remoteURL *url.URL
}

// run will pass the args to git and run the command
func (e *literalGitExecutor) run(args ...string) ([]byte, error) {
	var b []byte
	var err error
	logger := e.logger.WithField("args", strings.Join(args, " "))
	c := exec.Command(e.git, args...)
	c.Dir = e.dir
	b, err = c.CombinedOutput()
	if err != nil {
		logger.WithError(err).Warningf("Running command returned error %v with output %s.", err, string(b))
	} else {
		logger.Info("Ran command.")
	}

	return b, err
}

// run will pass the args to git and run the command, retrying the specified
// number of times with an exponential backoff in between
func (e *literalGitExecutor) runWithRetries(retries int, args ...string) ([]byte, error) {
	var b []byte
	var err error
	sleepyTime := time.Second
	logger := e.logger.WithField("args", strings.Join(args, " "))
	for i := 0; i < retries; i++ {
		b, err = e.run(args...)
		if err != nil {
			logger.WithField("attempt", i).Warning("Running command failed, retrying...")
			time.Sleep(sleepyTime)
			sleepyTime *= 2
			continue
		}
		break
	}
	if err != nil {
		logger.Error("Failed to run command.")
	}
	return b, err
}

// remote exposes the remote URL for use in commands against the remote
func (e *literalGitExecutor) remote() string {
	return e.remoteURL.String()
}

type cacheInteractor interface {
	walk(func(baseDir string) filepath.WalkFunc) error
	operateOnFileData(path string, work func(contents []byte, err error) error) error
	cacheCleaner
	dir() string
}

type cacheCleaner interface {
	clean() error
}

type cacheDirInteractor struct {
	// cacheDir is the directory where a repository cache is cloned
	cacheDir string
}

// walk allows consumers to walk around in our files while having knowledge of
// our root rootCacheDir but not leaking that information out unless they really want to
func (i *cacheDirInteractor) walk(walkFn func(baseDir string) filepath.WalkFunc) error {
	return filepath.Walk(i.cacheDir, walkFn(i.cacheDir))
}

// operateOnFileData allows consumers to do work on the contents of a specific file
// in our repository without knowing where it is on disk
func (i *cacheDirInteractor) operateOnFileData(path string, work func(contents []byte, err error) error) error {
	return work(ioutil.ReadFile(filepath.Join(i.cacheDir, path)))
}

func (i *cacheDirInteractor) clean() error {
	return os.RemoveAll(i.cacheDir)
}

func (i *cacheDirInteractor) dir() string {
	return i.cacheDir
}

type cacheInitializer interface {
	initialize(orgRepo string) (bool, cacheInteractor, error)
	initializeClone() (cacheInteractor, error)
}

type cacheDirInitializer struct {
	// rootCacheDir is the root directory for the cache
	rootCacheDir string
}

// initialize ensures that the directory to be used by the cache exists
// and returns whether this is a fresh cache and an interactor for it
func (i *cacheDirInitializer) initialize(orgRepo string) (bool, cacheInteractor, error) {
	globalCache := filepath.Join(i.rootCacheDir, orgRepo)
	interactor := &cacheDirInteractor{cacheDir: globalCache}
	_, initialErr := os.Stat(globalCache)
	if os.IsNotExist(initialErr) {
		// We ignore this error since we can handle it
		initialErr = nil
		// Cache miss, clone it now.
		if createErr := os.MkdirAll(globalCache, os.ModePerm); createErr != nil && !os.IsExist(createErr) {
			return false, nil, createErr
		}
		return true, interactor, nil
	}
	return false, interactor, initialErr
}

// initializeClone creates a new cache for use for downstream copies of the
// shared cache
func (i *cacheDirInitializer) initializeClone() (cacheInteractor, error) {
	localCache, err := ioutil.TempDir("", "git")
	return &cacheDirInteractor{cacheDir: localCache}, err
}

// executorFor returns an executor for a cached repo
func executorFor(git string, remote *url.URL) func(orgRepo string, interactor cacheInteractor) gitExecutor {
	return func(orgRepo string, interactor cacheInteractor) gitExecutor {
		// do not copy seed remote URL
		newRemote := *remote
		newRemote.Path = fmt.Sprintf("%s/%s", newRemote.Path, orgRepo)
		return newLiteralGitExecutor(interactor.dir(), git, &newRemote)
	}
}

// Client can clone repos. It keeps a local cache, so successive clones of the
// same repo should be quick. Create with NewClient. Be sure to clean it up.
type Client struct {
	// logger will be used to log git operations and must be set.
	logger *logrus.Entry

	// initializer knows how to initialize cached repos
	initializer cacheInitializer

	// executorFactory knows how to create git executors
	executorFactory func(repo string, interactor cacheInteractor) gitExecutor

	// cleaner knows how to clean up the shared cache
	cleaner cacheCleaner

	// The mutex protects repoLocks which protect individual repos. This is
	// necessary because Clone calls for the same repo are racy. Rather than
	// one lock for all repos, use a lock per repo.
	// Lock with Client.lockRepo, unlock with Client.unlockRepo.
	rlm       sync.Mutex
	repoLocks map[string]*sync.Mutex
}

// Clean removes the local repo cache. The Client is unusable after calling.
func (c *Client) Clean() error {
	return c.cleaner.clean()
}

// NewClient returns a client that talks to GitHub. It will fail if git is not
// in the PATH.
func NewClient(remote *url.URL) (*Client, error) {
	git, err := exec.LookPath("git")
	if err != nil {
		return nil, err
	}
	cache, err := ioutil.TempDir("", "git")
	if err != nil {
		return nil, err
	}
	return &Client{
		logger:          logrus.WithField("client", "git"),
		executorFactory: executorFor(git, remote),
		initializer:     &cacheDirInitializer{rootCacheDir: cache},
		cleaner:         &cacheDirInteractor{cacheDir: cache},
		repoLocks:       make(map[string]*sync.Mutex),
	}, nil
}

func (c *Client) lockRepo(repo string) {
	c.rlm.Lock()
	if _, ok := c.repoLocks[repo]; !ok {
		c.repoLocks[repo] = &sync.Mutex{}
	}
	m := c.repoLocks[repo]
	c.rlm.Unlock()
	m.Lock()
}

func (c *Client) unlockRepo(repo string) {
	c.rlm.Lock()
	defer c.rlm.Unlock()
	c.repoLocks[repo].Unlock()
}

// Clone clones a repository. Pass the full repository name, such as
// "kubernetes/test-infra" as the repo.
// This function may take a long time if it is the first time cloning the repo.
// In that case, it must do a full git mirror clone. For large repos, this can
// take a while. Once that is done, it will do a git fetch instead of a clone,
// which will usually take at most a few seconds.
func (c *Client) Clone(repo string) (*Repo, error) {
	c.lockRepo(repo)
	defer c.unlockRepo(repo)

	// Ensure shared cache exists and is up-to-date
	initial, cache, err := c.initializer.initialize(repo)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize cache for repo %s: %v", repo, err)
	}

	executor := c.executorFactory(repo, cache)
	if initial {
		if b, err := executor.runWithRetries(3, "clone", "--mirror", executor.remote(), cache.dir()); err != nil {
			return nil, fmt.Errorf("git cache clone error: %v. output: %s", err, string(b))
		}
	}

	if b, err := executor.runWithRetries(3, "fetch"); err != nil {
		return nil, fmt.Errorf("git fetch error: %v. output: %s", err, string(b))
	}

	localCache, err := c.initializer.initializeClone()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize local cache for repo %s: %v", repo, err)
	}

	localExecutor := c.executorFactory(repo, localCache)
	if b, err := localExecutor.run("clone", cache.dir(), localCache.dir()); err != nil {
		return nil, fmt.Errorf("git repo clone error: %v. output: %s", err, string(b))
	}

	return &Repo{
		logger: c.logger.WithFields(logrus.Fields{
			"repo": repo,
		}),
		executor: localExecutor,
		cache:    localCache,
	}, nil
}

// Repo is a clone of a git repository. Create with Client.Clone, and don't
// forget to clean it up after.
type Repo struct {
	executor gitExecutor
	cache    cacheInteractor

	logger *logrus.Entry
}

// Clean deletes the repo. It is unusable after calling.
func (r *Repo) Clean() error {
	return r.cache.clean()
}

// Walk allows for downstream consumers to touch files in this
// repo while being aware of the base rootCacheDir without us exposing
// the directories directly.
func (r *Repo) Walk(input func(baseDir string) filepath.WalkFunc) error {
	return r.cache.walk(input)
}

// OperateOnFileData allows for downstream consumers to touch
// files in this repo while being aware of the base rootCacheDir without
// us exposing the directories directly.
func (r *Repo) OperateOnFileData(path string, work func(contents []byte, err error) error) error {
	return r.cache.operateOnFileData(path, work)
}

// Checkout runs git checkout.
func (r *Repo) Checkout(commitlike string) error {
	r.logger.WithField("commitlike", commitlike).Info("Checking out revision.")
	if b, err := r.executor.run("checkout", commitlike); err != nil {
		return fmt.Errorf("error checking out %s: %v. output: %s", commitlike, err, string(b))
	}
	return nil
}

// RevParse runs git rev-parse.
func (r *Repo) RevParse(commitlike string) (string, error) {
	r.logger.WithField("commitlike", commitlike).Info("Parsing revision.")
	b, err := r.executor.run("rev-parse", commitlike)
	if err != nil {
		return "", fmt.Errorf("error rev-parsing %s: %v. output: %s", commitlike, err, string(b))
	}
	return string(b), nil
}

// CheckoutNewBranch creates a new branch and checks it out.
func (r *Repo) CheckoutNewBranch(branch string) error {
	r.logger.WithField("branch", branch).Info("Creating and checking out new branch.")
	if b, err := r.executor.run("checkout", "-b", branch); err != nil {
		return fmt.Errorf("error checking out %s: %v. output: %s", branch, err, string(b))
	}
	return nil
}

// Merge attempts to merge commitlike into the current branch. It returns true
// if the merge completes. It returns an error if the abort fails.
func (r *Repo) Merge(commitlike string) (bool, error) {
	logger := r.logger.WithField("commitlike", commitlike)
	logger.Info("Merging revision.")

	b, err := r.executor.run("merge", "--no-ff", "--no-stat", "-m merge", commitlike)
	if err == nil {
		return true, nil
	}
	logger.WithError(err).Warningf("Merge failed with output: %s", string(b))

	if b, err := r.executor.run("merge", "--abort"); err != nil {
		return false, fmt.Errorf("error aborting merge for commitlike %s: %v. output: %s", commitlike, err, string(b))
	}

	return false, nil
}

// Am tries to apply the patch in the given path into the current branch
// by performing a three-way merge (similar to git cherry-pick). It returns
// an error if the patch cannot be applied.
func (r *Repo) Am(path string) error {
	logger := r.logger.WithField("path", path)
	logger.Infof("Applying patch from path.")
	b, err := r.executor.run("am", "--3way", path)
	if err == nil {
		return nil
	}
	output := string(b)
	logger.WithError(err).Warningf("Patch apply failed with output: %s", output)
	if b, abortErr := r.executor.run("am", "--abort"); abortErr != nil {
		logger.WithError(abortErr).Warningf("Aborting patch apply failed with output: %s", string(b))
	}
	applyMsg := "The copy of the patch that failed is found in: .git/rebase-apply/patch"
	if strings.Contains(output, applyMsg) {
		i := strings.Index(output, applyMsg)
		err = fmt.Errorf("%s", output[:i])
	}
	return err
}

// Push pushes over https to the provided owner/repo#branch using a password
// for basic auth.
func (r *Repo) Push(branch string) error {
	r.logger.WithFields(logrus.Fields{"branch": branch}).Info("Pushing to remote branch.")
	_, err := r.executor.run("push", r.executor.remote(), branch)
	return err
}

// CheckoutPullRequest does exactly that.
func (r *Repo) CheckoutPullRequest(number int) error {
	r.logger.WithFields(logrus.Fields{"pr": number}).Info("Checking out pull request.")
	if b, err := r.executor.runWithRetries(3, "fetch", r.executor.remote(), fmt.Sprintf("pull/%d/head:pull%d", number, number)); err != nil {
		return fmt.Errorf("git fetch failed for PR %d: %v. output: %s", number, err, string(b))
	}
	if b, err := r.executor.run("checkout", fmt.Sprintf("pull%d", number)); err != nil {
		return fmt.Errorf("git checkout failed for PR %d: %v. output: %s", number, err, string(b))
	}
	return nil
}

// Config runs git config.
func (r *Repo) Config(key, value string) error {
	r.logger.Infof("Running git config %s %s", key, value)
	if b, err := r.executor.run("config", key, value); err != nil {
		return fmt.Errorf("git config %s %s failed: %v. output: %s", key, value, err, string(b))
	}
	return nil
}

// Log returns the git log, one line per commit.
func (r *Repo) Log() (string, error) {
	r.logger.Info("Running git log --oneline")
	b, err := r.executor.run("log", "--oneline")
	output := string(b)
	if err != nil {
		return output, fmt.Errorf("git log --oneline failed: %v. output: %s", err, output)
	}
	return output, nil
}
