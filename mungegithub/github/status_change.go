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

package github

import (
	"sync"

	"k8s.io/kubernetes/pkg/util/sets"
)

// StatusChange keeps track of issue/commit for status changes
type StatusChange struct {
	heads        map[int]string      // Pull-Request ID -> head-sha
	pullRequests map[string]sets.Int // head-sha -> Pull-Request IDs
	changed      sets.String         // SHA of commits whose status changed
	mutex        sync.Mutex
}

// NewStatusChange creates a new status change tracker
func NewStatusChange() *StatusChange {
	return &StatusChange{
		heads:        map[int]string{},
		pullRequests: map[string]sets.Int{},
		changed:      sets.NewString(),
	}
}

// UpdatePullRequestHead updates the head commit for a pull-request
func (s *StatusChange) UpdatePullRequestHead(pullRequestID int, newHead string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if oldHead, has := s.heads[pullRequestID]; has {
		delete(s.pullRequests, oldHead)
	}
	s.heads[pullRequestID] = newHead
	if _, has := s.pullRequests[newHead]; !has {
		s.pullRequests[newHead] = sets.NewInt()
	}
	s.pullRequests[newHead].Insert(pullRequestID)
}

// CommitStatusChanged must be called when the status for this commit has changed
func (s *StatusChange) CommitStatusChanged(commit string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.changed.Insert(commit)
}

// PopChangedPullRequests returns the list of issues changed since last call
func (s *StatusChange) PopChangedPullRequests() []int {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	changedPullRequests := sets.NewInt()
	for _, commit := range s.changed.List() {
		if pullRequests, has := s.pullRequests[commit]; has {
			changedPullRequests = changedPullRequests.Union(pullRequests)
		}
	}
	s.changed = sets.NewString()

	return changedPullRequests.List()
}
