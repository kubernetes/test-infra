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
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

// Interactor knows how to operate on a git repository cloned from GitHub
// using a local cache.
type Interactor interface {
	// Directory exposes the directory in which the repository has been cloned
	Directory() string
	// Clean removes the repository. It is up to the user to call this once they are done
	Clean() error
	// Checkout runs `git checkout`
	Checkout(commitlike string) error
	// RevParse runs `git rev-parse`
	RevParse(commitlike string) (string, error)
	// BranchExists determines if a branch with the name exists
	BranchExists(branch string) bool
	// CheckoutNewBranch creates a new branch from HEAD and checks it out
	CheckoutNewBranch(branch string) error
	// Merge merges the commitlike into the current HEAD
	Merge(commitlike string) (bool, error)
	// MergeWithStrategy merges the commitlike into the current HEAD with the strategy
	MergeWithStrategy(commitlike, mergeStrategy string) (bool, error)
	// MergeAndCheckout merges all commitlikes into the current HEAD with the appropriate strategy
	MergeAndCheckout(baseSHA string, mergeStrategy string, headSHAs ...string) error
	// Am calls `git am`
	Am(path string) error
	// Fetch calls `git fetch`
	Fetch() error
	// FetchRef fetches the refspec
	FetchRef(refspec string) error
	// CheckoutPullRequest fetches and checks out the synthetic refspec from GitHub for a pull request HEAD
	CheckoutPullRequest(number int) error
	// Config runs `git config`
	Config(key, value string) error
	// Diff runs `git diff`
	Diff(head, sha string) (changes []string, err error)
	// MergeCommitsExistBetween determines if merge commits exist between target and HEAD
	MergeCommitsExistBetween(target, head string) (bool, error)
	// ShowRef returns the commit for a commitlike. Unlike rev-parse it does not require a checkout.
	ShowRef(commitlike string) (string, error)
}

// cacher knows how to cache and update repositories in a central cache
type cacher interface {
	// MirrorClone sets up a mirror of the source repository.
	MirrorClone() error
	// RemoteUpdate fetches all updates from the remote.
	RemoteUpdate() error
}

// cloner knows how to clone repositories from a central cache
type cloner interface {
	// Clone clones the repository from a local path.
	Clone(from string) error
}

type interactor struct {
	executor executor
	remote   RemoteResolver
	dir      string
	logger   *logrus.Entry
}

// Directory exposes the directory in which this repository has been cloned
func (i *interactor) Directory() string {
	return i.dir
}

// Clean cleans up the repository from the on-disk cache
func (i *interactor) Clean() error {
	return os.RemoveAll(i.dir)
}

// Clone clones the repository from a local path.
func (i *interactor) Clone(from string) error {
	i.logger.Infof("Creating a clone of the repo at %s from %s", i.dir, from)
	if out, err := i.executor.Run("clone", from, i.dir); err != nil {
		return fmt.Errorf("error creating a clone: %v %v", err, string(out))
	}
	return nil
}

// MirrorClone sets up a mirror of the source repository.
func (i *interactor) MirrorClone() error {
	i.logger.Infof("Creating a mirror of the repo at %s", i.dir)
	i.logger.Infof("Creating a mirror of the repo at %s", i.dir)
	remote, err := i.remote()
	if err != nil {
		return fmt.Errorf("could not resolve remote for cloning: %v", err)
	}
	if out, err := i.executor.Run("clone", "--mirror", remote, i.dir); err != nil {
		return fmt.Errorf("error creating a mirror clone: %v %v", err, string(out))
	}
	return nil
}

// Checkout runs git checkout.
func (i *interactor) Checkout(commitlike string) error {
	i.logger.Infof("Checking out %q", commitlike)
	if out, err := i.executor.Run("checkout", commitlike); err != nil {
		return fmt.Errorf("error checking out %q: %v %v", commitlike, err, string(out))
	}
	return nil
}

// RevParse runs git rev-parse.
func (i *interactor) RevParse(commitlike string) (string, error) {
	i.logger.Infof("Parsing revision %q", commitlike)
	out, err := i.executor.Run("rev-parse", commitlike)
	if err != nil {
		return "", fmt.Errorf("error parsing %q: %v %v", commitlike, err, string(out))
	}
	return string(out), nil
}

