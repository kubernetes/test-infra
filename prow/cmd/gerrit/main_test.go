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
	"errors"
	"flag"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/pkg/io"
	"k8s.io/test-infra/prow/gerrit/client"
)

type fakeOpener struct{}

func (o fakeOpener) Reader(ctx context.Context, path string) (io.ReadCloser, error) {
	return nil, storage.ErrObjectNotExist
}

func (o fakeOpener) Writer(ctx context.Context, path string) (io.WriteCloser, error) {
	return nil, errors.New("do not call Writer")
}

func TestFlags(t *testing.T) {
	cases := []struct {
		name     string
		args     map[string]string
		del      sets.String
		expected func(*options)
		err      bool
	}{
		{
			name: "minimal flags work",
		},
		{
			name: "expicitly set --dry-run=false",
			args: map[string]string{
				"--dry-run": "false",
			},
			expected: func(o *options) {
				o.dryRun = false
			},
		},
		{
			name: "explicitly set --dry-run=true",
			args: map[string]string{
				"--dry-run": "true",
			},
			expected: func(o *options) {
				o.dryRun = true
			},
		},
		{
			name:     "dry run defaults to false",
			del:      sets.NewString("--dry-run"),
			expected: func(o *options) {},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expected := &options{
				projects:         client.ProjectsFlag{},
				lastSyncFallback: "gs://path",
				configPath:       "yo",
				dryRun:           false,
			}
			expected.projects.Set("foo=bar")
			if tc.expected != nil {
				tc.expected(expected)
			}
			argMap := map[string]string{
				"--gerrit-projects":    "foo=bar",
				"--last-sync-fallback": "gs://path",
				"--config-path":        "yo",
				"--dry-run":            "false",
			}
			for k, v := range tc.args {
				argMap[k] = v
			}
			for k := range tc.del {
				delete(argMap, k)
			}

			var args []string
			for k, v := range argMap {
				args = append(args, k+"="+v)
			}
			fs := flag.NewFlagSet("fake-flags", flag.PanicOnError)
			actual := gatherOptions(fs, args...)
			switch err := actual.Validate(); {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Errorf("failed to receive expected error")
			case !reflect.DeepEqual(*expected, actual):
				t.Errorf("%#v != expected %#v", actual, *expected)
			}
		})
	}
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

	expected := later.Truncate(time.Second)
	st = syncTime{
		path:   path,
		opener: open,
		ctx:    ctx,
	}
	if err := st.init(); err != nil {
		t.Fatalf("Failed init: %v", err)
	}
	if actual := st.Current(); !actual.Equal(expected) {
		t.Errorf("init() failed to reload %v, got %v", expected, actual)
	}

	st = syncTime{
		path:   path,
		opener: fakeOpener{}, // return storage.ErrObjectNotExist on open
		ctx:    ctx,
	}
	if err := st.init(); err != nil {
		t.Fatalf("Failed init: %v", err)
	}
	if actual := st.Current(); now.After(actual) || actual.After(later) {
		t.Fatalf("should initialize to start %v <= actual <= later %v, but got %v", now, later, actual)
	}
}
