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

package main

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/test-infra/pkg/io"
)

func TestSyncTime(t *testing.T) {

	dir, err := ioutil.TempDir("", "fake-gerrit-value")
	if err != nil {
		t.Fatalf("Could not create temp file: %v", err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "value.txt")
	var noCreds string
	ctx := context.Background()
	open, err := io.NewOpener(ctx, noCreds)
	if err != nil {
		t.Fatalf("Failed to create opener: %v", err)
	}
	st := syncTime{
		path:   path,
		opener: open,
		ctx:    ctx,
	}
	now := time.Now()
	if err := st.init(); err != nil {
		t.Fatalf("Failed init: %v", err)
	}
	cur := st.Current()
	if now.After(cur) {
		t.Fatalf("%v should be >= time before init was called: %v", cur, now)
	}

	earlier := now.Add(-time.Hour)
	later := now.Add(time.Hour)

	if err := st.Update(earlier); err != nil {
		t.Fatalf("Failed update: %v", err)
	}
	if actual := st.Current(); !actual.Equal(cur) {
		t.Errorf("Update(%v) should not have reduced value from %v, got %v", earlier, cur, actual)
	}

	if err := st.Update(later); err != nil {
		t.Fatalf("Failed update: %v", err)
	}
	if actual := st.Current(); !actual.After(cur) {
		t.Errorf("Update(%v) did not move current value to after %v, got %v", later, cur, actual)
	}
}
