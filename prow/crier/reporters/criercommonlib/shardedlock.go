/*
Copyright 2018 The Kubernetes Authors.

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

// Package criercommonlib contains shared lib used by reporters
package criercommonlib

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"
)

// SimplePull contains info for identifying a shard
type SimplePull struct {
	org, repo string
	number    int
}

// NewSimplePull creates SimplePull
func NewSimplePull(org, repo string, number int) *SimplePull {
	return &SimplePull{org: org, repo: repo, number: number}
}

// ShardedLock contains sharding information based on PRs
type ShardedLock struct {
	mapLock *semaphore.Weighted
	locks   map[SimplePull]*semaphore.Weighted
}

// NewShardedLock creates ShardedLock
func NewShardedLock(mapLock *semaphore.Weighted, locks map[SimplePull]*semaphore.Weighted) *ShardedLock {
	return &ShardedLock{mapLock: mapLock, locks: locks}
}

// GetLock aquires the lock for a PR
func (s *ShardedLock) GetLock(ctx context.Context, key SimplePull) (*semaphore.Weighted, error) {
	if err := s.mapLock.Acquire(ctx, 1); err != nil {
		return nil, err
	}
	defer s.mapLock.Release(1)
	if _, exists := s.locks[key]; !exists {
		s.locks[key] = semaphore.NewWeighted(1)
	}
	return s.locks[key], nil
}

// Cleanup deletes all locks by acquiring first
// the mapLock and then each individual lock before
// deleting it. The individual lock must be acquired
// because otherwise it may be held, we delete it from
// the map, it gets recreated and acquired and two
// routines report in parallel for the same job.
// Note that while this function is running, no new
// presubmit reporting can happen, as we hold the mapLock.
func (s *ShardedLock) Cleanup() {
	ctx := context.Background()
	s.mapLock.Acquire(ctx, 1)
	defer s.mapLock.Release(1)

	for key, lock := range s.locks {
		lock.Acquire(ctx, 1)
		delete(s.locks, key)
		lock.Release(1)
	}
}

// RunCleanup asynchronously runs the cleanup once per hour.
func (s *ShardedLock) RunCleanup() {
	go func() {
		for range time.Tick(time.Hour) {
			logrus.Debug("Starting to clean up presubmit locks")
			startTime := time.Now()
			s.Cleanup()
			logrus.WithField("duration", time.Since(startTime).String()).Debug("Finished cleaning up presubmit locks")
		}
	}()
}
