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

package mungers

import (
	"sync"

	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"
	"k8s.io/kubernetes/pkg/util/sets"

	"github.com/spf13/cobra"
)

type issueIndexKey string

// IssueCacher keeps track of issues that track flaky tests, so we can find them.
type IssueCacher struct {
	// TODO: this can easily be extended to keep multiple issues indexes if needed.
	labelFilter sets.String

	lock       sync.RWMutex
	index      map[issueIndexKey]int
	prevIndex  map[issueIndexKey]int
	loopStarts int
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
	p.index = map[issueIndexKey]int{}
	p.prevIndex = map[issueIndexKey]int{}
	return nil
}

// EachLoop is called at the start of every munge loop
func (p *IssueCacher) EachLoop() error {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.prevIndex = p.index
	p.index = map[issueIndexKey]int{}
	p.loopStarts++
	return nil
}

// AddFlags will add any request flags to the cobra `cmd`
func (p *IssueCacher) AddFlags(cmd *cobra.Command, config *github.Config) {}

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

	p.lock.Lock()
	defer p.lock.Unlock()
	p.index[key] = *obj.Issue.Number
}

func (p *IssueCacher) keyFromIssue(obj *github.MungeObject) (issueIndexKey, bool) {
	if obj.Issue == nil || obj.Issue.Title == nil {
		return "", false
	}
	// Currently, just use the issue title directly.
	return issueIndexKey(*obj.Issue.Title), true
}

// IssueForKey returns the issue matching the given key
func (p *IssueCacher) IssueForKey(key string) (int, bool) {
	// TODO: do I need to compress the key? Is there a length limit on the title?
	p.lock.RLock()
	defer p.lock.RUnlock()
	if n, ok := p.index[issueIndexKey(key)]; ok {
		return n, true
	}
	if n, ok := p.prevIndex[issueIndexKey(key)]; ok {
		return n, true
	}
	// TODO: attempt fuzzy matching?
	return 0, false
}

// Created adds this entry to the cache, in case you try to access it again
// before another complete pass is made.
func (p *IssueCacher) Created(key string, number int) {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.index[issueIndexKey(key)] = number
}

// Synced returns true if we've made at least one complete pass through all
// issues.
func (p *IssueCacher) Synced() bool {
	p.lock.RLock()
	defer p.lock.RUnlock()
	return p.loopStarts > 1
}
