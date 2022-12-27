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
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/git/types"
)

const github = "github.com"

// Client can clone repos. It keeps a local cache, so successive clones of the
// same repo should be quick. Create with NewClient. Be sure to clean it up.
type Client struct {
	// logger will be used to log git operations and must be set.
	logger *logrus.Entry

	credLock sync.RWMutex
	// user is used when pushing or pulling code if specified.
	user string

	// needed to generate the token.
	tokenGenerator GitTokenGenerator

	// dir is the location of the git cache.
	dir string
	// git is the path to the git binary.
	git string
	// base is the base path for git clone calls. For users it will be set to
	// GitHub, but for tests set it to a directory with git repos.
	base string
	// host is the git host.
	// TODO: use either base or host. the redundancy here is to help landing
	// #14609 easier.
	host string

	// The mutex protects repoLocks which protect individual repos. This is
	// necessary because Clone calls for the same repo are racy. Rather than
	// one lock for all repos, use a lock per repo.
	// Lock with Client.lockRepo, unlock with Client.unlockRepo.
	rlm       sync.Mutex
	repoLocks map[string]*sync.Mutex
}

// Clean removes the local repo cache. The Client is unusable after calling.
func (c *Client) Clean() error {
	return os.RemoveAll(c.dir)
}

// NewClient returns a client that talks to GitHub. It will fail if git is not
// in the PATH.
func NewClient() (*Client, error) {
	return NewClientWithHost(github)
}

// NewClientWithHost creates a client with specified host.
func NewClientWithHost(host string) (*Client, error) {
	g, err := exec.LookPath("git")
	if err != nil {
		return nil, err
	}
	t, err := os.MkdirTemp("", "git")
	if err != nil {
		return nil, err
	}
	return &Client{
		logger:         logrus.WithField("client", "git"),
		tokenGenerator: func(_ string) (string, error) { return "", nil },
		dir:            t,
		git:            g,
		base:           fmt.Sprintf("https://%s", host),
		host:           host,
		repoLocks:      make(map[string]*sync.Mutex),
	}, nil
}

// SetRemote sets the remote for the client. This is not thread-safe, and is
// useful for testing. The client will clone from remote/org/repo, and Repo
// objects spun out of the client will also hit that path.
// TODO: c.host field needs to be updated accordingly.
func (c *Client) SetRemote(remote string) {
	c.base = remote
}

type GitTokenGenerator func(org string) (string, error)

// SetCredentials sets credentials in the client to be used for pushing to
// or pulling from remote repositories.
func (c *Client) SetCredentials(user string, tokenGenerator GitTokenGenerator) {
	c.credLock.Lock()
	defer c.credLock.Unlock()
	c.user = user
	c.tokenGenerator = tokenGenerator
}

