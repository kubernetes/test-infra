/*
Copyright 2014 The Kubernetes Authors All rights reserved.

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

// git-sync is a command that pull a git repository to a local directory.

package main // import "k8s.io/contrib/git-sync"

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"
)

var flRepo = flag.String("repo", envString("GIT_SYNC_REPO", ""), "git repo url")
var flBranch = flag.String("branch", envString("GIT_SYNC_BRANCH", "master"), "git branch")
var flRev = flag.String("rev", envString("GIT_SYNC_REV", "HEAD"), "git rev")
var flDest = flag.String("dest", envString("GIT_SYNC_DEST", ""), "destination path")
var flWait = flag.Int("wait", envInt("GIT_SYNC_WAIT", 0), "number of seconds to wait before next sync")
var flOneTime = flag.Bool("one-time", envBool("GIT_SYNC_ONE_TIME", false), "exit after the initial checkout")
var flDepth = flag.Int("depth", envInt("GIT_SYNC_DEPTH", 0), "shallow clone with a history truncated to the specified number of commits")

var flMaxSyncFailures = flag.Int("max-sync-failures", envInt("GIT_SYNC_MAX_SYNC_FAILURES", 0),
	`number of consecutive failures allowed before aborting (the first pull must succeed)`)

var flUsername = flag.String("username", envString("GIT_SYNC_USERNAME", ""), "username")
var flPassword = flag.String("password", envString("GIT_SYNC_PASSWORD", ""), "password")

var flChmod = flag.Int("change-permissions", envInt("GIT_SYNC_PERMISSIONS", 0), `If set it will change the permissions of the directory 
		that contains the git repository. Example: 744`)

func envString(key, def string) string {
	if env := os.Getenv(key); env != "" {
		return env
	}
	return def
}

func envBool(key string, def bool) bool {
	if env := os.Getenv(key); env != "" {
		res, err := strconv.ParseBool(env)
		if err != nil {
			return def
		}

		return res
	}
	return def
}

func envInt(key string, def int) int {
	if env := os.Getenv(key); env != "" {
		val, err := strconv.Atoi(env)
		if err != nil {
			log.Printf("invalid value for %q: using default: %q", key, def)
			return def
		}
		return val
	}
	return def
}

const usage = "usage: GIT_SYNC_REPO= GIT_SYNC_DEST= [GIT_SYNC_BRANCH= GIT_SYNC_WAIT= GIT_SYNC_DEPTH= GIT_SYNC_USERNAME= GIT_SYNC_PASSWORD= GIT_SYNC_ONE_TIME= GIT_SYNC_MAX_SYNC_FAILURES=] git-sync -repo GIT_REPO_URL -dest PATH [-branch -wait -username -password -depth -one-time -max-sync-failures]"

func main() {
	flag.Parse()
	if *flRepo == "" || *flDest == "" {
		flag.Usage()
		log.Fatal(usage)
	}
	if _, err := exec.LookPath("git"); err != nil {
		log.Fatalf("required git executable not found: %v", err)
	}

	if *flUsername != "" && *flPassword != "" {
		if err := setupGitAuth(*flUsername, *flPassword, *flRepo); err != nil {
			log.Fatalf("error creating .netrc file: %v", err)
		}
	}

	initialSync := true
	failCount := 0
	for {
		if err := syncRepo(*flRepo, *flDest, *flBranch, *flRev, *flDepth); err != nil {
			if initialSync || failCount >= *flMaxSyncFailures {
				log.Fatalf("error syncing repo: %v", err)
			}

			failCount++
			log.Printf("unexpected error syncing repo: %v", err)
			log.Printf("waiting %d seconds before retryng", *flWait)
			time.Sleep(time.Duration(*flWait) * time.Second)
			continue
		}

		initialSync = false
		failCount = 0

		if *flOneTime {
			os.Exit(0)
		}

		log.Printf("waiting %d seconds", *flWait)
		time.Sleep(time.Duration(*flWait) * time.Second)
		log.Println("done")
	}
}

// syncRepo syncs the branch of a given repository to the destination at the given rev.
func syncRepo(repo, dest, branch, rev string, depth int) error {
	gitRepoPath := path.Join(dest, ".git")
	_, err := os.Stat(gitRepoPath)
	switch {
	case os.IsNotExist(err):
		// clone repo
		args := []string{"clone", "--no-checkout", "-b", branch}
		if depth != 0 {
			args = append(args, "-depth")
			args = append(args, string(depth))
		}
		args = append(args, repo)
		args = append(args, dest)
		output, err := runCommand("git", "", args)
		if err != nil {
			return err
		}

		log.Printf("clone %q: %s", repo, string(output))
	case err != nil:
		return fmt.Errorf("error checking if repo exist %q: %v", gitRepoPath, err)
	}

	// fetch branch
	output, err := runCommand("git", dest, []string{"pull", "origin", branch})
	if err != nil {
		return err
	}

	log.Printf("fetch %q: %s", branch, string(output))

	// reset working copy
	output, err = runCommand("git", dest, []string{"reset", "--hard", rev})
	if err != nil {
		return err
	}

	log.Printf("reset %q: %v", rev, string(output))

	if *flChmod != 0 {
		// set file permissions
		_, err = runCommand("chmod", "", []string{"-R", string(*flChmod), dest})
		if err != nil {
			return err
		}
	}

	return nil
}

func runCommand(command, cwd string, args []string) ([]byte, error) {
	cmd := exec.Command(command, args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return []byte{}, fmt.Errorf("error running command %q : %v: %s", strings.Join(cmd.Args, " "), err, string(output))
	}

	return output, nil
}

func setupGitAuth(username, password, gitURL string) error {
	log.Println("setting up the git credential cache")
	cmd := exec.Command("git", "config", "--global", "credential.helper", "cache")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error setting up git credentials %v: %s", err, string(output))
	}

	cmd = exec.Command("git", "credential", "approve")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	creds := fmt.Sprintf("url=%v\nusername=%v\npassword=%v\n", gitURL, username, password)
	io.Copy(stdin, bytes.NewBufferString(creds))
	stdin.Close()
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error setting up git credentials %v: %s", err, string(output))
	}

	return nil
}
