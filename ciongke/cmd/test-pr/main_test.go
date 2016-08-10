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

	"github.com/kubernetes/test-infra/ciongke/gcs/fakegcs"
	"github.com/kubernetes/test-infra/ciongke/kube/fakekube"
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
		Workspace: cloneDir,
		RepoName:  "repo",
		RepoURL:   gitDir,
		Branch:    "master",
		PRNumber:  1,
	}
	merged, head, err := tc.checkoutPR()
	if err != nil {
		t.Errorf("Expected no error when merging: %s", err)
	} else {
		if !merged {
			t.Error("Expected merge to be okay.")
		}
		if len(head) == 0 {
			t.Error("Expected non-empty head SHA.")
		}
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
		Workspace: cloneDir,
		RepoName:  "repo",
		RepoURL:   gitDir,
		Branch:    "master",
		PRNumber:  1,
	}
	merged, head, err := tc.checkoutPR()
	if err != nil {
		t.Errorf("Expected no error when merging: %s", err)
	} else {
		if merged {
			t.Error("Expected merge to not happen.")
		}
		if len(head) == 0 {
			t.Error("Expected non-empty head SHA.")
		}
	}
}

func TestUploadSource(t *testing.T) {
	dir, err := ioutil.TempDir("", "test-pr-upload")
	if err != nil {
		t.Fatalf("Error creating temp dir: %s", err)
	}
	defer os.RemoveAll(dir)
	if err := os.Mkdir(filepath.Join(dir, "repo"), os.ModePerm); err != nil {
		t.Fatalf("Error creating repo dir: %s", err)
	}
	gc := fakegcs.FakeClient{}
	tc := &testClient{
		Workspace:    dir,
		RepoName:     "repo",
		PRNumber:     5,
		SourceBucket: "sb",
		GCSClient:    &gc,
	}
	if err := tc.uploadSource(); err != nil {
		t.Fatalf("Didn't expect error uploading source: %s", err)
	}
	if b, _ := gc.Download("sb", "5.tar.gz"); len(b) == 0 {
		t.Fatalf("Expected non-empty tar in GCS")
	}
}

func TestStartTests(t *testing.T) {
	dir, err := ioutil.TempDir("", "test-pr-start-tests")
	if err != nil {
		t.Fatalf("Error creating temp dir: %s", err)
	}
	defer os.RemoveAll(dir)
	if err := os.Mkdir(filepath.Join(dir, "repo"), os.ModePerm); err != nil {
		t.Fatalf("Error creating repo dir: %s", err)
	}
	yml := `---
- name: test1
  description: this tests something
  image: img:1
- name: test2
  image: img:2`
	if err := ioutil.WriteFile(filepath.Join(dir, "repo", ".test.yml"), []byte(yml), os.ModePerm); err != nil {
		t.Fatalf("Error writing .test.yml: %s", err)
	}
	kc := fakekube.FakeClient{}
	tc := &testClient{
		Workspace:    dir,
		RunTestImage: "run-test:1",
		RepoName:     "repo",
		RepoOwner:    "kuber",
		PRNumber:     5,
		Namespace:    "default",
		SourceBucket: "sb",
		KubeClient:   &kc,
	}
	if err := tc.startTests("abcdef"); err != nil {
		t.Fatalf("Did not expect error starting tests: %s", err)
	}
	if len(kc.Jobs) != 2 {
		t.Fatalf("Expected two jobs to be created.")
	}
	// TODO: Validate the jobs.
}
