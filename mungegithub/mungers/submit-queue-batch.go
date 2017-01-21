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
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
	"k8s.io/contrib/mungegithub/github"
)

// xref k8s.io/test-infra/prow/cmd/deck/jobs.go
type prowJob struct {
	Type    string `json:"type"`
	Repo    string `json:"repo"`
	Refs    string `json:"refs"`
	State   string `json:"state"`
	Context string `json:"context"`
}

type prowJobs []prowJob

// getJobs reads job information as JSON from a given URL.
func getJobs(url string) (prowJobs, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)

	jobs := prowJobs{}
	err = json.Unmarshal(body, &jobs)
	if err != nil {
		return nil, err
	}
	return jobs, nil
}

func (j prowJobs) filter(pred func(prowJob) bool) prowJobs {
	out := prowJobs{}
	for _, job := range j {
		if pred(job) {
			out = append(out, job)
		}
	}
	return out
}

func (j prowJobs) repo(repo string) prowJobs {
	return j.filter(func(job prowJob) bool { return job.Repo == repo })
}

func (j prowJobs) batch() prowJobs {
	return j.filter(func(job prowJob) bool { return job.Type == "batch" })
}

func (j prowJobs) successful() prowJobs {
	return j.filter(func(job prowJob) bool { return job.State == "success" })
}

func (j prowJobs) firstUnfinished() *prowJob {
	for _, job := range j {
		if job.State == "triggered" || job.State == "pending" {
			return &job
		}
	}
	return nil
}

type batchPull struct {
	Number int
	Sha    string
}

// Batch represents a specific merge state:
// a base branch and SHA, and the SHAs of each PR merged into it.
type Batch struct {
	BaseName string
	BaseSha  string
	Pulls    []batchPull
}

func (b *Batch) String() string {
	out := b.BaseName + ":" + b.BaseSha
	for _, pull := range b.Pulls {
		out += "," + strconv.Itoa(pull.Number) + ":" + pull.Sha
	}
	return out
}

// batchRefToBatch parses a string into a Batch.
// The input is a comma-separated list of colon-separated ref/sha pairs,
// like "master:abcdef0,123:f00d,456:f00f".
func batchRefToBatch(batchRef string) (Batch, error) {
	batch := Batch{}
	for i, ref := range strings.Split(batchRef, ",") {
		parts := strings.Split(ref, ":")
		if len(parts) != 2 {
			return Batch{}, errors.New("bad batchref: " + batchRef)
		}
		if i == 0 {
			batch.BaseName = parts[0]
			batch.BaseSha = parts[1]
		} else {
			num, err := strconv.ParseInt(parts[0], 10, 32)
			if err != nil {
				return Batch{}, fmt.Errorf("bad batchref: %s (%v)", batchRef, err)
			}
			batch.Pulls = append(batch.Pulls, batchPull{int(num), parts[1]})
		}
	}
	return batch, nil
}

// getCompleteBatches returns a list of Batches that passed all
// required tests.
func (sq *SubmitQueue) getCompleteBatches(jobs prowJobs) []Batch {
	// for each batch specifier, a set of successful contexts
	batchContexts := make(map[string]map[string]interface{})
	for _, job := range jobs {
		if batchContexts[job.Refs] == nil {
			batchContexts[job.Refs] = make(map[string]interface{})
		}
		batchContexts[job.Refs][job.Context] = nil
	}
	batches := []Batch{}
	for batchRef, contexts := range batchContexts {
		match := true
		// Did this succeed in all the contexts we want?
		for _, ctx := range sq.RequiredStatusContexts {
			if _, ok := contexts[ctx]; !ok {
				match = false
			}
		}
		for _, ctx := range sq.RequiredRetestContexts {
			if _, ok := contexts[ctx]; !ok {
				match = false
			}
		}
		if match {
			batch, err := batchRefToBatch(batchRef)
			if err != nil {
				continue
			}
			batches = append(batches, batch)
		}
	}
	return batches
}

// batchIntersectsQueue returns whether at least one PR in the batch is queued.
func (sq *SubmitQueue) batchIntersectsQueue(batch Batch) bool {
	sq.Lock()
	defer sq.Unlock()
	for _, pull := range batch.Pulls {
		if _, ok := sq.githubE2EQueue[pull.Number]; ok {
			return true
		}
	}
	return false
}