func (c *Client) getCredentials(org string) (string, string, error) {
	c.credLock.RLock()
	defer c.credLock.RUnlock()
	token, err := c.tokenGenerator(org)
	return c.user, token, err
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

func remoteFromBase(base, user, pass, host, org, repo string) string {
	baseWithAuth := base
	if user != "" && pass != "" {
		baseWithAuth = fmt.Sprintf("https://%s:%s@%s", user, pass, host)
	}
	return fmt.Sprintf("%s/%s/%s", baseWithAuth, org, repo)
}

// Clone clones a repository. Pass the full repository name, such as
// "kubernetes/test-infra" as the repo.
// This function may take a long time if it is the first time cloning the repo.
// In that case, it must do a full git mirror clone. For large repos, this can
// take a while. Once that is done, it will do a git fetch instead of a clone,
// which will usually take at most a few seconds.
func (c *Client) Clone(organization, repository string) (*Repo, error) {
	orgRepo := organization + "/" + repository
	c.lockRepo(orgRepo)
	defer c.unlockRepo(orgRepo)

	user, pass, err := c.getCredentials(organization)
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}
	cache := filepath.Join(c.dir, orgRepo) + ".git"
	remote := remoteFromBase(c.base, user, pass, c.host, organization, repository)
	if _, err := os.Stat(cache); os.IsNotExist(err) {
		// Cache miss, clone it now.
		c.logger.WithField("repo", orgRepo).Info("Cloning for the first time.")
		if err := os.MkdirAll(filepath.Dir(cache), os.ModePerm); err != nil && !os.IsExist(err) {
			return nil, err
		}
		if b, err := retryCmd(c.logger, "", c.git, "clone", "--mirror", remote, cache); err != nil {
			return nil, fmt.Errorf("git cache clone error: %v. output: %s", err, string(b))
		}
	} else if err != nil {
		return nil, err
	} else {
		// Cache hit. Do a git fetch to keep updated.
		// Update remote url, if we use apps auth the token changes every hour
		if b, err := retryCmd(c.logger, cache, c.git, "remote", "set-url", "origin", remote); err != nil {
			return nil, fmt.Errorf("updating remote url failed: %w. output: %s", err, string(b))
		}
		c.logger.WithField("repo", orgRepo).Info("Fetching.")
		if b, err := retryCmd(c.logger, cache, c.git, "fetch", "--prune"); err != nil {
			return nil, fmt.Errorf("git fetch error: %v. output: %s", err, string(b))
		}
	}
	t, err := os.MkdirTemp("", "git")
	if err != nil {
		return nil, err
	}
	if b, err := exec.Command(c.git, "clone", cache, t).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git repo clone error: %v. output: %s", err, string(b))
	}
	// Updating remote url to true remote like `git@github.com:kubernetes/test-infra.git`,
	// instead of something like `/tmp/12345/test-infra`, so that `git fetch` in this clone makes more sense.
	cmd := exec.Command(c.git, "remote", "set-url", "origin", remote)
	cmd.Dir = t
	if b, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("updating remote url failed: %w. output: %s", err, string(b))
	}
	r := &Repo{
		dir:            t,
		logger:         c.logger,
		git:            c.git,
		host:           c.host,
		base:           c.base,
		org:            organization,
		repo:           repository,
		user:           user,
		pass:           pass,
		tokenGenerator: c.tokenGenerator,
	}
	// disable git GC
	if err := r.Config("gc.auto", "0"); err != nil {
		return nil, err
	}
	return r, nil
}

// Repo is a clone of a git repository. Create with Client.Clone, and don't
// forget to clean it up after.
type Repo struct {
	// dir is the location of the git repo.
	dir string

	// git is the path to the git binary.
	git string
	// host is the git host.
	host string
	// base is the base path for remote git fetch calls.
	base string
	// org is the organization name: "org" in "org/repo".
	org string
	// repo is the repository name: "repo" in "org/repo".
	repo string
	// user is used for pushing to the remote repo.
	user string
	// pass is used for pushing to the remote repo.
	pass string

	// needed to generate the token.
	tokenGenerator GitTokenGenerator

	credLock sync.RWMutex

	logger *logrus.Entry
}

// Directory exposes the location of the git repo
func (r *Repo) Directory() string {
	return r.dir
}

// SetLogger sets logger: Do not use except in unit tests
func (r *Repo) SetLogger(logger *logrus.Entry) {
	r.logger = logger
}

// SetGit sets git: Do not use except in unit tests
func (r *Repo) SetGit(git string) {
	r.git = git
}

// Clean deletes the repo. It is unusable after calling.
func (r *Repo) Clean() error {
	return os.RemoveAll(r.dir)
}

// refreshRepoAuth updates Repo client token when current token is going to expire.
// Git client authenticating with PAT(personal access token) doesn't have this problem as it's a single token.
// GitHub app auth will need this for rotating token every hour.
func (r *Repo) refreshRepoAuth() error {
	// Lock because we'll update r.pass here
	r.credLock.Lock()
	defer r.credLock.Unlock()
	pass, err := r.tokenGenerator(r.org)
	if err != nil {
		return fmt.Errorf("failed to get token: %w", err)
	}
	if pass == r.pass { // Token unchanged, no need to do anything
		return nil
	}

	r.pass = pass
	remote := remoteFromBase(r.base, r.user, r.pass, r.host, r.org, r.repo)
	if b, err := r.gitCommand("remote", "set-url", "origin", remote).CombinedOutput(); err != nil {
		return fmt.Errorf("updating remote url failed: %w. output: %s", err, string(b))
	}
	return nil
}

// ResetHard runs `git reset --hard`
func (r *Repo) ResetHard(commitlike string) error {
	// `git reset --hard` doesn't cleanup untracked file
	r.logger.Info("Clean untracked files and dirs.")
	if b, err := r.gitCommand("clean", "-df").CombinedOutput(); err != nil {
		return fmt.Errorf("error clean -df: %v. output: %s", err, string(b))
	}
	r.logger.WithField("commitlike", commitlike).Info("Reset hard.")
	co := r.gitCommand("reset", "--hard", commitlike)
	if b, err := co.CombinedOutput(); err != nil {
		return fmt.Errorf("error reset hard %s: %v. output: %s", commitlike, err, string(b))
	}
	return nil
}

