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

package mungers

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"k8s.io/test-infra/mungegithub/mungeopts"
	"k8s.io/test-infra/mungegithub/options"

	githubapi "github.com/google/go-github/github"
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

func TestGetSuccessfulBatchJobs(t *testing.T) {
	body := `[
	{"type":"batch", "repo":"a", "refs":"1", "state":"success", "context":"$"},
	{"type":"batch", "repo":"a", "refs":"2", "state":"success", "context":"!"},

	{"type":"pr", "repo":"a", "refs":"1", "state":"success", "context":"$"},
	{"type":"batch", "repo":"b", "refs":"1", "state":"success", "context":"$"},
	{"type":"batch", "repo":"a", "refs":"1", "state":"fail", "context":"$"}
	]`
	serv := httptest.NewServer(stringHandler(body))
	defer serv.Close()
	jobs, err := getJobs(serv.URL)
	if err != nil {
		t.Fatal(err)
	}
	jobs = jobs.repo("a").batch().successful()
	expectEqual(t, "batchJobs", jobs, prowJobs{
		{Type: "batch", Repo: "a", State: "success", Refs: "1", Context: "$"},
		{Type: "batch", Repo: "a", State: "success", Refs: "2", Context: "!"},
	})
}

func TestBatchRefToBatch(t *testing.T) {
	_, strconvErr := strconv.ParseInt("a", 10, 32)
	for _, test := range []struct {
		ref      string
		expected Batch
		err      error
	}{
		{"m:a", Batch{"m", "a", nil}, nil},
		{"m:a,1:b", Batch{"m", "a", []batchPull{{1, "b"}}}, nil},
		{"m:a,1:b,2:c", Batch{"m", "a", []batchPull{{1, "b"}, {2, "c"}}}, nil},
		{"asdf", Batch{}, errors.New("bad batchref: asdf")},
		{"m:a,a:3", Batch{}, fmt.Errorf("bad batchref: m:a,a:3 (%v)", strconvErr)},
	} {
		batch, err := batchRefToBatch(test.ref)
		expectEqual(t, "error", err, test.err)
		expectEqual(t, "batch", batch, test.expected)
		if err == nil {
			expectEqual(t, "batch.String()", batch.String(), test.ref)
		}
	}
}

func TestGetCompletedBatches(t *testing.T) {
	mungeopts.RequiredContexts.Retest = []string{"rt"}
	mungeopts.RequiredContexts.Merge = []string{"st"}
	sq := SubmitQueue{opts: options.New()}
	for _, test := range []struct {
		jobs    prowJobs
		batches []Batch
	}{
		{prowJobs{}, []Batch{}},
		{prowJobs{{Refs: "m:a", Context: "rt"}}, []Batch{}},
		{prowJobs{{Refs: "m:a", Context: "st"}}, []Batch{}},
		{prowJobs{{Refs: "m:a", Context: "rt"}, {Refs: "m:a", Context: "st"}}, []Batch{{"m", "a", nil}}},
	} {
		expectEqual(t, "getCompletedBatches", sq.getCompleteBatches(test.jobs), test.batches)
	}
}

func TestBatchMatchesCommits(t *testing.T) {
	makeCommits := func(spec []string) []*githubapi.RepositoryCommit {
		out := []*githubapi.RepositoryCommit{}
		for _, s := range spec {
			i := strings.Index(s, " ")
			refs := s[:i]
			msg := s[i+1:]
			split := strings.Split(refs, ":")
			commit := githubapi.RepositoryCommit{
				SHA:    &split[0],
				Commit: &githubapi.Commit{Message: &msg},
			}
			for _, parent := range strings.Split(split[1], ",") {
				p := string(parent) // thanks, Go!
				commit.Parents = append(commit.Parents, githubapi.Commit{SHA: &p})
			}
			out = append(out, &commit)
		}
		return out
	}

	for _, test := range []struct {
		pulls    []batchPull
		commits  []string
		expected int
		err      string
	}{
		// no commits
		{nil, []string{}, 0, "no commits"},
		// base matches
		{nil, []string{"a:0 blah"}, 0, ""},
		// base doesn't match
		{nil, []string{"b:0 blaga"}, 0, "Unknown non-merge commit b"},
		// PR could apply
		{[]batchPull{{1, "c"}}, []string{"a:0 blah"}, 0, ""},
		// PR already applied
		{[]batchPull{{1, "c"}}, []string{"d:a,c Merge #1", "c:a fix stuff", "a:0 blah"}, 1, ""},
		// unknown merge
		{[]batchPull{{2, "d"}}, []string{"d:a,c Merge #1", "c:a fix stuff", "a:0 blah"}, 0, "Merge of something not in batch"},
		// unknown commit
		{[]batchPull{{2, "d"}}, []string{"c:a fix stuff", "a:0 blah"}, 0, "Unknown non-merge commit c"},
		// PRs could apply
		{[]batchPull{{1, "c"}, {2, "e"}}, []string{"a:0 blah"}, 0, ""},
		// 1 PR applied
		{[]batchPull{{1, "c"}, {2, "e"}}, []string{"d:a,c Merge #1", "c:a fix stuff", "a:0 blah"}, 1, ""},
		// other PR merged
		{[]batchPull{{1, "c"}, {2, "e"}}, []string{"d:a,g Merge #3", "g:a add feature", "a:0 blah"}, 0, "Merge of something not in batch"},
		// both PRs already merged
		{[]batchPull{{1, "c"}, {2, "e"}},
			[]string{"f:d,e Merge #2", "e:a fix bug", "d:a,c Merge #1", "c:a fix stuff", "a:0 blah"}, 2, ""},
		// PRs merged in wrong order
		{[]batchPull{{1, "c"}, {2, "e"}},
			[]string{"f:d,c Merge #1", "d:a,e Merge #2", "e:a fix bug", "c:a fix stuff", "a:0 blah"}, 0, "Batch PRs merged out-of-order"},
	} {
		batch := Batch{"m", "a", test.pulls}
		commits := makeCommits(test.commits)
		actual, err := batch.matchesCommits(commits)
		if err == nil {
			err = errors.New("")
		}
		expectEqual(t, "batch.matchesCommits", actual, test.expected)
		expectEqual(t, "batch.matchesCommits err", err.Error(), test.err)
	}
}