// BranchExists returns true if branch exists in heads.
func (i *interactor) BranchExists(branch string) bool {
	i.logger.Infof("Checking if branch %q exists", branch)
	_, err := i.executor.Run("ls-remote", "--exit-code", "--heads", "origin", branch)
	return err == nil
}

// CheckoutNewBranch creates a new branch and checks it out.
func (i *interactor) CheckoutNewBranch(branch string) error {
	i.logger.Infof("Checking out new branch %q", branch)
	if out, err := i.executor.Run("checkout", "-b", branch); err != nil {
		return fmt.Errorf("error checking out new branch %q: %v %v", branch, err, string(out))
	}
	return nil
}

// Merge attempts to merge commitlike into the current branch. It returns true
// if the merge completes. It returns an error if the abort fails.
func (i *interactor) Merge(commitlike string) (bool, error) {
	return i.MergeWithStrategy(commitlike, "merge")
}

// MergeWithStrategy attempts to merge commitlike into the current branch given the merge strategy.
// It returns true if the merge completes. if the merge does not complete successfully, we try to
// abort it and return an error if the abort fails.
func (i *interactor) MergeWithStrategy(commitlike, mergeStrategy string) (bool, error) {
	i.logger.Infof("Merging %q using the %q strategy", commitlike, mergeStrategy)
	switch mergeStrategy {
	case "merge":
		return i.mergeMerge(commitlike)
	case "squash":
		return i.squashMerge(commitlike)
	default:
		return false, fmt.Errorf("merge strategy %q is not supported", mergeStrategy)
	}
}

func (i *interactor) mergeMerge(commitlike string) (bool, error) {
	out, err := i.executor.Run("merge", "--no-ff", "--no-stat", "-m", "merge", commitlike)
	if err == nil {
		return true, nil
	}
	i.logger.WithError(err).Warnf("Error merging %q: %s", commitlike, string(out))
	if out, err := i.executor.Run("merge", "--abort"); err != nil {
		return false, fmt.Errorf("error aborting merge of %q: %v %v", commitlike, err, string(out))
	}
	return false, nil
}

func (i *interactor) squashMerge(commitlike string) (bool, error) {
	out, err := i.executor.Run("merge", "--squash", "--no-stat", commitlike)
	if err != nil {
		i.logger.WithError(err).Warnf("Error staging merge for %q: %s", commitlike, string(out))
		if out, err := i.executor.Run("reset", "--hard", "HEAD"); err != nil {
			return false, fmt.Errorf("error aborting merge of %q: %v %v", commitlike, err, string(out))
		}
		return false, nil
	}
	out, err = i.executor.Run("commit", "--no-stat", "-m", "merge")
	if err != nil {
		i.logger.WithError(err).Warnf("Error committing merge for %q: %s", commitlike, string(out))
		if out, err := i.executor.Run("reset", "--hard", "HEAD"); err != nil {
			return false, fmt.Errorf("error aborting merge of %q: %v %v", commitlike, err, string(out))
		}
		return false, nil
	}
	return true, nil
}