// IsDirty checks whether the repo is dirty or not
func (r *Repo) IsDirty() (bool, error) {
	r.logger.Info("Checking is dirty.")
	b, err := r.gitCommand("status", "--porcelain").CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("error add -A: %v. output: %s", err, string(b))
	}
	return len(b) > 0, nil
}

func (r *Repo) gitCommand(arg ...string) *exec.Cmd {
	cmd := exec.Command(r.git, arg...)
	cmd.Dir = r.dir
	r.logger.WithField("args", cmd.Args).WithField("dir", cmd.Dir).Debug("Constructed git command")
	return cmd
}

// Checkout runs git checkout.
func (r *Repo) Checkout(commitlike string) error {
	r.logger.WithField("commitlike", commitlike).Info("Checkout.")
	co := r.gitCommand("checkout", commitlike)
	if b, err := co.CombinedOutput(); err != nil {
		return fmt.Errorf("error checking out %s: %v. output: %s", commitlike, err, string(b))
	}
	return nil
}

// RevParse runs git rev-parse.
func (r *Repo) RevParse(commitlike string) (string, error) {
	r.logger.WithField("commitlike", commitlike).Info("RevParse.")
	b, err := r.gitCommand("rev-parse", commitlike).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("error rev-parsing %s: %v. output: %s", commitlike, err, string(b))
	}
	return string(b), nil
}

// BranchExists returns true if branch exists in heads.
func (r *Repo) BranchExists(branch string) bool {
	heads := "origin"
	r.logger.WithFields(logrus.Fields{"branch": branch, "heads": heads}).Info("Checking if branch exists.")
	co := r.gitCommand("ls-remote", "--exit-code", "--heads", heads, branch)
	return co.Run() == nil
}

// CheckoutNewBranch creates a new branch and checks it out.
func (r *Repo) CheckoutNewBranch(branch string) error {
	r.logger.WithField("branch", branch).Info("Create and checkout.")
	co := r.gitCommand("checkout", "-b", branch)
	if b, err := co.CombinedOutput(); err != nil {
		return fmt.Errorf("error checking out %s: %v. output: %s", branch, err, string(b))
	}
	return nil
}

// Merge attempts to merge commitlike into the current branch. It returns true
// if the merge completes. It returns an error if the abort fails.
func (r *Repo) Merge(commitlike string) (bool, error) {
	return r.MergeWithStrategy(commitlike, types.MergeMerge)
}

// MergeWithStrategy attempts to merge commitlike into the current branch given the merge strategy.
// It returns true if the merge completes. It returns an error if the abort fails.
func (r *Repo) MergeWithStrategy(commitlike string, mergeStrategy types.PullRequestMergeType) (bool, error) {
	r.logger.WithField("commitlike", commitlike).Info("Merging.")
	switch mergeStrategy {
	case types.MergeMerge:
		return r.mergeWithMergeStrategyMerge(commitlike)
	case types.MergeSquash:
		return r.mergeWithMergeStrategySquash(commitlike)
	case types.MergeRebase:
		return r.mergeWithMergeStrategyRebase(commitlike)
	default:
		return false, fmt.Errorf("merge strategy %q is not supported", mergeStrategy)
	}
}

func (r *Repo) mergeWithMergeStrategyMerge(commitlike string) (bool, error) {
	co := r.gitCommand("merge", "--no-ff", "--no-stat", "-m merge", commitlike)

	b, err := co.CombinedOutput()
	if err == nil {
		return true, nil
	}
	r.logger.WithField("out", string(b)).WithError(err).Infof("Merge failed.")

	if b, err := r.gitCommand("merge", "--abort").CombinedOutput(); err != nil {
		return false, fmt.Errorf("error aborting merge for commitlike %s: %v. output: %s", commitlike, err, string(b))
	}

	return false, nil
}

