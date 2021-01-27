/*
Copyright 2017 The Kubernetes Authors.

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

package sidecar

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strconv"
	"testing"
	"time"

	"k8s.io/test-infra/prow/entrypoint"
	"k8s.io/test-infra/prow/pod-utils/wrapper"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/sets"
)

var re = regexp.MustCompile(`(?m)(Failed to open) .*log\.txt: .*$`)

func TestWait(t *testing.T) {
	aborted := strconv.Itoa(entrypoint.AbortedErrorCode)
	skip := strconv.Itoa(entrypoint.PreviousErrorCode)
	const (
		pass = "0"
		fail = "1"
	)
	cases := []struct {
		name         string
		markers      []string
		abort        bool
		pass         bool
		accessDenied bool
		missing      bool
		failures     int
	}{
		{
			name:    "pass, not abort when 1 item passes",
			markers: []string{pass},
			pass:    true,
		},
		{
			name:    "pass when all items pass",
			markers: []string{pass, pass, pass},
			pass:    true,
		},
		{
			name:     "fail, not abort when 1 item fails",
			markers:  []string{fail},
			failures: 1,
		},
		{
			name:     "fail when any item fails",
			markers:  []string{pass, fail, pass},
			failures: 1,
		},
		{
			name:     "abort and fail when 1 item aborts",
			markers:  []string{aborted},
			abort:    true,
			failures: 1,
		},
		{
			name:     "abort when any item aborts",
			markers:  []string{pass, aborted, fail},
			abort:    true,
			failures: 2,
		},
		{
			name:     "fail when marker cannot be read",
			markers:  []string{pass, "not-an-exit-code", pass},
			failures: 1,
		},
		{
			name:     "fail when marker does not exist",
			markers:  []string{pass},
			missing:  true,
			failures: 1,
		},
		{
			name:     "count all failures",
			markers:  []string{pass, fail, aborted, skip, fail, pass},
			abort:    true,
			failures: 3,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir, err := ioutil.TempDir("", tc.name)
			if err != nil {
				t.Errorf("%s: error creating temp dir: %v", tc.name, err)
			}
			defer func() {
				if err := os.RemoveAll(tmpDir); err != nil {
					t.Errorf("%s: error cleaning up temp dir: %v", tc.name, err)
				}
			}()

			var entries []wrapper.Options

			for i, m := range tc.markers {
				p := path.Join(tmpDir, fmt.Sprintf("marker-%d.txt", i))
				var opt wrapper.Options
				opt.MarkerFile = p
				if err := ioutil.WriteFile(p, []byte(m), 0600); err != nil {
					t.Fatalf("could not create marker %d: %v", i, err)
				}
				entries = append(entries, opt)
			}

			ctx, cancel := context.WithCancel(context.Background())
			if tc.missing {
				entries = append(entries, wrapper.Options{MarkerFile: "missing-marker.txt"})
				go cancel()
			}

			pass, abort, failures := wait(ctx, entries)
			cancel()
			if pass != tc.pass {
				t.Errorf("expected pass %t != actual %t", tc.pass, pass)
			}
			if abort != tc.abort {
				t.Errorf("expected abort %t != actual %t", tc.abort, abort)
			}
			if failures != tc.failures {
				t.Errorf("expected failures %d != actual %d", tc.failures, failures)
			}
		})
	}
}

func TestWaitParallelContainers(t *testing.T) {
	aborted := strconv.Itoa(entrypoint.AbortedErrorCode)
	skip := strconv.Itoa(entrypoint.PreviousErrorCode)
	const (
		pass                 = "0"
		fail                 = "1"
		missingMarkerTimeout = time.Second
	)
	cases := []struct {
		name         string
		markers      []string
		abort        bool
		pass         bool
		accessDenied bool
		missing      bool
		failures     int
	}{
		{
			name:    "pass, not abort when 1 item passes",
			markers: []string{pass},
			pass:    true,
		},
		{
			name:    "pass when all items pass",
			markers: []string{pass, pass, pass},
			pass:    true,
		},
		{
			name:     "fail, not abort when 1 item fails",
			markers:  []string{fail},
			failures: 1,
		},
		{
			name:     "fail when any item fails",
			markers:  []string{pass, fail, pass},
			failures: 1,
		},
		{
			name:     "abort and fail when 1 item aborts",
			markers:  []string{aborted},
			abort:    true,
			failures: 1,
		},
		{
			name:     "abort when any item aborts",
			markers:  []string{pass, aborted, fail},
			abort:    true,
			failures: 2,
		},
		{
			name:     "fail when marker does not exist",
			markers:  []string{pass},
			missing:  true,
			failures: 1,
		},
		{
			name:     "count all failures",
			markers:  []string{pass, fail, aborted, skip, fail, pass},
			abort:    true,
			failures: 3,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir, err := ioutil.TempDir("", tc.name)
			if err != nil {
				t.Errorf("%s: error creating temp dir: %v", tc.name, err)
			}
			defer func() {
				if err := os.RemoveAll(tmpDir); err != nil {
					t.Errorf("%s: error cleaning up temp dir: %v", tc.name, err)
				}
			}()

			var entries []wrapper.Options

			for i := range tc.markers {
				p := path.Join(tmpDir, fmt.Sprintf("marker-%d.txt", i))
				var opt wrapper.Options
				opt.MarkerFile = p
				entries = append(entries, opt)
			}

			if tc.missing {
				missingPath := path.Join(tmpDir, "missing-marker.txt")
				entries = append(entries, wrapper.Options{MarkerFile: missingPath})
			}

			ctx, cancel := context.WithCancel(context.Background())

			type WaitResult struct {
				pass     bool
				abort    bool
				failures int
			}

			waitResultsCh := make(chan WaitResult)

			go func() {
				pass, abort, failures := wait(ctx, entries)
				waitResultsCh <- WaitResult{pass, abort, failures}
			}()

			errCh := make(chan error, len(tc.markers))
			for i, m := range tc.markers {

				options := entries[i]

				entrypointOptions := entrypoint.Options{
					Options: &options,
				}
				marker, err := strconv.Atoi(m)
				if err != nil {
					errCh <- fmt.Errorf("invalid exit code: %v", err)
				}
				go func() {
					errCh <- entrypointOptions.Mark(marker)
				}()

			}

			if tc.missing {
				go func() {
					select {
					case <-time.After(missingMarkerTimeout):
						cancel()
						errCh <- nil
					}
				}()
			}

			for range tc.markers {
				if err := <-errCh; err != nil {
					t.Fatalf("could not create marker: %v", err)
				}
			}

			waitRes := <-waitResultsCh

			cancel()
			if waitRes.pass != tc.pass {
				t.Errorf("expected pass %t != actual %t", tc.pass, waitRes.pass)
			}
			if waitRes.abort != tc.abort {
				t.Errorf("expected abort %t != actual %t", tc.abort, waitRes.abort)
			}
			if waitRes.failures != tc.failures {
				t.Errorf("expected failures %d != actual %d", tc.failures, waitRes.failures)
			}
		})
	}
}

func TestCombineMetadata(t *testing.T) {
	cases := []struct {
		name     string
		pieces   []string
		expected map[string]interface{}
	}{
		{
			name:   "no problem when metadata file is not there",
			pieces: []string{"missing"},
		},
		{
			name:   "simple metadata",
			pieces: []string{`{"hello": "world"}`},
			expected: map[string]interface{}{
				"hello": "world",
			},
		},
		{
			name: "merge pieces",
			pieces: []string{
				`{"hello": "hello", "world": "world", "first": 1}`,
				`{"hello": "hola", "world": "world", "second": 2}`,
			},
			expected: map[string]interface{}{
				"hello":  "hola",
				"world":  "world",
				"first":  1.0,
				"second": 2.0,
			},
		},
		{
			name: "errors go into sidecar-errors",
			pieces: []string{
				`{"hello": "there"}`,
				"missing",
				"read-error",
				"json-error", // this is invalid json
				`{"world": "thanks"}`,
			},
			expected: map[string]interface{}{
				"hello": "there",
				"world": "thanks",
				errorKey: map[string]error{
					name(2): errors.New("read"),
					name(3): errors.New("json"),
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir, err := ioutil.TempDir("", tc.name)
			if err != nil {
				t.Errorf("%s: error creating temp dir: %v", tc.name, err)
			}
			defer func() {
				if err := os.RemoveAll(tmpDir); err != nil {
					t.Errorf("%s: error cleaning up temp dir: %v", tc.name, err)
				}
			}()
			var entries []wrapper.Options

			for i, m := range tc.pieces {
				p := path.Join(tmpDir, fmt.Sprintf("metadata-%d.txt", i))
				var opt wrapper.Options
				opt.MetadataFile = p
				entries = append(entries, opt)
				if m == "missing" {
					continue
				} else if m == "read-error" {
					if err := os.Mkdir(p, 0700); err != nil {
						t.Fatalf("could not create %s: %v", p, err)
					}
					continue
				}
				// not-json is invalid json
				if err := ioutil.WriteFile(p, []byte(m), 0600); err != nil {
					t.Fatalf("could not create metadata %d: %v", i, err)
				}
			}

			actual := combineMetadata(entries)
			expectedErrors, _ := tc.expected[errorKey].(map[string]error)
			actualErrors, _ := actual[errorKey].(map[string]error)
			delete(tc.expected, errorKey)
			delete(actual, errorKey)
			if !equality.Semantic.DeepEqual(tc.expected, actual) {
				t.Errorf("maps do not match:\n%s", diff.ObjectReflectDiff(tc.expected, actual))
			}

			if !equality.Semantic.DeepEqual(sets.StringKeySet(expectedErrors), sets.StringKeySet(actualErrors)) { // ignore the error values
				t.Errorf("errors do not match:\n%s", diff.ObjectReflectDiff(expectedErrors, actualErrors))
			}
		})
	}
}

func name(idx int) string {
	return nameEntry(idx, wrapper.Options{})
}

func TestLogReaders(t *testing.T) {
	cases := []struct {
		name           string
		containerNames []string
		processLogs    map[string]string
		expected       map[string]string
	}{
		{
			name: "works with 1 container",
			containerNames: []string{
				"test",
			},
			processLogs: map[string]string{
				"process-log.txt": "hello world",
			},
			expected: map[string]string{
				"build-log.txt": "hello world",
			},
		},
		{
			name: "works with 1 container with no name",
			containerNames: []string{
				"",
			},
			processLogs: map[string]string{
				"process-log.txt": "hello world",
			},
			expected: map[string]string{
				"build-log.txt": "hello world",
			},
		},
		{
			name: "multiple logs works",
			containerNames: []string{
				"test1",
				"test2",
			},
			processLogs: map[string]string{
				"test1-log.txt": "hello",
				"test2-log.txt": "world",
			},
			expected: map[string]string{
				"test1-build-log.txt": "hello",
				"test2-build-log.txt": "world",
			},
		},
		{
			name: "note when a part has a problem",
			containerNames: []string{
				"test1",
				"test2",
				"test3",
			},
			processLogs: map[string]string{
				"test1-log.txt": "hello",
				"test2-log.txt": "missing",
				"test3-log.txt": "world",
			},
			expected: map[string]string{
				"test1-build-log.txt": "hello",
				"test2-build-log.txt": "Failed to open test2-log.txt: whatever\n",
				"test3-build-log.txt": "world",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir, err := ioutil.TempDir("", tc.name)
			if err != nil {
				t.Errorf("%s: error creating temp dir: %v", tc.name, err)
			}
			defer func() {
				if err := os.RemoveAll(tmpDir); err != nil {
					t.Errorf("%s: error cleaning up temp dir: %v", tc.name, err)
				}
			}()

			for name, log := range tc.processLogs {
				p := path.Join(tmpDir, name)
				if log == "missing" {
					continue
				}
				if err := ioutil.WriteFile(p, []byte(log), 0600); err != nil {
					t.Fatalf("could not create log %s: %v", name, err)
				}
			}

			var entries []wrapper.Options

			for _, containerName := range tc.containerNames {
				log := "process-log.txt"
				if len(tc.containerNames) > 1 {
					log = fmt.Sprintf("%s-log.txt", containerName)
				}
				p := path.Join(tmpDir, log)
				var opt wrapper.Options
				opt.ProcessLog = p
				opt.ContainerName = containerName
				entries = append(entries, opt)
			}

			readers := logReaders(entries)
			const repl = "$1 <SNIP>"
			actual := make(map[string]string)
			for name, reader := range readers {
				buf, err := ioutil.ReadAll(reader)
				if err != nil {
					t.Fatalf("failed to read all: %v", err)
				}
				actual[name] = re.ReplaceAllString(string(buf), repl)
			}

			for name, log := range tc.expected {
				tc.expected[name] = re.ReplaceAllString(log, repl)
			}

			if !equality.Semantic.DeepEqual(tc.expected, actual) {
				t.Errorf("maps do not match:\n%s", diff.ObjectReflectDiff(tc.expected, actual))
			}
		})
	}

}
