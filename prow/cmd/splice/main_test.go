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

	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
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
		{"Number":4, "BaseRef": "master"},
		{"Number":1},
		{"Number":5, "BaseRef": "release-1.5"}
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
	return s.gitCall("checkout", "-B", name, "master")
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
		"pull/1/head": {"a": "1", "e": "1"},
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
	if err != nil {
		t.Fatal(err)
	}
	expectEqual(t, "mergeable PRs", mergeable, []int{3, 2, 1})

	// PRs that cause merge conflicts should be skipped
	mergeable, err = s.findMergeable(up.dir, []int{1, 4, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	expectEqual(t, "mergeable PRs", mergeable, []int{1, 2, 3})

	// doing a force push should work as well!
	err = up.addBranches(branchesSpec{
		"pull/2/head": {"b": "2", "e": "2"}, // now conflicts with 1
	})
	if err != nil {
		t.Fatal(err)
	}
	mergeable, err = s.findMergeable(up.dir, []int{3, 2, 1, 4})
	if err != nil {
		t.Fatal(err)
	}
	expectEqual(t, "mergeable PRs", mergeable, []int{3, 2})

}

func fakeRefs(ref, sha string) kube.Refs {
	return kube.Refs{
		BaseRef: ref,
		BaseSHA: sha,
	}
}

func fakeProwJob(context string, jobType kube.ProwJobType, completed bool, state kube.ProwJobState, refs kube.Refs) kube.ProwJob {
	pj := kube.ProwJob{
		Status: kube.ProwJobStatus{
			State: state,
		},
		Spec: kube.ProwJobSpec{
			Context: context,
			Refs:    &refs,
			Type:    jobType,
		},
	}
	if completed {
		pj.SetComplete()
	}
	return pj
}

func TestCompletedJobs(t *testing.T) {
	refs := fakeRefs("ref", "sha")
	other := fakeRefs("otherref", "othersha")
	tests := []struct {
		name      string
		jobs      []kube.ProwJob
		refs      kube.Refs
		completed []string
	}{
		{
			name: "completed when passed",
			jobs: []kube.ProwJob{
				fakeProwJob("passed-a", kube.BatchJob, true, kube.SuccessState, refs),
				fakeProwJob("passed-b", kube.BatchJob, true, kube.SuccessState, refs),
			},
			refs:      refs,
			completed: []string{"passed-a", "passed-b"},
		},
		{
			name: "ignore bad ref",
			jobs: []kube.ProwJob{
				fakeProwJob("passed-a", kube.BatchJob, true, kube.SuccessState, other),
			},
			refs: refs,
		},
		{
			name: "only complete good refs",
			jobs: []kube.ProwJob{
				fakeProwJob("passed-a", kube.BatchJob, true, kube.SuccessState, refs),
				fakeProwJob("passed-b-bad-ref", kube.BatchJob, true, kube.SuccessState, other),
			},
			refs:      refs,
			completed: []string{"passed-a"},
		},
		{
			name: "completed when good and bad ref",
			jobs: []kube.ProwJob{
				fakeProwJob("passed-a", kube.BatchJob, true, kube.SuccessState, refs),
				fakeProwJob("passed-a", kube.BatchJob, true, kube.SuccessState, other),
			},
			refs:      refs,
			completed: []string{"passed-a"},
		},
		{
			name: "ignore incomplete",
			jobs: []kube.ProwJob{
				fakeProwJob("passed-a", kube.BatchJob, true, kube.SuccessState, refs),
				fakeProwJob("pending-b", kube.BatchJob, false, kube.PendingState, refs),
			},
			refs:      refs,
			completed: []string{"passed-a"},
		},
		{
			name: "ignore failed",
			jobs: []kube.ProwJob{
				fakeProwJob("passed-a", kube.BatchJob, true, kube.SuccessState, refs),
				fakeProwJob("failed-b", kube.BatchJob, true, kube.FailureState, refs),
			},
			refs:      refs,
			completed: []string{"passed-a"},
		},
		{
			name: "ignore non-batch",
			jobs: []kube.ProwJob{
				fakeProwJob("passed-a", kube.BatchJob, true, kube.SuccessState, refs),
				fakeProwJob("non-batch-b", kube.PresubmitJob, true, kube.SuccessState, refs),
			},
			refs:      refs,
			completed: []string{"passed-a"},
		},
	}

	for _, tc := range tests {
		completed := completedJobs(tc.jobs, tc.refs)
		var completedContexts []string
		for _, job := range completed {
			completedContexts = append(completedContexts, job.Spec.Context)
		}
		expectEqual(t, "completed contexts", completedContexts, tc.completed)
	}
}

func TestRequiredPresubmits(t *testing.T) {
	tests := []struct {
		name       string
		possible   []config.Presubmit
		required   []string
		overridden sets.String
	}{
		{
			name: "basic",
			possible: []config.Presubmit{
				{
					Name:      "always",
					AlwaysRun: true,
				},
				{
					Name:      "optional",
					AlwaysRun: false,
				},
				{
					Name:       "hidden",
					AlwaysRun:  true,
					SkipReport: true,
				},
				{
					Name:      "optional_but_overridden",
					AlwaysRun: false,
				},
			},
			required:   []string{"always", "optional_but_overridden"},
			overridden: sets.NewString("optional_but_overridden"),
		},
	}

	for _, tc := range tests {
		var names []string
		for _, job := range requiredPresubmits(tc.possible, tc.overridden) {
			names = append(names, job.Name)
		}
		expectEqual(t, tc.name, names, tc.required)
	}
}

func TestNeededPresubmits(t *testing.T) {
	tests := []struct {
		name     string
		possible []config.Presubmit
		current  []kube.ProwJob
		refs     kube.Refs
		required []string
	}{
		{
			name: "basic",
			possible: []config.Presubmit{
				{
					Name:      "always",
					AlwaysRun: true,
				},
				{
					Name:      "optional",
					AlwaysRun: false,
				},
				{
					Name:       "hidden",
					AlwaysRun:  true,
					SkipReport: true,
				},
			},
			required: []string{"always"},
		},
		{
			name: "skip already passed",
			possible: []config.Presubmit{
				{
					Name:      "new",
					Context:   "brandnew",
					AlwaysRun: true,
				},
				{
					Name:      "passed",
					Context:   "already-ran",
					AlwaysRun: true,
				},
			},
			current: []kube.ProwJob{
				fakeProwJob("already-ran", kube.BatchJob, true, kube.SuccessState, fakeRefs("ref", "sha")),
			},
			refs:     fakeRefs("ref", "sha"),
			required: []string{"new"},
		},
		{
			name: "handle branches/skipbranches specifiers",
			possible: []config.Presubmit{
				{
					Name:      "old",
					Brancher:  config.Brancher{Branches: []string{"release-1.2", "release-1.3"}},
					AlwaysRun: true,
				},
				{
					Name:      "outdated",
					Brancher:  config.Brancher{SkipBranches: []string{"master"}},
					AlwaysRun: true,
				},
				{
					Name:      "latest",
					Brancher:  config.Brancher{Branches: []string{"master"}},
					AlwaysRun: true,
				},
			},
			required: []string{"latest"},
		},
	}

	for _, tc := range tests {
		if err := config.SetPresubmitRegexes(tc.possible); err != nil {
			t.Fatalf("could not set regexes: %v", err)
		}
		var names []string
		for _, job := range neededPresubmits(tc.possible, tc.current, tc.refs, sets.String{}) {
			names = append(names, job.Name)
		}
		expectEqual(t, tc.name, names, tc.required)
	}
}
