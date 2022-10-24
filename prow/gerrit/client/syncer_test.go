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

package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/google/go-cmp/cmp"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/io"
)

type fakeOpener struct{}

func (o fakeOpener) Reader(ctx context.Context, path string) (io.ReadCloser, error) {
	return nil, storage.ErrObjectNotExist
}

func (o fakeOpener) Writer(ctx context.Context, path string, _ ...io.WriterOptions) (io.WriteCloser, error) {
	return nil, errors.New("do not call Writer")
}

func TestSyncTime(t *testing.T) {

	dir := t.TempDir()
	path := filepath.Join(dir, "value.txt")
	var noCreds string
	ctx := context.Background()
	open, err := io.NewOpener(ctx, noCreds, noCreds)
	if err != nil {
		t.Fatalf("Failed to create opener: %v", err)
	}

	st := SyncTime{
		path:   path,
		opener: open,
		ctx:    ctx,
	}
	testProjects := map[string]map[string]*config.GerritQueryFilter{
		"foo": {
			"bar": {},
		},
	}
	now := time.Now()
	if err := st.Init(testProjects); err != nil {
		t.Fatalf("Failed init: %v", err)
	}
	cur := st.Current()["foo"]["bar"]
	if now.After(cur) {
		t.Fatalf("%v should be >= time before init was called: %v", cur, now)
	}

	earlier := now.Add(-time.Hour)
	later := now.Add(time.Hour)

	if err := st.Update(LastSyncState{"foo": {"bar": earlier}}); err != nil {
		t.Fatalf("Failed update: %v", err)
	}
	if actual := st.Current()["foo"]["bar"]; !actual.Equal(cur) {
		t.Errorf("Update(%v) should not have reduced value from %v, got %v", earlier, cur, actual)
	}

	if err := st.Update(LastSyncState{"foo": {"bar": later}}); err != nil {
		t.Fatalf("Failed update: %v", err)
	}
	if actual := st.Current()["foo"]["bar"]; !actual.After(cur) {
		t.Errorf("Update(%v) did not move current value to after %v, got %v", later, cur, actual)
	}

	expected := later
	st = SyncTime{
		path:   path,
		opener: open,
		ctx:    ctx,
	}
	if err := st.Init(testProjects); err != nil {
		t.Fatalf("Failed init: %v", err)
	}
	if actual := st.Current()["foo"]["bar"]; !actual.Equal(expected) {
		t.Errorf("init() failed to reload %v, got %v", expected, actual)
	}
	// Make sure update can work
	if err := st.update(map[string]map[string]*config.GerritQueryFilter{"foo-updated": {"bar-updated": nil}}); err != nil {
		t.Fatalf("Failed update: %v", err)
	}
	{
		gotState := st.Current()
		if gotRepos, ok := gotState["foo-updated"]; !ok {
			t.Fatal("Update() org failed.")
		} else if _, ok := gotRepos["bar-updated"]; !ok {
			t.Fatal("Update() repo failed.")
		}
	}

	st = SyncTime{
		path:   path,
		opener: fakeOpener{}, // return storage.ErrObjectNotExist on open
		ctx:    ctx,
	}
	if err := st.Init(testProjects); err != nil {
		t.Fatalf("Failed init: %v", err)
	}
	if actual := st.Current()["foo"]["bar"]; now.After(actual) || actual.After(later) {
		t.Fatalf("should initialize to start %v <= actual <= later %v, but got %v", now, later, actual)
	}
}

// TestSyncTimeThreadSafe ensures that the sync time can be updated threadsafe
// without lock.
func TestSyncTimeThreadSafe(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "value.txt")
	var noCreds string
	ctx := context.Background()
	open, err := io.NewOpener(ctx, noCreds, noCreds)
	if err != nil {
		t.Fatalf("Failed to create opener: %v", err)
	}

	st := SyncTime{
		path:   path,
		opener: open,
		ctx:    ctx,
	}
	testProjects := map[string]map[string]*config.GerritQueryFilter{
		"foo1": {
			"bar1": {},
		},
		"foo2": {
			"bar2": {},
		},
	}
	if err := st.Init(testProjects); err != nil {
		t.Fatalf("Failed init: %v", err)
	}

	// This is for detecting threading issue, running 100 times should be
	// sufficient for catching the issue.
	for i := 0; i < 100; i++ {
		// Two threads, one update foo1, the other update foo2
		var wg sync.WaitGroup
		wg.Add(2)
		later := time.Now().Add(time.Hour)
		var threadErr error
		go func() {
			defer wg.Done()
			syncTime := st.Current()
			latest := syncTime.DeepCopy()
			latest["foo1"]["bar1"] = later
			if err := st.Update(latest); err != nil {
				threadErr = fmt.Errorf("failed update: %v", err)
			}
		}()

		go func() {
			defer wg.Done()
			syncTime := st.Current()
			latest := syncTime.DeepCopy()
			latest["foo2"]["bar2"] = later
			if err := st.Update(latest); err != nil {
				threadErr = fmt.Errorf("failed update: %v", err)
			}
		}()

		wg.Wait()
		if threadErr != nil {
			t.Fatalf("Failed running goroutines: %v", err)
		}

		want := LastSyncState(map[string]map[string]time.Time{
			"foo1": {"bar1": later},
			"foo2": {"bar2": later},
		})

		if diff := cmp.Diff(st.Current(), want); diff != "" {
			t.Fatalf("Mismatch. Want(-), got(+):\n%s", diff)
		}
	}
}

func TestNewProjectAddition(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "value.txt")

	testTime := time.Now().Add(-time.Minute)
	testStVal := LastSyncState{"foo": {"bar": testTime}}
	testStValBytes, _ := json.Marshal(testStVal)
	_ = os.WriteFile(path, testStValBytes, os.ModePerm)

	var noCreds string
	ctx := context.Background()
	open, err := io.NewOpener(ctx, noCreds, noCreds)
	if err != nil {
		t.Fatalf("Failed to create opener: %v", err)
	}

	testProjects := map[string]map[string]*config.GerritQueryFilter{
		"foo": {
			"bar": {},
		},
		"qwe": {
			"qux": {},
		},
	}

	st := SyncTime{
		path:   path,
		opener: open,
		ctx:    ctx,
	}

	if err := st.Init(testProjects); err != nil {
		t.Fatalf("Failed init: %v", err)
	}
	if _, ok := st.val["qwe"]; !ok {
		t.Error("expected tracker to initialize a new entry for qwe, but did not")
	}
	if _, ok := st.val["qwe"]["qux"]; !ok {
		t.Error("expected tracker to initialize a new entry for qwe/qux, but did not")
	}
}
