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
	"sort"
	"sync"

	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"
	"k8s.io/kubernetes/pkg/util/sets"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
	"github.com/spf13/cobra"
)

type issueIndexKey string

type issueList []int

func (l *issueList) add(i int) {
	*l = append(*l, i)
	// Not efficient but we don't expect big lists.
	sort.Ints([]int(*l))
}
func (l *issueList) mostRecent() int {
	n := len(*l)
	return (*l)[n-1]
}

type keyToIssueList map[issueIndexKey]*issueList

// IssueCacher keeps track of issues that track flaky tests, so we can find them.
type IssueCacher struct {
	// TODO: this can easily be extended to keep multiple issues indexes if needed.
	labelFilter sets.String

	lock                                sync.RWMutex
	index                               keyToIssueList
	prevIndex                           keyToIssueList
	firstSyncStarted, firstSyncFinished bool

	config *github.Config
}

func init() {
	RegisterMungerOrDie(&IssueCacher{})
}

// Name is the name usable in --pr-mungers
func (p *IssueCacher) Name() string { return "issue-cacher" }

// RequiredFeatures is a slice of 'features' that must be provided
func (p *IssueCacher) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (p *IssueCacher) Initialize(config *github.Config, features *features.Features) error {
	p.labelFilter = sets.NewString("kind/flake")
	p.index = keyToIssueList{}
	p.prevIndex = keyToIssueList{}
	p.config = config
	return nil
}

// EachLoop is called at the start of every munge loop
func (p *IssueCacher) EachLoop() error {
	func() {
		p.lock.Lock()
		defer p.lock.Unlock()
		p.prevIndex = p.index
		p.index = keyToIssueList{}
		if !p.firstSyncStarted {
			p.firstSyncStarted = true
		} else if !p.firstSyncFinished {
			p.firstSyncFinished = true
		}
	}()
	p.findClosedIssues()
	return nil
}

// AddFlags will add any request flags to the cobra `cmd`
func (p *IssueCacher) AddFlags(cmd *cobra.Command, config *github.Config) {}

func (p *IssueCacher) findClosedIssues() {
	issues, err := p.config.ListAllIssues(&githubapi.IssueListByRepoOptions{
		State:  "closed",
		Labels: p.labelFilter.List(),
	})
	if err != nil {
		glog.Errorf("Error getting closed issues labeled %v: %v", p.labelFilter, err)
		return
	}
	for _, issue := range issues {
		p.Munge(&github.MungeObject{Issue: issue})
	}
}

// Munge is the workhorse the will actually make updates to the PR
func (p *IssueCacher) Munge(obj *github.MungeObject) {
	if obj.IsPR() {
		return
	}
	if !obj.HasLabels(p.labelFilter.List()) {
		return
	}

	key, ok := p.keyFromIssue(obj)
	if !ok {
		return
	}

	p.addNumberToKey(key, *obj.Issue.Number)
}

func (p *IssueCacher) addNumberToKey(key issueIndexKey, issueNumber int) {
	p.lock.Lock()
	defer p.lock.Unlock()
	if l, ok := p.index[key]; ok {
		l.add(issueNumber)
	} else {
		p.index[key] = &issueList{issueNumber}
	}
}

func (p *IssueCacher) keyFromIssue(obj *github.MungeObject) (issueIndexKey, bool) {
	if obj.Issue == nil || obj.Issue.Title == nil {
		return "", false
	}
	// Currently, just use the issue title directly.
	return issueIndexKey(*obj.Issue.Title), true
}

// AllIssuesForKey returns all known issues matching the key, oldest first.
func (p *IssueCacher) AllIssuesForKey(key string) []int {
	p.lock.RLock()
	defer p.lock.RUnlock()
	got := sets.NewInt()
	if n, ok := p.index[issueIndexKey(key)]; ok {
		got.Insert([]int(*n)...)
	}
	if n, ok := p.prevIndex[issueIndexKey(key)]; ok {
		got.Insert([]int(*n)...)
	}
	out := got.List()
	sort.Ints(out)
	return out
}

// Created adds this entry to the cache, in case you try to access it again
// before another complete pass is made.
func (p *IssueCacher) Created(key string, number int) {
	p.addNumberToKey(issueIndexKey(key), number)
}

// Synced returns true if we've made at least one complete pass through all
// issues.
func (p *IssueCacher) Synced() bool {
	p.lock.RLock()
	defer p.lock.RUnlock()
	return p.firstSyncFinished
}