// matchesCommit determines if the batch can be merged given some commits.
// That is, does it contain exactly:
// 1) the batch's BaseSha
// 2) (optional) merge commits for PRs in the batch
// 3) any merged PRs in the batch are sequential from the beginning
// The return value is the number of PRs already merged, and any errors.
func (b *Batch) matchesCommits(commits []*githubapi.RepositoryCommit) (int, error) {
	if len(commits) == 0 {
		return 0, errors.New("no commits")
	}

	shaToPR := make(map[string]int)

	for _, pull := range b.Pulls {
		shaToPR[pull.Sha] = pull.Number
	}

	matchedPRs := []int{}

	// convert the list of commits into a DAG for easy following
	dag := make(map[string]*githubapi.RepositoryCommit)
	for _, commit := range commits {
		dag[*commit.SHA] = commit
	}

	ref := *commits[0].SHA
	for {
		if ref == b.BaseSha {
			break // found the base ref (condition #1)
		}
		commit, ok := dag[ref]
		if !ok {
			return 0, errors.New("ran out of commits (missing ref " + ref + ")")
		}
		message := ""
		if commit.Commit != nil && commit.Commit.Message != nil {
			// The actual commit message is buried a little oddly.
			message = *commit.Commit.Message
		}
		if len(commit.Parents) == 2 && strings.HasPrefix(message, "Merge") {
			// looks like a merge commit!

			// first parent is the normal branch
			ref = *commit.Parents[0].SHA
			// second parent is the PR
			pr, ok := shaToPR[*commit.Parents[1].SHA]
			if !ok {
				return 0, errors.New("Merge of something not in batch")
			}
			matchedPRs = append(matchedPRs, pr)
		} else {
			return 0, errors.New("Unknown non-merge commit " + ref)
		}
	}

	// Now, ensure that the merged PRs are ordered correctly.
	for i, pr := range matchedPRs {
		if b.Pulls[len(matchedPRs)-1-i].Number != pr {
			return 0, errors.New("Batch PRs merged out-of-order")
		}
	}
	return len(matchedPRs), nil
}

// batchIsApplicable returns whether a successful batch result can be used--
// 1) some of the batch is still unmerged and in the queue.
// 2) the recent commits are the batch head ref or merges of batch PRs.
// 3) all unmerged PRs in the batch are still in the queue.
// The return value is the number of PRs already merged, and any errors.
func (sq *SubmitQueue) batchIsApplicable(batch Batch) (int, error) {
	// batch must intersect the queue
	if !sq.batchIntersectsQueue(batch) {
		return 0, errors.New("batch has no PRs in Queue")
	}
	commits, err := sq.githubConfig.GetBranchCommits(batch.BaseName, 100)
	if err != nil {
		glog.Errorf("Error getting commits for batchIsApplicable: %v", err)
		return 0, errors.New("failed to get branch commits: " + err.Error())
	}
	return batch.matchesCommits(commits)
}

func (sq *SubmitQueue) handleGithubE2EBatchMerge() {
	repo := sq.githubConfig.Org + "/" + sq.githubConfig.Project
	for range time.Tick(1 * time.Minute) {
		allJobs, err := getJobs(sq.BatchURL)
		if err != nil {
			glog.Errorf("Error reading batch jobs from Prow URL %v: %v", sq.BatchURL, err)
			continue
		}
		batchJobs := allJobs.batch().repo(repo)
		jobs := batchJobs.successful()
		batches := sq.getCompleteBatches(jobs)
		batchErrors := make(map[string]string)
		for _, batch := range batches {
			_, err := sq.batchIsApplicable(batch)
			if err != nil {
				batchErrors[batch.String()] = err.Error()
				continue
			}
			sq.doBatchMerge(batch)
		}
		sq.batchStatus.Error = batchErrors
		sq.batchStatus.Running = batchJobs.firstUnfinished()
	}
}

// doBatchMerge iteratively merges PRs in the batch if possible.
// If you modify this, consider modifying doGithubE2EAndMerge too.
func (sq *SubmitQueue) doBatchMerge(batch Batch) {
	sq.mergeLock.Lock()
	defer sq.mergeLock.Unlock()

	// Test again inside the merge lock, in case some other merge snuck in.
	match, err := sq.batchIsApplicable(batch)
	if err != nil {
		glog.Errorf("unexpected! batchIsApplicable failed after success %v", err)
		return
	}
	if !sq.e2eStable(true) {
		return
	}

	glog.Infof("merging batch: %s", batch)
	prs := []*github.MungeObject{}
	// Check entire batch's preconditions first.
	for _, pull := range batch.Pulls[match:] {
		obj, err := sq.githubConfig.GetObject(pull.Number)
		if err != nil {
			glog.Errorf("error getting object for pr #%d: %v", pull.Number, err)
			return
		}
		if sha, _, ok := obj.GetHeadAndBase(); !ok {
			glog.Errorf("error getting pr #%d sha", pull.Number, err)
			return
		} else if sha != pull.Sha {
			glog.Errorf("error: batch PR #%d HEAD changed: %s instead of %s",
				sha, pull.Sha)
			return
		}
		if !sq.validForMergeExt(obj, false) {
			return
		}
		prs = append(prs, obj)
	}

	// Make the merge less confusing: describe the overall batch.
	prStrings := []string{}
	for _, pull := range batch.Pulls {
		prStrings = append(prStrings, strconv.Itoa(pull.Number))
	}
	extra := fmt.Sprintf(" (batch tested with PRs %s)", strings.Join(prStrings, ", "))

	// then merge each
	for _, pr := range prs {
		ok := sq.mergePullRequest(pr, mergedBatch, extra)
		if !ok {
			return
		}
		atomic.AddInt32(&sq.batchMerges, 1)
	}
}
