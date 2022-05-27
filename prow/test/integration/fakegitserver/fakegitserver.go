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
	"encoding/json"
	"errors"
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

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/test/integration/lib/fakegitserver"
)

type options struct {
	port              int
	gitBinary         string
	gitReposParentDir string
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
	r.PathPrefix("/setup-repo").Handler(setupRepoHandler(o.gitReposParentDir))

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", o.port),
		Handler: r,
	}

	logrus.Info("Start server")
	interrupts.ListenAndServe(server, 5*time.Second)
}

// setupRepoHandler executes a JSON payload of instructions to set up a Git
// repo.
func setupRepoHandler(gitReposParentDir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		buf, err := ioutil.ReadAll(req.Body)
		defer req.Body.Close()
		if err != nil {
			logrus.Errorf("failed to read request body: %v", err)
			http.Error(w, err.Error(), 500)
			return
		}

		logrus.Infof("request body received: %v", string(buf))
		var repoSetup fakegitserver.RepoSetup
		err = json.Unmarshal(buf, &repoSetup)
		if err != nil {
			logrus.Errorf("failed to parse request body as FGSRepoSetup: %v", err)
			http.Error(w, err.Error(), 500)
			return
		}

		repo, err := setupRepo(gitReposParentDir, &repoSetup)
		if err != nil {
			// Just log the error if the setup fails so that the developer can
			// fix their error and retry without having to restart this server.
			logrus.Errorf("failed to setup repo: %v", err)
			http.Error(w, err.Error(), 500)
			return
		}

		msg, err := getLog(repo)
		if err != nil {
			logrus.Errorf("failed to get repo stats: %v", err)
			http.Error(w, err.Error(), 500)
			return
		}
		fmt.Fprintf(w, "%s", msg)
	})
}

// getLog creates a report of Git repo statistics.
func getLog(repo *git.Repository) (string, error) {

	var stats string

	// Show `git log --all` equivalent.
	ref, err := repo.Head()
	if err != nil {
		return "", errors.New("could not get HEAD")
	}
	commits, err := repo.Log(&git.LogOptions{From: ref.Hash(), All: true})
	if err != nil {
		return "", errors.New("could not get git logs")
	}
	err = commits.ForEach(func(commit *object.Commit) error {
		stats += fmt.Sprintln(commit)
		return nil
	})

	return stats, nil
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

func setupRepo(gitReposParentDir string, repoSetup *fakegitserver.RepoSetup) (*git.Repository, error) {
	dir := filepath.Join(gitReposParentDir, repoSetup.Name+".git")
	logger := logrus.WithField("directory", dir)

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		if repoSetup.Overwrite {
			if err := os.RemoveAll(dir); err != nil {
				logger.Error("(overwrite) could not remove directory")
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("path %s already exists but overwrite is not enabled; aborting", dir)
		}
	}

	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		logger.Error("could not create directory")
		return nil, err
	}

	repo, err := git.PlainInit(dir, false)
	if err != nil {
		logger.Error("could not initialize git repo in directory")
		return nil, err
	}

	if err := setGitConfigOptions(repo); err != nil {
		logger.Error("config setup failed")
		return nil, err
	}

	if err := runSetupScript(dir, repoSetup.Script); err != nil {
		logger.Error("running the repo setup script failed")
		return nil, err
	}

	if err := convertToBareRepo(repo, dir); err != nil {
		logger.Error("conversion to bare repo failed")
		return nil, err
	}

	repo, err = git.PlainOpen(dir)
	if err != nil {
		logger.Error("could not reopen repo")
		return nil, err
	}
	return repo, nil
}

func convertToBareRepo(repo *git.Repository, repoPath string) error {
	// Convert to a bare repo.
	config, err := repo.Config()
	if err != nil {
		return err
	}
	config.Core.IsBare = true
	repo.SetConfig(config)

	tempDir, err := ioutil.TempDir("", "fgs")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	// Move "<REPO>/.git" directory to a temporary directory.
	err = os.Rename(filepath.Join(repoPath, ".git"), filepath.Join(tempDir, ".git"))
	if err != nil {
		return err
	}

	// Delete <REPO> folder. This takes care of deleting all worktree files.
	err = os.RemoveAll(repoPath)
	if err != nil {
		return err
	}

	// Move the .git folder to the <REPO> folder path.
	err = os.Rename(filepath.Join(tempDir, ".git"), repoPath)
	if err != nil {
		return err
	}
	return nil
}

func runSetupScript(repoPath, script string) error {
	// Catch errors in the script.
	script = "set -eu;" + script

	logrus.Infof("setup script looks like: %v", script)

	cmd := exec.Command("sh", "-c", script)
	cmd.Dir = repoPath

	// By default, make it so that the git commands contained in the script
	// result in reproducible commits. This can be overridden by the script
	// itself if it chooses to (re-)export the same environment variables.
	cmd.Env = []string{
		"GIT_AUTHOR_NAME=abc",
		"GIT_AUTHOR_EMAIL=d@e.f",
		"GIT_AUTHOR_DATE='Thu May 19 12:34:56 2022 +0000'",
		"GIT_COMMITTER_NAME=abc",
		"GIT_COMMITTER_EMAIL=d@e.f",
		"GIT_COMMITTER_DATE='Thu May 19 12:34:56 2022 +0000'"}

	return cmd.Run()
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
