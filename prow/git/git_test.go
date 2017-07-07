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

package git

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// runCmd runs cmd in dir with arg.
func runCmd(cmd, dir string, arg ...string) error {
	c := exec.Command(cmd, arg...)
	c.Dir = dir
	if b, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %v: %v, %s", cmd, arg, err, string(b))
	}
	return nil
}

func TestClone(t *testing.T) {
	c, err := NewClient()
	if err != nil {
		t.Fatalf("Making client: %v", err)
	}
	defer func() {
		if err := c.Clean(); err != nil {
			t.Errorf("Cleaning up old client: %v", err)
		}
	}()
	// Make fake local repositories in a temp dir.
	tmp, err := ioutil.TempDir("", "test")
	if err != nil {
		t.Fatalf("Making tmpdir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tmp); err != nil {
			t.Errorf("Cleaning up tmp dir: %v", err)
		}
	}()
	c.base = tmp
	if err := makeFakeRepo(c.git, tmp, "foo", "bar"); err != nil {
		t.Fatalf("Making fake repo: %v", err)
	}
	if err := makeFakeRepo(c.git, tmp, "foo", "baz"); err != nil {
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
	if err := addCommit(c.git, filepath.Join(tmp, "foo", "bar"), "second"); err != nil {
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
	log.Dir = r3.dir
	if b, err := log.CombinedOutput(); err != nil {
		t.Fatalf("git log: %v, %s", err, string(b))
	} else {
		t.Logf("git log output: %s", string(b))
		if len(bytes.Split(bytes.TrimSpace(b), []byte("\n"))) != 2 {
			t.Error("Wrong number of commits in git log output. Expected 2")
		}
	}
}

func makeFakeRepo(git, tmp, org, repo string) error {
	rdir := filepath.Join(tmp, org, repo)
	if err := os.MkdirAll(rdir, os.ModePerm); err != nil {
		return err
	}

	if err := runCmd(git, rdir, "init"); err != nil {
		return err
	}
	if err := runCmd(git, rdir, "config", "user.email", "test@test.test"); err != nil {
		return err
	}
	if err := runCmd(git, rdir, "config", "user.name", "test test"); err != nil {
		return err
	}
	if err := addCommit(git, rdir, "initial"); err != nil {
		return err
	}

	return nil
}

func addCommit(git, rdir, name string) error {
	if err := ioutil.WriteFile(filepath.Join(rdir, name), []byte("wow!"), os.ModePerm); err != nil {
		return err
	}

	if err := runCmd(git, rdir, "add", name); err != nil {
		return err
	}
	if err := runCmd(git, rdir, "commit", "-m", "wow"); err != nil {
		return err
	}
	return nil
}
