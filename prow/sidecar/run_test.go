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
	"strings"
	"testing"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/entrypoint"
	"k8s.io/test-infra/prow/pod-utils/wrapper"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/sets"
)

func TestGetRevisionFromRef(t *testing.T) {
	var tests = []struct {
		name     string
		refs     *prowapi.Refs
		expected string
	}{
		{
			name: "Refs with Pull",
			refs: &prowapi.Refs{
				BaseRef: "master",
				BaseSHA: "deadbeef",
				Pulls: []prowapi.Pull{
					{
						Number: 123,
						SHA:    "abcd1234",
					},
				},
			},
			expected: "abcd1234",
		},
		{
			name: "Refs with BaseSHA",
			refs: &prowapi.Refs{
				BaseRef: "master",
				BaseSHA: "deadbeef",
			},
			expected: "deadbeef",
		},
		{
			name: "Refs with BaseRef",
			refs: &prowapi.Refs{
				BaseRef: "master",
			},
			expected: "master",
		},
	}

	for _, test := range tests {
		if actual, expected := getRevisionFromRef(test.refs), test.expected; actual != expected {
			t.Errorf("%s: got revision:%s but expected: %s", test.name, actual, expected)
		}
	}
}

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

func TestLogReader(t *testing.T) {
	cases := []struct {
		name     string
		pieces   []string
		expected []string
	}{
		{
			name:     "basically works",
			pieces:   []string{"hello world"},
			expected: []string{"hello world"},
		},
		{
			name:   "multiple logging works",
			pieces: []string{"first", "second"},
			expected: []string{
				start(name(0)),
				"first",
				start(name(1)),
				"second",
			},
		},
		{
			name:   "note when a part has aproblem",
			pieces: []string{"first", "missing", "third"},
			expected: []string{
				start(name(0)),
				"first",
				start(name(1)),
				"Failed to open log-1.txt: whatever\n",
				start(name(2)),
				"third",
			},
		},
	}

	re := regexp.MustCompile(`(?m)(Failed to open) .*log-\d.txt: .*$`)
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
				p := path.Join(tmpDir, fmt.Sprintf("log-%d.txt", i))
				var opt wrapper.Options
				opt.ProcessLog = p
				entries = append(entries, opt)
				if m == "missing" {
					continue
				}
				if err := ioutil.WriteFile(p, []byte(m), 0600); err != nil {
					t.Fatalf("could not create log %d: %v", i, err)
				}
			}

			buf, err := ioutil.ReadAll(logReader(entries))
			if err != nil {
				t.Fatalf("failed to read all: %v", err)
			}
			const repl = "$1 <SNIP>"
			actual := re.ReplaceAllString(string(buf), repl)
			expected := re.ReplaceAllString(strings.Join(tc.expected, ""), repl)
			if !equality.Semantic.DeepEqual(expected, actual) {
				t.Errorf("maps do not match:\n%s", diff.ObjectReflectDiff(expected, actual))
			}
		})
	}

}
