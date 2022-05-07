/*
Copyright 2022 The Kubernetes Authors.

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

// fakegitserver serves Git repositories over HTTP for integration tests.
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/cgi"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-billy/v5/util"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pjutil"
)

type options struct {
	port                int
	gitBinary           string
	gitReposParentDir   string
	populateSampleRepos bool
	fooRepoRemoteURL    string
}

func (o *options) validate() error {
	return nil
}

// flagOptions defines default options.
func flagOptions() *options {
	o := &options{}
	flag.IntVar(&o.port, "port", 8888, "Port to listen on.")
	flag.StringVar(&o.gitBinary, "git-binary", "/usr/bin/git", "Path to the `git` binary.")
	flag.StringVar(&o.gitReposParentDir, "git-repos-parent-dir", "/git-repo", "Path to the parent folder containing all Git repos to serve over HTTP.")
	flag.BoolVar(&o.populateSampleRepos, "populate-sample-repos", false, "Whether to populate /git-repo with hardcoded sample repos. Used for integration tests.")
	flag.StringVar(&o.fooRepoRemoteURL, "foo-repo-remote-URL", "http://localhost:8888/repo/foo", "URL of foo repo, as a submodule from inside the bar repo. This is only used when -populate-sample-repos is given.")
	return o
}

func main() {
	logrusutil.ComponentInit()

	o := flagOptions()
	flag.Parse()
	if err := o.validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid arguments.")
	}
	defer interrupts.WaitForGracefulShutdown()

	health := pjutil.NewHealth()
	health.ServeReady()

	r := mux.NewRouter()

	// Only send requests under the /repo/... path to git-http-backend. This way
	// we can have other paths (if necessary) to take in custom commands from
	// integration tests (e.g., "/admin/reset" to reset all repos back to their
	// original state).
	r.PathPrefix("/repo").Handler(gitCGIHandler(o.gitBinary, o.gitReposParentDir))

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", o.port),
		Handler: r,
	}

	if o.populateSampleRepos {
		err := mkSampleRepos(o.gitReposParentDir, o.fooRepoRemoteURL)
		if err != nil {
			logrus.Fatalf("failed to create sample repos: %v", err)
		}
	}

	if err := initRepos(o.gitReposParentDir); err != nil {
		logrus.Fatal(err)
	}

	logrus.Info("Start server")
	interrupts.ListenAndServe(server, 5*time.Second)
}

// gitCGIHandler returns an http.Handler that is backed by git-http-backend (a
// CGI executable). git-http-backend is the `git http-backend` subcommand that
// comes distributed with a default git installation.
func gitCGIHandler(gitBinary, gitReposParentDir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		h := &cgi.Handler{
			Path: gitBinary,
			Env: []string{
				"GIT_PROJECT_ROOT=" + gitReposParentDir,
				// Allow reading of all repos under gitReposParentDir.
				"GIT_HTTP_EXPORT_ALL=1",
			},
			Args: []string{
				"http-backend",
			},
		}
		// Remove the "/repo" prefix, because git-http-backend expects the
		// request to simply be the Git repo name.
		req.URL.Path = strings.TrimPrefix(string(req.URL.Path), "/repo")
		// It appears that this RequestURI field is not used; but for
		// completeness trim the prefix here as well.
		req.RequestURI = strings.TrimPrefix(req.RequestURI, "/repo")
		h.ServeHTTP(w, req)
	})
}

// initRepos sets common, sensible Git configuration options for all repos in
// gitReposParentDir.
func initRepos(gitReposParentDir string) error {
	files, err := ioutil.ReadDir(gitReposParentDir)
	if err != nil {
		return err
	}

	for _, f := range files {
		if !f.Mode().IsDir() {
			continue
		}
		repoPath := fmt.Sprintf("%s/%s", gitReposParentDir, f.Name())
		r, err := git.PlainOpen(repoPath)
		if err != nil {
			return err
		}
		if err := setGitConfigOptions(r); err != nil {
			return err
		}
	}
	return nil
}

func setGitConfigOptions(r *git.Repository) error {
	config, err := r.Config()
	if err != nil {
		return err
	}

	// Ensure that the given Git repo allows anonymous push access. This is
	// required for unauthenticated clients to push to the repo over HTTP.
	config.Raw.SetOption("http", "", "receivepack", "true")

	// Advertise all objects. This allows clients to fetch by raw commit SHAs,
	// avoiding the dreaded
	//
	// 		Server does not allow request for unadvertised object <SHA>
	//
	// error.
	config.Raw.SetOption("uploadpack", "", "allowAnySHA1InWant", "true")

	r.SetConfig(config)

	return nil
}

// mkSampleRepos creates sample Git repos under /git-repo. The created repos
// always have the same SHA because we set the Git name, email, and timestamp to
// a constant value in defaultSignature(). The repos created here are meant to
// be used for integrations tests.
//
// Having the repo information here (instead of in the integration test itself)
// helps us debug repo state without having to run the test.
func mkSampleRepos(gitReposParentDir, fooRepoRemoteURL string) error {
	err := mkSampleRepoFoo(gitReposParentDir)
	if err != nil {
		return fmt.Errorf("failed to create foo.git repo: %v", err)
	}

	err = mkSampleRepoBar(gitReposParentDir, fooRepoRemoteURL)
	if err != nil {
		return fmt.Errorf("failed to create bar.git repo: %v", err)
	}

	return nil
}

func mkSampleRepoFoo(gitReposParentDir string) error {
	repoPath := filepath.Join(gitReposParentDir, "foo.git")
	fs := osfs.New(repoPath)
	dot, err := fs.Chroot(".git")
	if err != nil {
		return err
	}
	storage := filesystem.NewStorage(dot, cache.NewObjectLRUDefault())

	r, err := git.Init(storage, fs)
	if err != nil {
		return err
	}

	// Create first commit.
	err = util.WriteFile(fs, "README.txt", []byte("hello\n"), 0755)
	if err != nil {
		return err
	}

	w, err := r.Worktree()
	if err != nil {
		return err
	}

	_, err = w.Add("README.txt")
	if err != nil {
		return err
	}

	_, err = w.Commit("commit 1", &git.CommitOptions{
		Author:    defaultSignature(),
		Committer: defaultSignature(),
	})
	if err != nil {
		return err
	}

	// Create second commit.
	err = util.WriteFile(fs, "README.txt", []byte("hello world!\n"), 0755)
	if err != nil {
		return err
	}

	_, err = w.Add("README.txt")
	if err != nil {
		return err
	}

	_, err = w.Commit("commit 2", &git.CommitOptions{
		Author:    defaultSignature(),
		Committer: defaultSignature(),
	})
	if err != nil {
		return err
	}

	commit2, err := r.Head()
	if err != nil {
		return err
	}

	// Create 3 separate PR references (GitHub style "refs/pull/<ID>/head"). We
	// take care to make sure that these PRs don't have a merge conflict. We can
	// of course also create PRs that would result in a merge conflict in the
	// future, if desired.
	prs := []struct {
		name   string
		number int
	}{
		{name: "PR1", number: 1},
		{name: "PR2", number: 2},
		{name: "PR3", number: 3},
	}

	for _, pr := range prs {
		// Use detached HEAD state to prevent modifying the master branch.
		err = w.Checkout(&git.CheckoutOptions{Hash: commit2.Hash()})
		if err != nil {
			return err
		}
		err = util.WriteFile(fs, pr.name+".txt", []byte(pr.name+"\n"), 0755)
		if err != nil {
			return err
		}

		_, err = w.Add(pr.name + ".txt")
		if err != nil {
			return err
		}

		_, err = w.Commit(pr.name, &git.CommitOptions{
			Author:    defaultSignature(),
			Committer: defaultSignature(),
		})
		if err != nil {
			return err
		}

		prRefName := plumbing.ReferenceName(fmt.Sprintf("refs/pull/%d/head", pr.number))
		headRef, err := r.Head()
		if err != nil {
			return err
		}
		ref := plumbing.NewHashReference(prRefName, headRef.Hash())
		err = r.Storer.SetReference(ref)
		if err != nil {
			return err
		}
	}

	// Check out master again. This moves HEAD back to it.
	err = w.Checkout(&git.CheckoutOptions{Branch: "refs/heads/master"})
	if err != nil {
		return err
	}

	// Convert to a bare repo.
	config, err := r.Config()
	if err != nil {
		return err
	}
	config.Core.IsBare = true
	r.SetConfig(config)

	globPattern := filepath.Join(repoPath, "*.txt")
	matches, err := filepath.Glob(globPattern)
	if err != nil {
		return err
	}
	for _, match := range matches {
		if err := os.RemoveAll(match); err != nil {
			return err
		}
	}

	err = os.Rename(filepath.Join(repoPath, ".git"), filepath.Join(gitReposParentDir, "/.git"))
	if err != nil {
		return err
	}
	err = os.Remove(repoPath)
	if err != nil {
		return err
	}
	err = os.Rename(filepath.Join(gitReposParentDir, ".git"), repoPath)
	if err != nil {
		return err
	}

	return nil
}

func mkSampleRepoBar(gitReposParentDir, fooRepoRemoteURL string) error {
	repoPath := filepath.Join(gitReposParentDir, "bar.git")
	fs := osfs.New(repoPath)
	dot, err := fs.Chroot(".git")
	if err != nil {
		return err
	}
	storage := filesystem.NewStorage(dot, cache.NewObjectLRUDefault())

	r, err := git.Init(storage, fs)
	if err != nil {
		return err
	}

	// Create first commit.
	err = util.WriteFile(fs, "bar.txt", []byte("bar\n"), 0755)
	if err != nil {
		return err
	}

	w, err := r.Worktree()
	if err != nil {
		return err
	}

	_, err = w.Add("bar.txt")
	if err != nil {
		return err
	}

	_, err = w.Commit("commit 1", &git.CommitOptions{
		Author:    defaultSignature(),
		Committer: defaultSignature(),
	})
	if err != nil {
		return err
	}

	// Create second commit.
	err = util.WriteFile(fs, "bar.txt", []byte("hello world!\n"), 0755)
	if err != nil {
		return err
	}

	_, err = w.Add("bar.txt")
	if err != nil {
		return err
	}

	_, err = w.Commit("commit 2", &git.CommitOptions{
		Author:    defaultSignature(),
		Committer: defaultSignature(),
	})
	if err != nil {
		return err
	}

	commit2, err := r.Head()
	if err != nil {
		return err
	}

	// Create 3 separate PR references (GitHub style "refs/pull/<ID>/head"). We
	// take care to make sure that these PRs don't have a merge conflict. We can
	// of course also create PRs that would result in a merge conflict in the
	// future, if desired.
	prs := []struct {
		name   string
		number int
	}{
		{name: "PR1", number: 1},
		{name: "PR2", number: 2},
		{name: "PR3", number: 3},
	}

	for _, pr := range prs {
		// Use detached HEAD state to prevent modifying the master branch.
		err = w.Checkout(&git.CheckoutOptions{Hash: commit2.Hash()})
		if err != nil {
			return err
		}
		err = util.WriteFile(fs, pr.name+".txt", []byte(pr.name+"\n"), 0755)
		if err != nil {
			return err
		}

		_, err = w.Add(pr.name + ".txt")
		if err != nil {
			return err
		}

		_, err = w.Commit(pr.name, &git.CommitOptions{
			Author:    defaultSignature(),
			Committer: defaultSignature(),
		})
		if err != nil {
			return err
		}

		prRefName := plumbing.ReferenceName(fmt.Sprintf("refs/pull/%d/head", pr.number))
		headRef, err := r.Head()
		if err != nil {
			return err
		}
		ref := plumbing.NewHashReference(prRefName, headRef.Hash())
		err = r.Storer.SetReference(ref)
		if err != nil {
			return err
		}
	}

	// Check out master again. This moves HEAD back to it.
	err = w.Checkout(&git.CheckoutOptions{Branch: "refs/heads/master"})
	if err != nil {
		return err
	}

	// Now add foo.git as a submodule. We add it using a local path (under
	// gitReposParentDir), but afterwards have to modify the module so that it
	// uses http://<hostname>.  We can't use a static hostname, because it
	// depends on whether this binary is running locally or in a Docker
	// container or in a KIND cluster. So we let the user choose with the
	// -foo-repo-remote-URL option (fooRepoRemoteURL here).
	//
	// Unfortunately go-git does not naively support the creation of submodules,
	// so we have to run git directly.
	cmd := exec.Command("git", "submodule", "add", filepath.Join(gitReposParentDir, "foo.git"))
	cmd.Dir = repoPath
	err = cmd.Run()
	if err != nil {
		return err
	}

	cmd = exec.Command("git", "submodule", "set-url", "--", "foo", fooRepoRemoteURL)
	cmd.Dir = repoPath
	err = cmd.Run()
	if err != nil {
		return err
	}

	_, err = w.Add(".gitmodules")
	if err != nil {
		return err
	}

	_, err = w.Commit("add submodule", &git.CommitOptions{
		Author:    defaultSignature(),
		Committer: defaultSignature(),
	})
	if err != nil {
		return err
	}

	// Convert to a bare repo.
	config, err := r.Config()
	if err != nil {
		return err
	}
	config.Core.IsBare = true
	r.SetConfig(config)

	err = os.Rename(filepath.Join(repoPath, ".git"), filepath.Join(gitReposParentDir, ".git"))
	if err != nil {
		return err
	}
	err = os.RemoveAll(repoPath)
	if err != nil {
		return err
	}
	err = os.Rename(filepath.Join(gitReposParentDir, ".git"), repoPath)
	if err != nil {
		return err
	}

	return nil
}

func defaultSignature() *object.Signature {
	when, err := time.Parse(object.DateFormat, "Thu May 19 12:34:56 2022 +0000")
	if err != nil {
		logrus.Fatal(err)
	}
	return &object.Signature{
		Name:  "abc",
		Email: "d@e.f",
		When:  when,
	}
}