func (r *Repo) mergeWithMergeStrategySquash(commitlike string) (bool, error) {
	co := r.gitCommand("merge", "--squash", "--no-stat", commitlike)

	b, err := co.CombinedOutput()
	if err != nil {
		r.logger.WithField("out", string(b)).WithError(err).Infof("Merge failed.")
		if b, err := r.gitCommand("reset", "--hard", "HEAD").CombinedOutput(); err != nil {
			return false, fmt.Errorf("error resetting after failed squash for commitlike %s: %v. output: %s", commitlike, err, string(b))
		}
		return false, nil
	}

	b, err = r.gitCommand("commit", "--no-stat", "-m", "merge").CombinedOutput()
	if err != nil {
		r.logger.WithField("out", string(b)).WithError(err).Infof("Commit after squash failed.")
		return false, err
	}

	return true, nil
}

func (r *Repo) mergeWithMergeStrategyRebase(commitlike string) (bool, error) {
	if commitlike == "" {
		return false, errors.New("branch must be set")
	}

	headRev, err := r.revParse("HEAD")
	if err != nil {
		r.logger.WithError(err).Infof("Failed to parse HEAD revision")
		return false, err
	}
	headRev = strings.TrimSuffix(headRev, "\n")

	co := r.gitCommand("rebase", "--no-stat", headRev, commitlike)
	b, err := co.CombinedOutput()
	if err != nil {
		r.logger.WithField("out", string(b)).WithError(err).Infof("Rebase failed.")
		if b, err := r.gitCommand("rebase", "--abort").CombinedOutput(); err != nil {
			return false, fmt.Errorf("error aborting after failed rebase for commitlike %s: %v. output: %s", commitlike, err, string(b))
		}
		return false, nil
	}

	return true, nil
}

func (r *Repo) revParse(args ...string) (string, error) {
	fullArgs := append([]string{"rev-parse"}, args...)
	co := r.gitCommand(fullArgs...)
	b, err := co.CombinedOutput()
	if err != nil {
		return "", errors.New(string(b))
	}
	return string(b), nil
}

// MergeAndCheckout merges the provided headSHAs in order onto baseSHA using the provided strategy.
// If no headSHAs are provided, it will only checkout the baseSHA and return.
// Only the `merge` and `squash` strategies are supported.
func (r *Repo) MergeAndCheckout(baseSHA string, mergeStrategy types.PullRequestMergeType, headSHAs ...string) error {
	if baseSHA == "" {
		return errors.New("baseSHA must be set")
	}
	if err := r.Checkout(baseSHA); err != nil {
		return err
	}
	if len(headSHAs) == 0 {
		return nil
	}
	r.logger.WithFields(logrus.Fields{"headSHAs": headSHAs, "baseSHA": baseSHA, "strategy": mergeStrategy}).Info("Merging.")
	for _, headSHA := range headSHAs {
		ok, err := r.MergeWithStrategy(headSHA, mergeStrategy)
		if err != nil {
			return err
		} else if !ok {
			return fmt.Errorf("failed to merge %q", headSHA)
		}
	}
	return nil
}

// Am tries to apply the patch in the given path into the current branch
// by performing a three-way merge (similar to git cherry-pick). It returns
// an error if the patch cannot be applied.
func (r *Repo) Am(path string) error {
	r.logger.WithField("path", path).Info("Applying.")
	co := r.gitCommand("am", "--3way", path)
	b, err := co.CombinedOutput()
	if err == nil {
		return nil
	}
	output := string(b)
	r.logger.WithField("out", output).WithError(err).Infof("Patch apply failed.")
	if b, abortErr := r.gitCommand("am", "--abort").CombinedOutput(); abortErr != nil {
		r.logger.WithField("out", string(b)).WithError(abortErr).Warning("Aborting patch apply failed.")
	}
	applyMsg := "The copy of the patch that failed is found in: .git/rebase-apply/patch"
	msg := ""
	if strings.Contains(output, applyMsg) {
		i := strings.Index(output, applyMsg)
		msg = string(output[:i])
	} else {
		msg = string(output)
	}
	return errors.New(msg)
}

// Push pushes over https to the provided owner/repo#branch using a password
// for basic auth.
func (r *Repo) Push(branch string, force bool) error {
	return r.PushToNamedFork(r.user, branch, force)
}

func (r *Repo) PushToNamedFork(forkName, branch string, force bool) error {
	if err := r.refreshRepoAuth(); err != nil {
		return err
	}
	if r.user == "" || r.pass == "" {
		return errors.New("cannot push without credentials - configure your git client")
	}
	r.logger.WithFields(logrus.Fields{"user": r.user, "repo": r.repo, "branch": branch}).Info("Pushing.")
	remote := remoteFromBase(r.base, r.user, r.pass, r.host, r.user, forkName)
	var co *exec.Cmd
	if !force {
		co = r.gitCommand("push", remote, branch)
	} else {
		co = r.gitCommand("push", "--force", remote, branch)
	}
	out, err := co.CombinedOutput()
	if err != nil {
		r.logger.WithField("out", string(out)).WithError(err).Error("Pushing failed.")
		return fmt.Errorf("pushing failed, output: %q, error: %w", string(out), err)
	}
	return nil
}

