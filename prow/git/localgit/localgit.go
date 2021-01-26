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

// Package localgit creates a local git repo that can be used for testing code
// that uses a git.Client.
package localgit

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"k8s.io/test-infra/prow/git"
	v2 "k8s.io/test-infra/prow/git/v2"
)

type Clients func() (*LocalGit, v2.ClientFactory, error)

// LocalGit stores the repos in a temp dir. Create with New and delete with
// Clean.
type LocalGit struct {
	// Dir is the path to the base temp dir. Repos are at Dir/org/repo.
	Dir string
	// Git is the location of the git binary.
	Git string
}

// New creates a LocalGit and a client factory from a git.Client pointing at it.
func New() (*LocalGit, v2.ClientFactory, error) {
	g, err := exec.LookPath("git")
	if err != nil {
		return nil, nil, err
	}
	t, err := ioutil.TempDir("", "localgit")
	if err != nil {
		return nil, nil, err
	}
	c, err := git.NewClient()
	if err != nil {
		os.RemoveAll(t)
		return nil, nil, err
	}

	getSecret := func(_ string) (string, error) {
		return "", nil
	}

	c.SetCredentials("", getSecret)

	c.SetRemote(t)
	return &LocalGit{
		Dir: t,
		Git: g,
	}, v2.ClientFactoryFrom(c), nil
}

// Clean deletes the local git dir.
func (lg *LocalGit) Clean() error {
	return os.RemoveAll(lg.Dir)
}

func runCmd(cmd, dir string, arg ...string) error {
	c := exec.Command(cmd, arg...)
	c.Dir = dir
	if b, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %v: %v, %s", cmd, arg, err, string(b))
	}
	return nil
}

func runCmdOutput(cmd, dir string, arg ...string) (string, error) {
	c := exec.Command(cmd, arg...)
	c.Dir = dir
	b, err := c.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %v: %v, %s", cmd, arg, err, string(b))
	}
	return strings.TrimSpace(string(b)), nil
}

// MakeFakeRepo creates the given repo and makes an initial commit.
func (lg *LocalGit) MakeFakeRepo(org, repo string) error {
	rdir := filepath.Join(lg.Dir, org, repo)
	if err := os.MkdirAll(rdir, os.ModePerm); err != nil {
		return err
	}

	if err := runCmd(lg.Git, rdir, "init"); err != nil {
		return err
	}
	if err := runCmd(lg.Git, rdir, "config", "user.email", "test@test.test"); err != nil {
		return err
	}
	if err := runCmd(lg.Git, rdir, "config", "user.name", "test test"); err != nil {
		return err
	}
	if err := runCmd(lg.Git, rdir, "config", "commit.gpgsign", "false"); err != nil {
		return err
	}
	if err := lg.AddCommit(org, repo, map[string][]byte{"initial": {}}); err != nil {
		return err
	}

	return nil
}

// AddCommit adds the files to a new commit in the repo.
func (lg *LocalGit) AddCommit(org, repo string, files map[string][]byte) error {
	rdir := filepath.Join(lg.Dir, org, repo)
	for f, b := range files {
		path := filepath.Join(rdir, f)
		if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
			return err
		}
		if err := ioutil.WriteFile(path, b, os.ModePerm); err != nil {
			return err
		}
		if err := runCmd(lg.Git, rdir, "add", f); err != nil {
			return err
		}
	}
	return runCmd(lg.Git, rdir, "commit", "-m", "wow")
}

// RmCommit adds a commit that removes some files from the repo
func (lg *LocalGit) RmCommit(org, repo string, files []string) error {
	rdir := filepath.Join(lg.Dir, org, repo)
	for _, f := range files {
		if err := runCmd(lg.Git, rdir, "rm", f); err != nil {
			return err
		}
	}
	return runCmd(lg.Git, rdir, "commit", "-m", "remove some files")
}

// CheckoutNewBranch does git checkout -b.
func (lg *LocalGit) CheckoutNewBranch(org, repo, branch string) error {
	rdir := filepath.Join(lg.Dir, org, repo)
	return runCmd(lg.Git, rdir, "checkout", "-b", branch)
}

// Checkout does git checkout.
func (lg *LocalGit) Checkout(org, repo, commitlike string) error {
	rdir := filepath.Join(lg.Dir, org, repo)
	return runCmd(lg.Git, rdir, "checkout", commitlike)
}

// RevParse does git rev-parse.
func (lg *LocalGit) RevParse(org, repo, commitlike string) (string, error) {
	rdir := filepath.Join(lg.Dir, org, repo)
	return runCmdOutput(lg.Git, rdir, "rev-parse", commitlike)
}

// Merge does git merge.
func (lg *LocalGit) Merge(org, repo, commitlike string) (string, error) {
	rdir := filepath.Join(lg.Dir, org, repo)
	return runCmdOutput(lg.Git, rdir, "merge", "--no-ff", "--no-stat", "-m merge", commitlike)
}

// Rebase does git rebase.
func (lg *LocalGit) Rebase(org, repo, commitlike string) (string, error) {
	rdir := filepath.Join(lg.Dir, org, repo)
	return runCmdOutput(lg.Git, rdir, "rebase", commitlike)
}

// NewV2 creates a LocalGit and a v2 client factory pointing at it.
func NewV2() (*LocalGit, v2.ClientFactory, error) {
	g, err := exec.LookPath("git")
	if err != nil {
		return nil, nil, err
	}
	t, err := ioutil.TempDir("", "localgit")
	if err != nil {
		return nil, nil, err
	}
	c, err := v2.NewLocalClientFactory(t,
		func() (name, email string, err error) { return "robot", "robot@beep.boop", nil },
		func(content []byte) []byte { return content })
	if err != nil {
		return nil, nil, err
	}
	return &LocalGit{
		Dir: t,
		Git: g,
	}, c, nil
}
