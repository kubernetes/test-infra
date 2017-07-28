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

package git_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"k8s.io/test-infra/prow/git/localgit"
)

func TestClone(t *testing.T) {
	lg, c, err := localgit.New()
	if err != nil {
		t.Fatalf("Making local git repo: %v", err)
	}
	defer func() {
		if err := lg.Clean(); err != nil {
			t.Errorf("Error cleaning LocalGit: %v", err)
		}
		if err := c.Clean(); err != nil {
			t.Errorf("Error cleaning Client: %v", err)
		}
	}()
	if err := lg.MakeFakeRepo("foo", "bar"); err != nil {
		t.Fatalf("Making fake repo: %v", err)
	}
	if err := lg.MakeFakeRepo("foo", "baz"); err != nil {
		t.Fatalf("Making fake repo: %v", err)
	}

	// Fresh clone, will be a cache miss.
	r1, err := c.Clone("foo/bar")
	if err != nil {
		t.Fatalf("Cloning the first time: %v", err)
	}
	defer func() {
		if err := r1.Clean(); err != nil {
			t.Errorf("Cleaning repo: %v", err)
		}
	}()

	// Clone from the same org.
	r2, err := c.Clone("foo/baz")
	if err != nil {
		t.Fatalf("Cloning another repo in the same org: %v", err)
	}
	defer func() {
		if err := r2.Clean(); err != nil {
			t.Errorf("Cleaning repo: %v", err)
		}
	}()

	// Make sure it fetches when we clone again.
	if err := lg.AddCommit("foo", "bar", map[string][]byte{"second": {}}); err != nil {
		t.Fatalf("Adding second commit: %v", err)
	}
	r3, err := c.Clone("foo/bar")
	if err != nil {
		t.Fatalf("Cloning a second time: %v", err)
	}
	defer func() {
		if err := r3.Clean(); err != nil {
			t.Errorf("Cleaning repo: %v", err)
		}
	}()
	log := exec.Command("git", "log", "--oneline")
	log.Dir = r3.Dir
	if b, err := log.CombinedOutput(); err != nil {
		t.Fatalf("git log: %v, %s", err, string(b))
	} else {
		t.Logf("git log output: %s", string(b))
		if len(bytes.Split(bytes.TrimSpace(b), []byte("\n"))) != 2 {
			t.Error("Wrong number of commits in git log output. Expected 2")
		}
	}
}

func TestCheckoutPR(t *testing.T) {
	lg, c, err := localgit.New()
	if err != nil {
		t.Fatalf("Making local git repo: %v", err)
	}
	defer func() {
		if err := lg.Clean(); err != nil {
			t.Errorf("Error cleaning LocalGit: %v", err)
		}
		if err := c.Clean(); err != nil {
			t.Errorf("Error cleaning Client: %v", err)
		}
	}()
	if err := lg.MakeFakeRepo("foo", "bar"); err != nil {
		t.Fatalf("Making fake repo: %v", err)
	}
	r, err := c.Clone("foo/bar")
	if err != nil {
		t.Fatalf("Cloning: %v", err)
	}
	defer func() {
		if err := r.Clean(); err != nil {
			t.Errorf("Cleaning repo: %v", err)
		}
	}()

	if err := lg.CheckoutNewBranch("foo", "bar", "pull/123/head"); err != nil {
		t.Fatalf("Checkout new branch: %v", err)
	}
	if err := lg.AddCommit("foo", "bar", map[string][]byte{"wow": {}}); err != nil {
		t.Fatalf("Add commit: %v", err)
	}

	if err := r.CheckoutPullRequest(123); err != nil {
		t.Fatalf("Checking out PR: %v", err)
	}
	if _, err := os.Stat(filepath.Join(r.Dir, "wow")); err != nil {
		t.Errorf("Didn't find file in PR after checking out: %v", err)
	}
}