// CheckoutPullRequest does exactly that.
func (r *Repo) CheckoutPullRequest(number int) error {
	if err := r.refreshRepoAuth(); err != nil {
		return err
	}
	r.logger.WithFields(logrus.Fields{"org": r.org, "repo": r.repo, "number": number}).Info("Fetching and checking out.")
	remote := remoteFromBase(r.base, r.user, r.pass, r.host, r.org, r.repo)
	if b, err := retryCmd(r.logger, r.dir, r.git, "fetch", remote, fmt.Sprintf("pull/%d/head:pull%d", number, number)); err != nil {
		return fmt.Errorf("git fetch failed for PR %d: %v. output: %s", number, err, string(b))
	}
	co := r.gitCommand("checkout", fmt.Sprintf("pull%d", number))
	if b, err := co.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout failed for PR %d: %v. output: %s", number, err, string(b))
	}
	return nil
}

// Config runs git config.
func (r *Repo) Config(args ...string) error {
	r.logger.WithField("args", args).Info("Running git config.")
	if b, err := r.gitCommand(append([]string{"config"}, args...)...).CombinedOutput(); err != nil {
		return fmt.Errorf("git config %v failed: %v. output: %s", args, err, string(b))
	}
	return nil
}

// retryCmd will retry the command a few times with backoff. Use this for any
// commands that will be talking to GitHub, such as clones or fetches.
func retryCmd(l *logrus.Entry, dir, cmd string, arg ...string) ([]byte, error) {
	var b []byte
	var err error
	sleepyTime := time.Second
	for i := 0; i < 3; i++ {
		c := exec.Command(cmd, arg...)
		c.Dir = dir
		b, err = c.CombinedOutput()
		if err != nil {
			err = fmt.Errorf("running %q %v returned error %w with output %q", cmd, arg, err, string(b))
			l.WithField("count", i+1).WithError(err).Debug("Retrying, if this is not the 3rd try then this will be retried.")
			time.Sleep(sleepyTime)
			sleepyTime *= 2
			continue
		}
		break
	}
	return b, err
}

// Diff runs 'git diff HEAD <sha> --name-only' and returns a list
// of file names with upcoming changes
func (r *Repo) Diff(head, sha string) (changes []string, err error) {
	r.logger.WithField("sha", sha).Info("Diff head.")
	output, err := r.gitCommand("diff", head, sha, "--name-only").CombinedOutput()
	if err != nil {
		return nil, err
	}
	scan := bufio.NewScanner(bytes.NewReader(output))
	scan.Split(bufio.ScanLines)
	for scan.Scan() {
		changes = append(changes, scan.Text())
	}
	return
}

// MergeCommitsExistBetween runs 'git log <target>..<head> --merged' to verify
// if merge commits exist between "target" and "head".
func (r *Repo) MergeCommitsExistBetween(target, head string) (bool, error) {
	r.logger.WithFields(logrus.Fields{"target": target, "head": head}).Info("Verifying if merge commits exist.")
	b, err := r.gitCommand("log", fmt.Sprintf("%s..%s", target, head), "--oneline", "--merges").CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("error verifying if merge commits exist between %s and %s: %v. output: %s", target, head, err, string(b))
	}
	return len(b) != 0, nil
}

// ShowRef returns the commit for a commitlike. Unlike rev-parse it does not require a checkout.
func (r *Repo) ShowRef(commitlike string) (string, error) {
	r.logger.WithField("commitlike", commitlike).Info("Getting the commit sha.")
	out, err := r.gitCommand("show-ref", "-s", commitlike).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get commit sha for commitlike %s: %w", commitlike, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Fetch fetches from remote
func (r *Repo) Fetch(arg ...string) error {
	arg = append([]string{"fetch"}, arg...)
	if err := r.refreshRepoAuth(); err != nil {
		return err
	}
	r.logger.Infof("Fetching from remote.")
	out, err := r.gitCommand(arg...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to fetch: %v.\nOutput: %s", err, string(out))
	}
	return nil
}
