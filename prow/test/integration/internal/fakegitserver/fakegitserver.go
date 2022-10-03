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

package fakegitserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cgi"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/sirupsen/logrus"
)

type Client struct {
	host       string
	httpClient *http.Client
}

type RepoSetup struct {
	// Name of the Git repo. It will get a ".git" appended to it and be
	// initialized underneath o.gitReposParentDir.
	Name string `json:"name"`
	// Script to execute. This script runs inside the repo to perform any
	// additional repo setup tasks. This script is executed by /bin/sh.
	Script string `json:"script"`
	// Whether to create the repo at the path (o.gitReposParentDir + name +
	// ".git") even if a file (directory) exists there already. This basically
	// does a 'rm -rf' of the folder first.
	Overwrite bool `json:"overwrite"`
}

func NewClient(host string, timeout time.Duration) *Client {
	return &Client{
		host: host,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) do(method, endpoint string, payload []byte, params map[string]string) (*http.Response, error) {
	baseURL := fmt.Sprintf("%s/%s", c.host, endpoint)
	req, err := http.NewRequest(method, baseURL, bytes.NewBuffer(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json; charset=UTF-8")
	q := req.URL.Query()
	for key, val := range params {
		q.Set(key, val)
	}
	req.URL.RawQuery = q.Encode()
	return c.httpClient.Do(req)
}

// SetupRepo sends a POST request with the RepoSetup contents.
func (c *Client) SetupRepo(repoSetup RepoSetup) error {
	buf, err := json.Marshal(repoSetup)
	if err != nil {
		return fmt.Errorf("could not marshal %v", repoSetup)
	}

	resp, err := c.do(http.MethodPost, "setup-repo", buf, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("got %v response", resp.StatusCode)
	}
	return nil
}

// GitCGIHandler returns an http.Handler that is backed by git-http-backend (a
// CGI executable). git-http-backend is the `git http-backend` subcommand that
// comes distributed with a default git installation.
func GitCGIHandler(gitBinary, gitReposParentDir string) http.Handler {
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

// SetupRepoHandler executes a JSON payload of instructions to set up a Git
// repo.
func SetupRepoHandler(gitReposParentDir string, mux *sync.Mutex) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		buf, err := io.ReadAll(req.Body)
		defer req.Body.Close()
		if err != nil {
			logrus.Errorf("failed to read request body: %v", err)
			http.Error(w, err.Error(), 500)
			return
		}

		logrus.Infof("request body received: %v", string(buf))
		var repoSetup RepoSetup
		err = json.Unmarshal(buf, &repoSetup)
		if err != nil {
			logrus.Errorf("failed to parse request body as FGSRepoSetup: %v", err)
			http.Error(w, err.Error(), 500)
			return
		}

		// setupRepo might need to access global git config, concurrent
		// modifications of which could result in error like "exit status 255
		// error: could not lock config file /root/.gitconfig". Use a mux to
		// avoid this
		repo, err := setupRepo(gitReposParentDir, &repoSetup, mux)
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

func setupRepo(gitReposParentDir string, repoSetup *RepoSetup, mux *sync.Mutex) (*git.Repository, error) {
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

	// Steps below might update global git config, lock it to avoid collision.
	mux.Lock()
	defer mux.Unlock()
	if err := setGitConfigOptions(repo); err != nil {
		logger.Error("config setup failed")
		return nil, err
	}

	if err := runSetupScript(dir, repoSetup.Script); err != nil {
		logger.Error("running the repo setup script failed")
		return nil, err
	}

	logger.Infof("successfully ran setup script in %s", dir)

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

func convertToBareRepo(repo *git.Repository, repoPath string) error {
	// Convert to a bare repo.
	config, err := repo.Config()
	if err != nil {
		return err
	}
	config.Core.IsBare = true
	repo.SetConfig(config)

	tempDir, err := os.MkdirTemp("", "fgs")
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

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

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
