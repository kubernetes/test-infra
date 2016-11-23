/*
Copyright 2016 The Kubernetes Authors.

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
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func expectEqual(t *testing.T, msg string, have interface{}, want interface{}) {
	if !reflect.DeepEqual(have, want) {
		t.Errorf("bad %s: got %v, wanted %v",
			msg, have, want)
	}
}

type stringHandler string

func (h stringHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "%s", h)
}

func TestGetQueuedPRs(t *testing.T) {
	body := `{"E2EQueue":[
		{"Number":3, "Title": "blah"},
		{"Number":4},
		{"Number":1}
	]}`
	serv := httptest.NewServer(stringHandler(body))
	defer serv.Close()
	q, err := getQueuedPRs(serv.URL)
	if err != nil {
		t.Fatal(err)
	}
	expectEqual(t, "queued PRs", q, []int{3, 4, 1})
}

// Since the splicer object already has helpers for doing git operations,
// extend it to be useful for producing git repos for testing!

// branch creates a new branch off of master
func (s *splicer) branch(name string) error {
	return s.gitCall("checkout", "-b", name, "master")
}

// commit makes a new commit with the provided files added.
func (s *splicer) commit(msg string, contents map[string]string) error {
	for fname, data := range contents {
		err := ioutil.WriteFile(s.dir+"/"+fname, []byte(data), 0644)
		if err != nil {
			return err
		}
		err = s.gitCall("add", fname)
		if err != nil {
			return err
		}
	}
	return s.gitCall("commit", "-m", msg)
}

// Create a basic commit (so master can be branched off)
func (s *splicer) firstCommit() error {
	return s.commit("first commit", map[string]string{"README": "hi"})
}

type branchesSpec map[string]map[string]string

// addBranches does multiple branch/commit calls.
func (s *splicer) addBranches(b branchesSpec) error {
	for name, contents := range b {
		err := s.branch(name)
		if err != nil {
			return err
		}
		err = s.commit("msg", contents)
		if err != nil {
			return err
		}
	}
	return nil
}

func TestGitOperations(t *testing.T) {
	s, err := makeSplicer()
	if err != nil {
		t.Fatal(err)
	}
	defer s.cleanup()
	err = s.firstCommit()
	if err != nil {
		t.Fatal(err)
	}
	err = s.addBranches(branchesSpec{
		"pr/123": {"a": "1", "b": "2"},
		"pr/456": {"a": "1", "b": "4", "c": "e"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFindMergeable(t *testing.T) {
	up, _ := makeSplicer()
	defer up.cleanup()
	up.firstCommit()
	err := up.addBranches(branchesSpec{
		"pull/1/head": {"a": "1"},
		"pull/2/head": {"b": "2"},
		"pull/3/head": {"a": "1", "b": "2", "c": "3"},
		"pull/4/head": {"a": "5"},
	})
	if err != nil {
		t.Fatal(err)
	}

	s, _ := makeSplicer()
	defer s.cleanup()
	mergeable, err := s.findMergeable(up.dir, []int{3, 2, 1, 4})
	if err != nil {
		t.Fatal(err)
	}
	expectEqual(t, "mergeable PRs", mergeable, []int{3, 2, 1})

	// findMergeable should work if repeated-- the repo should be
	// reset into a state so it can try to merge again.
	mergeable, err = s.findMergeable(up.dir, []int{3, 2, 1, 4})
	expectEqual(t, "mergeable PRs", mergeable, []int{3, 2, 1})
}