// Only the `merge` and `squash` strategies are supported.
func (i *interactor) MergeAndCheckout(baseSHA string, mergeStrategy string, headSHAs ...string) error {
	if baseSHA == "" {
		return errors.New("baseSHA must be set")
	}
	if err := i.Checkout(baseSHA); err != nil {
		return err
	}
	for _, headSHA := range headSHAs {
		ok, err := i.MergeWithStrategy(headSHA, mergeStrategy)
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
func (i *interactor) Am(path string) error {
	i.logger.Infof("Applying patch at %s", path)
	out, err := i.executor.Run("am", "--3way", path)
	if err == nil {
		return nil
	}
	i.logger.WithError(err).Infof("Patch apply failed with output: %s", string(out))
	if abortOut, abortErr := i.executor.Run("am", "--abort"); err != nil {
		i.logger.WithError(abortErr).Warningf("Aborting patch apply failed with output: %s", string(abortOut))
	}
	return errors.New(string(bytes.TrimPrefix(out, []byte("The copy of the patch that failed is found in: .git/rebase-apply/patch"))))
}

// RemoteUpdate fetches all updates from the remote.
func (i *interactor) RemoteUpdate() error {
	i.logger.Info("Updating from remote")
	if out, err := i.executor.Run("remote", "update"); err != nil {
		return fmt.Errorf("error updating: %v %v", err, string(out))
	}
	return nil
}

// Fetch fetches all updates from the remote.
func (i *interactor) Fetch() error {
	remote, err := i.remote()
	if err != nil {
		return fmt.Errorf("could not resolve remote for fetching: %v", err)
	}
	i.logger.Infof("Fetching from %s", remote)
	if out, err := i.executor.Run("fetch", remote); err != nil {
		return fmt.Errorf("error fetching: %v %v", err, string(out))
	}
	return nil
}

// FetchRef fetches a refspec from the remote and leaves it as FETCH_HEAD.
func (i *interactor) FetchRef(refspec string) error {
	remote, err := i.remote()
	if err != nil {
		return fmt.Errorf("could not resolve remote for fetching: %v", err)
	}
	i.logger.Infof("Fetching %q from %s", refspec, remote)
	if out, err := i.executor.Run("fetch", remote, refspec); err != nil {
		return fmt.Errorf("error fetching %q: %v %v", refspec, err, string(out))
	}
	return nil
}

// FetchFromRemote fetches all update from a specific remote and branch and leaves it as FETCH_HEAD.
func (i *interactor) FetchFromRemote(remote RemoteResolver, branch string) error {
	r, err := remote()
	if err != nil {
		return fmt.Errorf("couldn't get remote: %v", err)
	}

	i.logger.Infof("Fetching %s from %s", branch, r)
	if out, err := i.executor.Run("fetch", r, branch); err != nil {
		return fmt.Errorf("error fetching %s from %s: %v %v", branch, r, err, string(out))
	}
	return nil
}

// CheckoutPullRequest fetches the HEAD of a pull request using a synthetic refspec
// available on GitHub remotes and creates a branch at that commit.
func (i *interactor) CheckoutPullRequest(number int) error {
	i.logger.Infof("Checking out pull request %d", number)
	if err := i.FetchRef(fmt.Sprintf("pull/%d/head", number)); err != nil {
		return err
	}
	if err := i.Checkout("FETCH_HEAD"); err != nil {
		return err
	}
	if err := i.CheckoutNewBranch(fmt.Sprintf("pull%d", number)); err != nil {
		return err
	}
	return nil
}

// Config runs git config.
func (i *interactor) Config(key, value string) error {
	i.logger.Infof("Configuring %q=%q", key, value)
	if out, err := i.executor.Run("config", key, value); err != nil {
		return fmt.Errorf("error configuring %q=%q: %v %v", key, value, err, string(out))
	}
	return nil
}

// Diff lists the difference between the two references, returning the output
// line by line.
func (i *interactor) Diff(head, sha string) ([]string, error) {
	i.logger.Infof("Finding the differences between %q and %q", head, sha)
	out, err := i.executor.Run("diff", head, sha, "--name-only")
	if err != nil {
		return nil, err
	}
	var changes []string
	scan := bufio.NewScanner(bytes.NewReader(out))
	scan.Split(bufio.ScanLines)
	for scan.Scan() {
		changes = append(changes, scan.Text())
	}
	return changes, nil
}

// MergeCommitsExistBetween runs 'git log <target>..<head> --merged' to verify
// if merge commits exist between "target" and "head".
func (i *interactor) MergeCommitsExistBetween(target, head string) (bool, error) {
	i.logger.Infof("Determining if merge commits exist between %q and %q", target, head)
	out, err := i.executor.Run("log", fmt.Sprintf("%s..%s", target, head), "--oneline", "--merges")
	if err != nil {
		return false, fmt.Errorf("error verifying if merge commits exist between %q and %q: %v %s", target, head, err, string(out))
	}
	return len(out) != 0, nil
}

func (i *interactor) ShowRef(commitlike string) (string, error) {
	i.logger.Infof("Getting the commit sha for commitlike %s", commitlike)
	out, err := i.executor.Run("show-ref", "-s", commitlike)
	if err != nil {
		return "", fmt.Errorf("failed to get commit sha for commitlike %s: %v", commitlike, err)
	}
	return strings.TrimSpace(string(out)), nil
}
