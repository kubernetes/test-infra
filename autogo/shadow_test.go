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
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

type fakeInfo struct {
	dir bool
}

func (i fakeInfo) Name() string {
	return "fake"
}

func (i fakeInfo) Size() int64 {
	return 0
}
func (i fakeInfo) Mode() os.FileMode {
	return 0
}

func (i fakeInfo) ModTime() time.Time {
	return time.Now()
}

func (i fakeInfo) IsDir() bool {
	return i.dir
}
func (i fakeInfo) Sys() interface{} {
	return nil
}

func TestChoose(t *testing.T) {
	cases := []struct {
		name     string
		origin   string
		dest     string
		path     string
		dir      bool
		verr     bool
		skip     bool
		err      bool
		action   bool
		destpath string
	}{
		{
			name:     "mkdir",
			origin:   "something",
			dest:     "hello",
			path:     "something/foo",
			dir:      true,
			action:   true,
			destpath: "hello/foo",
		},
		{
			name:     "link .go",
			origin:   "foo",
			dest:     "bar",
			path:     "foo/hello/world.go",
			action:   true,
			destpath: "bar/hello/world.go",
		},
		{
			name:     "link .s",
			origin:   "foo",
			dest:     "bar",
			path:     "foo/something.s",
			action:   true,
			destpath: "bar/something.s",
		},
		{
			name:   "skip random file",
			origin: "foo",
			path:   "foo/something.random",
		},
		{
			name:   "skip testdata dir",
			origin: "foo",
			path:   "foo/good/testdata",
			dir:    true,
			skip:   true,
		},
		{
			name:   "error file in testdata",
			origin: "foo",
			path:   "foo/testdata/unexpected.go",
			err:    true,
		},
		{
			name:   "error on verr",
			origin: "foo",
			path:   "foo/error.go",
			verr:   true,
			err:    true,
		},
		{
			name:   "error on foreign path",
			origin: "foo",
			path:   "not-relative-to-foo",
			err:    true,
		},
		{
			name:   "error on non-child path",
			origin: "foo",
			path:   "foo/../bar",
			err:    true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := visitor{
				origin: tc.origin,
				dest:   tc.dest,
			}
			i := fakeInfo{
				dir: tc.dir,
			}
			var verr error
			if tc.verr {
				verr = errors.New("verr")
			}
			act, dest, err := v.choose(tc.path, i, verr)
			switch {
			case tc.skip:
				if err != filepath.SkipDir {
					t.Errorf("error %v is not SkipDir", err)
				}
			case err != nil && !tc.err:
				t.Errorf("unexpected error: %v", err)
			case err == nil && tc.err:
				t.Errorf("failed to raise an error")
			case act == nil && tc.action:
				t.Errorf("failed to receive an action")
			case act != nil && !tc.action:
				t.Errorf("unexpected action: %v", act)
			case dest != tc.destpath:
				t.Errorf("wrong destionation %s != expected %s", dest, tc.dest)
			}
		})
	}
}

func TestShadowClone(t *testing.T) {
	cases := []struct {
		name   string
		origin []string
		dest   []string
	}{
		{
			name: "basic",
			origin: []string{
				"foo/hello.go",
				"foo/yes.s",
				"foo/skip.random",
				"bar/something/nice.go",
				"totally/ignore/this.txt",
				"skip/testdata/woops.go",
			},
			dest: []string{
				"foo/hello.go",
				"foo/yes.s",
				"bar/something/nice.go",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o, err := ioutil.TempDir("", "shadow-origin")
			if err != nil {
				t.Fatalf("failed to create shadow-origin: %v", err)
			}
			defer os.RemoveAll(o)
			for _, op := range tc.origin {
				p := filepath.Join(o, op)
				d := filepath.Dir(p)
				if err := os.MkdirAll(d, 0700); err != nil {
					t.Fatalf("failed to create %s: %v", d, err)
				}
				if err := ioutil.WriteFile(p, []byte(op), 0600); err != nil {
					t.Fatalf("failed to create %s: %v", p, err)
				}
			}
			d, err := ioutil.TempDir("", "shadow-dest")
			if err != nil {
				t.Fatalf("failed to create shadow-dest: %v", err)
			}
			defer os.RemoveAll(d)
			if err = shadowClone(o, d); err != nil {
				t.Fatalf("shadowClone() failed: %v", err)
			}
			found := []string{}
			err = filepath.Walk(d, func(path string, info os.FileInfo, verr error) error {
				if verr != nil {
					return verr
				}
				if info.IsDir() {
					return nil
				}
				rel, err := filepath.Rel(d, path)
				if err != nil {
					return err
				}
				found = append(found, rel)
				return nil
			})
			if err != nil {
				t.Fatalf("failed to walk %s: %v", d, err)
			}
			sort.Strings(tc.dest)
			sort.Strings(found)
			if !reflect.DeepEqual(found, tc.dest) {
				t.Errorf("actual %s != expected %s", found, tc.dest)
			}
		})
	}
}
