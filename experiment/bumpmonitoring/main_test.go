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

package main

import (
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestFindConfigToUpdate(t *testing.T) {
	tests := []struct {
		name     string
		desc     string
		srcNodes []node
		dstNodes []node
		want     []string
		wantErr  bool
	}{
		{
			name: "base",
			dstNodes: []node{
				{Path: "mixins/grafana_dashboards/a.jsonnet"},
				{Path: "mixins/prometheus/a.libsonnet"},
			},
			want: []string{
				"mixins/grafana_dashboards/a.jsonnet",
				"mixins/prometheus/a.libsonnet",
			},
			wantErr: false,
		},
		{
			name: "multiple-files",
			dstNodes: []node{
				{Path: "mixins/grafana_dashboards/a.jsonnet"},
				{Path: "mixins/grafana_dashboards/b.jsonnet"},
			},
			want: []string{
				"mixins/grafana_dashboards/a.jsonnet",
				"mixins/grafana_dashboards/b.jsonnet",
			},
			wantErr: false,
		},
		{
			name: "wrong-extension",
			dstNodes: []node{
				{Path: "mixins/grafana_dashboards/a.somethingelse"},
			},
			wantErr: false,
		},
		{
			name: "excluded-file",
			dstNodes: []node{
				{Path: "mixins/grafana_dashboards/prometheus.libsonnet"},
			},
			wantErr: false,
		},
		{
			name: "empty",
		},
		{
			name: "empty dir",
			dstNodes: []node{
				{Path: "mixins/grafana_dashboards", IsDir: true},
			},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir, err := ioutil.TempDir("", tc.name)
			if err != nil {
				t.Fatalf("Failed creating temp dir: %v", err)
			}
			t.Cleanup(func() {
				os.RemoveAll(tmpDir)
			})
			srcRootDir, dstRootDir := seedTempDir(t, tmpDir, tc.srcNodes, tc.dstNodes)
			c := client{
				srcPath: srcRootDir,
				dstPath: dstRootDir,
			}
			if wantErr, gotErr := tc.wantErr, c.findConfigToUpdate(); (wantErr && (gotErr == nil)) || (!wantErr && (gotErr != nil)) {
				t.Fatalf("Error mismatch. want: %v, got: %v", wantErr, gotErr)
			}
			if diff := cmp.Diff(tc.want, c.paths, cmpopts.SortSlices(func(a, b string) bool {
				return a < b
			})); diff != "" {
				t.Fatalf("Config files mismatch. want(-), got(+):\n%s", diff)
			}
		})
	}
}

func TestCopyFiles(t *testing.T) {
	tests := []struct {
		name     string
		desc     string
		srcNodes []node
		dstNodes []node
		paths    []string
		want     []node
		wantErr  bool
	}{
		{
			name: "base",
			srcNodes: []node{
				{Path: "mixins/grafana_dashboards/a.jsonnet", Content: "123"},
				{Path: "mixins/prometheus/a.libsonnet", Content: "123"},
			},
			dstNodes: []node{
				{Path: "mixins/grafana_dashboards/a.jsonnet", Content: "456"},
				{Path: "mixins/prometheus/a.libsonnet", Content: "456"},
			},
			paths: []string{
				"mixins/grafana_dashboards/a.jsonnet",
				"mixins/prometheus/a.libsonnet",
			},
			want: []node{
				{Path: "mixins/grafana_dashboards/a.jsonnet", Content: "123"},
				{Path: "mixins/prometheus/a.libsonnet", Content: "123"},
			},
			wantErr: false,
		},
		{
			name: "not exist upstream",
			srcNodes: []node{
				{Path: "mixins/grafana_dashboards/b.jsonnet"},
			},
			dstNodes: []node{
				{Path: "mixins/grafana_dashboards/a.jsonnet"},
			},
			paths: []string{
				"mixins/grafana_dashboards/a.jsonnet",
			},
			want: []node{
				{Path: "mixins/grafana_dashboards/a.jsonnet"},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir, err := ioutil.TempDir("", tc.name)
			if err != nil {
				t.Fatalf("Failed creating temp dir: %v", err)
			}
			t.Cleanup(func() {
				os.RemoveAll(tmpDir)
			})
			srcRootDir, dstRootDir := seedTempDir(t, tmpDir, tc.srcNodes, tc.dstNodes)
			c := client{
				srcPath: srcRootDir,
				dstPath: dstRootDir,
				paths:   tc.paths,
			}
			if wantErr, gotErr := tc.wantErr, c.copyFiles(); (wantErr && (gotErr == nil)) || (!wantErr && (gotErr != nil)) {
				t.Fatalf("Error mismatch. want: %v, got: %v", wantErr, gotErr)
			}
			var got []node
			for _, p := range tc.paths {
				got = append(got, pathToNode(t, dstRootDir, p))
			}
			if diff := cmp.Diff(tc.want, got, cmpopts.SortSlices(func(a, b string) bool {
				return a < b
			})); diff != "" {
				t.Fatalf("Config files mismatch. want(-), got(+):\n%s", diff)
			}
		})
	}
}

type node struct {
	Path    string
	Content string
	IsDir   bool
}

func pathToNode(t *testing.T, root, p string) node {
	n := node{Path: p}

	p = path.Join(root, p)
	info, err := os.Lstat(p)
	if err != nil {
		t.Fatalf("Failed stats %q: %v", p, err)
	}
	if info.IsDir() {
		n.IsDir = true
	} else {
		bs, err := ioutil.ReadFile(p)
		if err != nil {
			t.Fatalf("Failed to read %q: %v", p, err)
		}
		n.Content = string(bs)
	}

	return n
}

func seedTempDir(t *testing.T, root string, srcNodes, dstNodes []node) (string, string) {
	srcRootDir := path.Join(root, "src")
	dstRootDir := path.Join(root, "dst")

	create := func(t *testing.T, root string, ns []node) {
		for _, n := range ns {
			p := path.Join(root, n.Path)
			if n.IsDir {
				if err := os.MkdirAll(p, 0755); err != nil {
					t.Fatalf("Failed creating dir %q: %v", p, err)
				}
			} else {
				dir := path.Dir(p)
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatalf("Failed creating dir %q: %v", dir, err)
				}
				if err := ioutil.WriteFile(p, []byte(n.Content), 0777); err != nil {
					t.Fatalf("Failed creating file %q: %v", p, err)
				}
			}
		}
	}

	create(t, srcRootDir, srcNodes)
	create(t, dstRootDir, dstNodes)

	return srcRootDir, dstRootDir
}
