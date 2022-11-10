/*
Copyright 2021 The Kubernetes Authors.

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

// Package client implements client that interacts with gerrit instances
package client

import (
	"context"
	"encoding/json"
	"fmt"
	stdio "io"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/io"
)

// opener has methods to read and write paths
type opener interface {
	Reader(ctx context.Context, path string) (io.ReadCloser, error)
	Writer(ctx context.Context, path string, opts ...io.WriterOptions) (io.WriteCloser, error)
}

type SyncTime struct {
	val    LastSyncState
	lock   sync.RWMutex
	path   string
	opener opener
	ctx    context.Context
}

func NewSyncTime(path string, opener opener, ctx context.Context) *SyncTime {
	return &SyncTime{
		path:   path,
		opener: opener,
		ctx:    ctx,
	}
}

func (st *SyncTime) Init(hostProjects map[string]map[string]*config.GerritQueryFilter) error {
	st.lock.RLock()
	zero := st.val == nil
	st.lock.RUnlock()
	if !zero {
		return nil
	}
	return st.update(hostProjects)
}

func (st *SyncTime) update(hostProjects map[string]map[string]*config.GerritQueryFilter) error {
	timeNow := time.Now()
	st.lock.Lock()
	defer st.lock.Unlock()
	state, err := st.currentState()
	if err != nil {
		return err
	}
	if state != nil {
		// Initialize new hosts, projects
		for host, projects := range hostProjects {
			if _, ok := state[host]; !ok {
				state[host] = map[string]time.Time{}
			}
			for project := range projects {
				if _, ok := state[host][project]; !ok {
					state[host][project] = timeNow
				}
			}
		}
		st.val = state
		logrus.WithField("lastSync", st.val).Infoln("Initialized successfully from lastSyncFallback.")
	} else {
		targetState := LastSyncState{}
		for host, projects := range hostProjects {
			targetState[host] = map[string]time.Time{}
			for project := range projects {
				targetState[host][project] = timeNow
			}
		}
		st.val = targetState
	}
	return nil
}

func (st *SyncTime) currentState() (LastSyncState, error) {
	r, err := st.opener.Reader(st.ctx, st.path)
	if io.IsNotExist(err) {
		logrus.Warnf("lastSyncFallback not found at %q", st.path)
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer io.LogClose(r)
	buf, err := stdio.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	var state LastSyncState
	if err := json.Unmarshal(buf, &state); err != nil {
		// Don't error on unmarshall error, let it default
		logrus.WithField("lastSync", st.val).Warnln("Failed to unmarshal lastSyncFallback, resetting all last update times to current.")
		return nil, nil
	}
	return state, nil
}

func (st *SyncTime) Current() LastSyncState {
	st.lock.RLock()
	defer st.lock.RUnlock()
	return st.val
}

func (st *SyncTime) Update(newState LastSyncState) error {
	st.lock.Lock()
	defer st.lock.Unlock()

	targetState := st.val.DeepCopy()

	var changed bool
	for host, newLastSyncs := range newState {
		if _, ok := targetState[host]; !ok {
			targetState[host] = map[string]time.Time{}
		}
		for project, newLastSync := range newLastSyncs {
			currentLastSync, ok := targetState[host][project]
			if !ok || currentLastSync.Before(newLastSync) {
				targetState[host][project] = newLastSync
				changed = true
			}
		}
	}

	if !changed {
		return nil
	}

	w, err := st.opener.Writer(st.ctx, st.path)
	if err != nil {
		return fmt.Errorf("open for write %q: %w", st.path, err)
	}
	stateBytes, err := json.Marshal(targetState)
	if err != nil {
		return fmt.Errorf("marshall state: %w", err)
	}
	if _, err := fmt.Fprint(w, string(stateBytes)); err != nil {
		io.LogClose(w)
		return fmt.Errorf("write %q: %w", st.path, err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close %q: %w", st.path, err)
	}
	st.val = targetState
	return nil
}
