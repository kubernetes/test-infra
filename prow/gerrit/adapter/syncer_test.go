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

// Package adapter implements a controller that interacts with gerrit instances
package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"k8s.io/test-infra/prow/gerrit/client"
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

	dir, err := ioutil.TempDir("", "fake-gerrit-value")
	if err != nil {
		t.Fatalf("Could not create temp file: %v", err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "value.txt")
	var noCreds string
	ctx := context.Background()
	open, err := io.NewOpener(ctx, noCreds, noCreds)
	if err != nil {
		t.Fatalf("Failed to create opener: %v", err)
	}

	st := syncTime{
		path:   path,
		opener: open,
		ctx:    ctx,
	}
	testProjectsFlag := client.ProjectsFlag{"foo": []string{"bar"}}
	now := time.Now()
	if err := st.init(testProjectsFlag); err != nil {
		t.Fatalf("Failed init: %v", err)
	}
	cur := st.Current()["foo"]["bar"]
	if now.After(cur) {
		t.Fatalf("%v should be >= time before init was called: %v", cur, now)
	}

	earlier := now.Add(-time.Hour)
	later := now.Add(time.Hour)

	if err := st.Update(client.LastSyncState{"foo": {"bar": earlier}}); err != nil {
		t.Fatalf("Failed update: %v", err)
	}
	if actual := st.Current()["foo"]["bar"]; !actual.Equal(cur) {
		t.Errorf("Update(%v) should not have reduced value from %v, got %v", earlier, cur, actual)
	}

	if err := st.Update(client.LastSyncState{"foo": {"bar": later}}); err != nil {
		t.Fatalf("Failed update: %v", err)
	}
	if actual := st.Current()["foo"]["bar"]; !actual.After(cur) {
		t.Errorf("Update(%v) did not move current value to after %v, got %v", later, cur, actual)
	}

	expected := later
	st = syncTime{
		path:   path,
		opener: open,
		ctx:    ctx,
	}
	if err := st.init(testProjectsFlag); err != nil {
		t.Fatalf("Failed init: %v", err)
	}
	if actual := st.Current()["foo"]["bar"]; !actual.Equal(expected) {
		t.Errorf("init() failed to reload %v, got %v", expected, actual)
	}
	// Make sure update can work
	if err := st.update(client.ProjectsFlag{"foo-updated": []string{"bar-updated"}}); err != nil {
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

	st = syncTime{
		path:   path,
		opener: fakeOpener{}, // return storage.ErrObjectNotExist on open
		ctx:    ctx,
	}
	if err := st.init(testProjectsFlag); err != nil {
		t.Fatalf("Failed init: %v", err)
	}
	if actual := st.Current()["foo"]["bar"]; now.After(actual) || actual.After(later) {
		t.Fatalf("should initialize to start %v <= actual <= later %v, but got %v", now, later, actual)
	}
}

func TestNewProjectAddition(t *testing.T) {
	dir, err := ioutil.TempDir("", "fake-gerrit-value")
	if err != nil {
		t.Fatalf("Could not create temp file: %v", err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "value.txt")

	testTime := time.Now().Add(-time.Minute)
	testStVal := client.LastSyncState{"foo": {"bar": testTime}}
	testStValBytes, _ := json.Marshal(testStVal)
	_ = ioutil.WriteFile(path, testStValBytes, os.ModePerm)

	var noCreds string
	ctx := context.Background()
	open, err := io.NewOpener(ctx, noCreds, noCreds)
	if err != nil {
		t.Fatalf("Failed to create opener: %v", err)
	}
	testProjectsFlag := client.ProjectsFlag{"foo": []string{"bar"}, "qwe": []string{"qux"}}

	st := syncTime{
		path:   path,
		opener: open,
		ctx:    ctx,
	}

	if err := st.init(testProjectsFlag); err != nil {
		t.Fatalf("Failed init: %v", err)
	}
	if _, ok := st.val["qwe"]; !ok {
		t.Error("expected tracker to initialize a new entry for qwe, but did not")
	}
	if _, ok := st.val["qwe"]["qux"]; !ok {
		t.Error("expected tracker to initialize a new entry for qwe/qux, but did not")
	}
}
