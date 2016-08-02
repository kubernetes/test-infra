/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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

package main

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func runTestCommand(t *testing.T, dir, name string, arg ...string) {
	cmd := exec.Command(name, arg...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Error running %s %v command: %s\n%s", name, arg, err, string(out))
	}
	if len(out) > 0 {
		t.Log(string(out))
	}
}

func TestCheckoutPRMergeable(t *testing.T) {
	// Create a testing repo here.
	gitDir, err := ioutil.TempDir("", "test-pr-git")
	if err != nil {
		t.Fatalf("Error creating temp dir: %s", err)
	}
	defer os.RemoveAll(gitDir)
	// The clone will go in here.
	cloneDir, err := ioutil.TempDir("", "test-pr-clone")
	if err != nil {
		t.Fatalf("Error creating temp dir: %s", err)
	}
	defer os.RemoveAll(cloneDir)

	// Set up a git repo where PR 1 can be merged.
	runTestCommand(t, gitDir, "git", "init")
	runTestCommand(t, gitDir, "git", "config", "user.name", "test")
	runTestCommand(t, gitDir, "git", "config", "user.email", "test@test.test")
	runTestCommand(t, gitDir, "touch", "a")
	runTestCommand(t, gitDir, "git", "add", "a")
	runTestCommand(t, gitDir, "git", "commit", "-m", "\"first commit\"")
	runTestCommand(t, gitDir, "git", "checkout", "-b", "pr-1")
	runTestCommand(t, gitDir, "touch", "b")
	runTestCommand(t, gitDir, "git", "add", "b")
	runTestCommand(t, gitDir, "git", "commit", "-m", "\"touch b\"")
	runTestCommand(t, gitDir, "git", "update-ref", "refs/heads/pull/1/head", "pr-1")

	tc := &testClient{
		Workspace:   cloneDir,
		RepoName:    "repo",
		RepoURL:     gitDir,
		Branch:      "master",
		PRNumber:    1,
		ExecCommand: exec.Command,
	}
	merged, err := tc.checkoutPR()
	if err != nil {
		t.Errorf("Expected no error when merging: %s", err)
	} else if !merged {
		t.Error("Expected merge to be okay.")
	}
}

func TestCheckoutPRUnmergable(t *testing.T) {
	// Create a testing repo here.
	gitDir, err := ioutil.TempDir("", "test-pr-git")
	if err != nil {
		t.Fatalf("Error creating temp dir: %s", err)
	}
	defer os.RemoveAll(gitDir)
	// The clone will go in here.
	cloneDir, err := ioutil.TempDir("", "test-pr-clone")
	if err != nil {
		t.Fatalf("Error creating temp dir: %s", err)
	}
	defer os.RemoveAll(cloneDir)

	// Set up a git repo where PR 1 can't be merged.
	runTestCommand(t, gitDir, "git", "init")
	runTestCommand(t, gitDir, "git", "config", "user.name", "test")
	runTestCommand(t, gitDir, "git", "config", "user.email", "test@test.test")
	runTestCommand(t, gitDir, "touch", "a")
	runTestCommand(t, gitDir, "git", "add", "a")
	runTestCommand(t, gitDir, "git", "commit", "-m", "\"first commit\"")
	runTestCommand(t, gitDir, "git", "checkout", "-b", "pr-1")
	if err = ioutil.WriteFile(filepath.Join(gitDir, "a"), []byte("hello"), 0); err != nil {
		t.Fatalf("Could not write file: %s", err)
	}
	runTestCommand(t, gitDir, "git", "add", "a")
	runTestCommand(t, gitDir, "git", "commit", "-m", "\"write hello\"")
	runTestCommand(t, gitDir, "git", "update-ref", "refs/heads/pull/1/head", "pr-1")
	runTestCommand(t, gitDir, "git", "checkout", "master")
	if err = ioutil.WriteFile(filepath.Join(gitDir, "a"), []byte("world"), 0); err != nil {
		t.Fatalf("Could not write file: %s", err)
	}
	runTestCommand(t, gitDir, "git", "add", "a")
	runTestCommand(t, gitDir, "git", "commit", "-m", "\"write world\"")

	tc := &testClient{
		Workspace:   cloneDir,
		RepoName:    "repo",
		RepoURL:     gitDir,
		Branch:      "master",
		PRNumber:    1,
		ExecCommand: exec.Command,
	}
	merged, err := tc.checkoutPR()
	if err != nil {
		t.Errorf("Expected no error when merging: %s", err)
	} else if merged {
		t.Error("Expected merge to not happen.")
	}
}
