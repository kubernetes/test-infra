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
	"io/ioutil"
	"net/http"
	"strconv"
	"sync"

	"k8s.io/test-infra/mungegithub/features"
	"k8s.io/test-infra/mungegithub/github"

	"github.com/NYTimes/gziphandler"
	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
	"github.com/spf13/cobra"
)

var _ Munger = &BulkLGTM{}

func init() {
	RegisterMungerOrDie(&BulkLGTM{})
}

// BulkLGTM knows how to aggregate a large number of small PRs into a single page for
// easy bulk review.
type BulkLGTM struct {
	config          *github.Config
	lock            sync.Mutex
	currentPRList   map[int]*github.MungeObject
	maxDiff         int
	maxCommits      int
	maxChangedFiles int
	githubUser      string
}

// Munge implements the Munger interface
func (b *BulkLGTM) Munge(obj *github.MungeObject) {
	pr, isPr := obj.GetPR()
	if !isPr {
		return
	}
	glog.V(4).Infof("Found a PR: %#v", *pr)
	if !*pr.Mergeable {
		glog.V(4).Infof("PR is not mergeable, skipping")
		return
	}
	if *pr.Commits > b.maxCommits {
		glog.V(4).Infof("PR has too many commits %d vs %d, skipping", *pr.Commits, b.maxCommits)
		return
	}
	if *pr.ChangedFiles > b.maxChangedFiles {
		glog.V(4).Infof("PR has too many changed files %d vs %d, skipping", *pr.ChangedFiles, b.maxChangedFiles)
		return
	}
	if *pr.Additions+*pr.Deletions > b.maxDiff {
		glog.V(4).Infof("PR has too many diffs %d vs %d, skipping", *pr.Additions+*pr.Deletions, b.maxDiff)
		return
	}
	if obj.HasLabel(lgtmLabel) {
		return
	}
	if !obj.HasLabel("cncf-cla: yes") {
		return
	}
	glog.V(4).Infof("Adding a PR: %d", *pr.Number)
	b.lock.Lock()
	defer b.lock.Unlock()
	if b.currentPRList == nil {
		b.currentPRList = map[int]*github.MungeObject{}
	}
	b.currentPRList[*pr.Number] = obj
}

// AddFlags implements the Munger interface
func (b *BulkLGTM) AddFlags(cmd *cobra.Command, config *github.Config) {
	cmd.Flags().IntVar(&b.maxDiff, "bulk-lgtm-max-diff", 10, "The maximum number of differences (additions + deletions) for PRs to include in the bulk LGTM list")
	cmd.Flags().IntVar(&b.maxChangedFiles, "bulk-lgtm-changed-files", 1, "The maximum number of changed files for PRs to include in the bulk LGTM list")
	cmd.Flags().IntVar(&b.maxCommits, "bulk-lgtm-max-commits", 1, "The maximum number of commits for PRs to include in the bulk LGTM list")
	cmd.Flags().StringVar(&b.githubUser, "bulk-lgtm-github-user", "", "Username on github to use for bulk-lgtm")
}

// Name implements the Munger interface
func (b *BulkLGTM) Name() string {
	return "bulk-lgtm"
}

// RequiredFeatures implements the Munger interface
func (b *BulkLGTM) RequiredFeatures() []string {
	return nil
}

// Initialize implements the Munger interface
func (b *BulkLGTM) Initialize(config *github.Config, features *features.Features) error {
	b.config = config

	if len(config.Address) > 0 {
		http.HandleFunc("/bulkprs/prs", b.ServePRs)
		http.HandleFunc("/bulkprs/prdiff", b.ServePRDiff)
		http.HandleFunc("/bulkprs/lgtm", b.ServeLGTM)
		if len(config.WWWRoot) > 0 {
			http.Handle("/", gziphandler.GzipHandler(http.FileServer(http.Dir(config.WWWRoot))))
		}

		go http.ListenAndServe(config.Address, nil)
	}

	return nil
}

// EachLoop implements the Munger interface
func (b *BulkLGTM) EachLoop() error {
	return nil
}

// FindPR finds a PR in the list given its number
func (b *BulkLGTM) FindPR(number int) *github.MungeObject {
	b.lock.Lock()
	defer b.lock.Unlock()
	return b.currentPRList[number]
}

// ServeLGTM serves the LGTM page over HTTP
func (b *BulkLGTM) ServeLGTM(res http.ResponseWriter, req *http.Request) {
	prNumber, err := strconv.Atoi(req.URL.Query().Get("number"))
	if err != nil {
		res.Header().Set("Content-type", "text/plain")
		res.WriteHeader(http.StatusInternalServerError)
		res.Write([]byte(err.Error()))
		return
	}
	obj := b.FindPR(prNumber)
	if obj == nil {
		res.Header().Set("Content-type", "text/plain")
		res.WriteHeader(http.StatusNotFound)
		return
	}
	if err := obj.AddAssignee(b.githubUser); err != nil {
		res.Header().Set("Content-type", "text/plain")
		res.WriteHeader(http.StatusInternalServerError)
		res.Write([]byte(err.Error()))
		return
	}
	if err := obj.WriteComment("/lgtm\n\nLGTM from the bulk LGTM tool"); err != nil {
		res.Header().Set("Content-type", "text/plain")
		res.WriteHeader(http.StatusInternalServerError)
		res.Write([]byte(err.Error()))
		return
	}
	res.Header().Set("Content-type", "text/plain")
	res.WriteHeader(http.StatusOK)
}

// ServePRDiff serves the difs in the PR over HTTP
func (b *BulkLGTM) ServePRDiff(res http.ResponseWriter, req *http.Request) {
	prNumber, err := strconv.Atoi(req.URL.Query().Get("number"))
	if err != nil {
		res.Header().Set("Content-type", "text/plain")
		res.WriteHeader(http.StatusInternalServerError)
		return
	}
	obj := b.FindPR(prNumber)
	if obj == nil {
		res.Header().Set("Content-type", "text/plain")
		res.WriteHeader(http.StatusNotFound)
		return
	}
	pr, _ := obj.GetPR()
	resp, err := http.Get(*pr.DiffURL)
	if err != nil {
		res.Header().Set("Content-type", "text/plain")
		res.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		res.Header().Set("Content-type", "text/plain")
		res.WriteHeader(http.StatusInternalServerError)
		return
	}
	res.Header().Set("Content-Type", "text/plain")
	res.WriteHeader(http.StatusOK)
	res.Write(data)
}

// ServePRs serves the current PR list over HTTP
func (b *BulkLGTM) ServePRs(res http.ResponseWriter, req *http.Request) {
	b.lock.Lock()
	defer b.lock.Unlock()
	var data []byte
	var err error
	if b.currentPRList == nil {
		data = []byte("[]")
	}
	arr := make([]*githubapi.PullRequest, len(b.currentPRList))
	for ix := range b.currentPRList {
		arr[ix], _ = b.currentPRList[ix].GetPR()
	}
	data, err = json.Marshal(arr)
	if err != nil {
		res.Header().Set("Content-type", "text/plain")
		res.WriteHeader(http.StatusInternalServerError)
		return
	}

	res.WriteHeader(http.StatusOK)
	res.Write(data)
}
