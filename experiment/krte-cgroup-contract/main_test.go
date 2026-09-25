/*
Copyright The Kubernetes Authors.

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
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseProcCgroup(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantV2  string
		wantCPU string // v1 cpu path, "" if none
		isErr   bool
	}{
		{"unified", "0::/a/b\n", "/a/b", "", false},
		{"legacy v1", "4:cpu,cpuacct:/x\n3:memory:/x\n", "", "/x", false},
		{"hybrid cpu on v1", "0::/only/v2\n4:cpu,cpuacct:/y\n", "/only/v2", "/y", false},
		{"malformed v2 path", "0::relative\n", "", "", true},
		{"blank lines ignored", "\n0::/z\n\n", "/z", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v2, v1, err := parseProcCgroup(strings.NewReader(tc.in))
			if (err != nil) != tc.isErr {
				t.Fatalf("err = %v, want error %v", err, tc.isErr)
			}
			if err != nil {
				return
			}
			if v2 != tc.wantV2 {
				t.Errorf("v2 = %q, want %q", v2, tc.wantV2)
			}
			if got := v1["cpu"]; got != tc.wantCPU {
				t.Errorf("v1[cpu] = %q, want %q", got, tc.wantCPU)
			}
		})
	}
}

func TestParseMounts(t *testing.T) {
	// 3 4 0:5 / /sys/fs/cgroup ro - cgroup2 cgroup2 rw
	mi := strings.Join([]string{
		`31 23 0:26 / /sys/fs/cgroup rw shared:9 - cgroup2 cgroup2 rw`,
		`40 31 0:35 /x /sys/fs/cgroup/x rw - cgroup2 cgroup2 rw`,
		`50 23 0:60 / /sys/fs/cgroup/cpu rw - cgroup cgroup rw,cpu,cpuacct`,
		`60 23 0:61 / /mnt/notcgroup rw - tmpfs tmpfs rw`,
		`70 23 0:70 /a\040b /sys/fs/cgroup/esc rw - cgroup2 cgroup2 rw`,
	}, "\n") + "\n"
	mounts, err := parseMounts(strings.NewReader(mi))
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 4 {
		t.Fatalf("got %d cgroup mounts, want 4 (tmpfs excluded)", len(mounts))
	}
	var v1cpu, escaped bool
	for _, m := range mounts {
		if m.version == 1 && contains(m.options, "cpu") {
			v1cpu = true
		}
		if m.root == "/a b" {
			escaped = true
		}
	}
	if !v1cpu {
		t.Error("did not detect the v1 cpu mount")
	}
	if !escaped {
		t.Error("did not unescape the mount root")
	}
}

func TestPickVisibleV2Mount(t *testing.T) {
	mounts := []cgroupMount{
		{version: 2, root: "/", point: "/sys/fs/cgroup"},
		{version: 2, root: "/x", point: "/sys/fs/cgroup"},
		{version: 1, root: "/", point: "/sys/fs/cgroup/cpu", options: []string{"cpu"}},
	}
	tests := []struct {
		name      string
		cgroup    string
		wantRoot  string
		wantFound bool
	}{
		{"longest prefix wins", "/x/y", "/x", true},
		{"root fallback", "/other", "/", true},
		{"exact root match", "/x", "/x", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, ok := pickVisibleV2Mount(tc.cgroup, mounts)
			if ok != tc.wantFound {
				t.Fatalf("found = %v, want %v", ok, tc.wantFound)
			}
			if ok && m.root != tc.wantRoot {
				t.Errorf("root = %q, want %q", m.root, tc.wantRoot)
			}
		})
	}

	if _, ok := pickVisibleV2Mount("/a", []cgroupMount{{version: 2, root: "/b", point: "/p"}}); ok {
		t.Error("expected no visible mount when nothing contains the cgroup")
	}
}

func TestParseCPUMax(t *testing.T) {
	tests := []struct {
		name  string
		raw   string
		want  float64
		ok    bool
		isErr bool
	}{
		{"unlimited", "max 100000", 0, false, false},
		{"two cpus", "200000 100000", 2, true, false},
		{"fractional", "50000 100000", 0.5, true, false},
		{"empty", "", 0, false, false},
		{"absent", "<absent>", 0, false, false},
		{"missing period", "200000", 0, false, true},
		{"bad quota", "abc 100000", 0, false, true},
		{"zero period", "200000 0", 0, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok, err := parseCPUMax(tc.raw)
			if (err != nil) != tc.isErr {
				t.Fatalf("err = %v, want error %v", err, tc.isErr)
			}
			if err == nil && (got != tc.want || ok != tc.ok) {
				t.Errorf("got (%v, %v), want (%v, %v)", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestFullLeaf(t *testing.T) {
	tests := []struct {
		name   string
		cgroup string
		mount  cgroupMount
		want   string
		isErr  bool
	}{
		{"root mount", "/a/b", cgroupMount{root: "/", point: "/sys/fs/cgroup"}, "/sys/fs/cgroup/a/b", false},
		{"namespaced at root", "/x", cgroupMount{root: "/x", point: "/sys/fs/cgroup"}, "/sys/fs/cgroup", false},
		{"namespaced child", "/x/y", cgroupMount{root: "/x", point: "/sys/fs/cgroup"}, "/sys/fs/cgroup/y", false},
		{"outside root", "/z", cgroupMount{root: "/x", point: "/sys/fs/cgroup"}, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := fullLeaf(tc.cgroup, tc.mount)
			if (err != nil) != tc.isErr {
				t.Fatalf("err = %v, want error %v", err, tc.isErr)
			}
			if err == nil && got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestUnescapeMount(t *testing.T) {
	tests := []struct{ in, want string }{
		{`plain`, `plain`},
		{`a\040b`, "a b"},
		{`tab\011here`, "tab\there"},
		{`back\134slash`, `back\slash`},
	}
	for _, tc := range tests {
		if got := unescapeMount(tc.in); got != tc.want {
			t.Errorf("unescapeMount(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// writeSummary must not lose the child entry when the optional host-side cgroup
// file is missing.
func TestWriteSummaryMissingHostCgroup(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, s snapshot) {
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("outer.json", snapshot{Label: "outer", CgroupMode: "unified", GOMAXPROCS: 2, HasVisibleCPULimit: true, MinimumVisibleCPUQuota: 2})
	write("docker-child.json", snapshot{Label: "child", CgroupMode: "unified", GOMAXPROCS: 20, Errors: []string{"x"}})

	if err := writeSummary(dir); err != nil {
		t.Fatalf("writeSummary: %v", err)
	}
	var doc summaryDoc
	data, err := os.ReadFile(filepath.Join(dir, "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.OuterForced == nil || doc.DockerChildForced == nil {
		t.Fatalf("outer/child summary entries missing: %+v", doc)
	}
	if doc.OuterForced.GOMAXPROCS != 2 || doc.DockerChildForced.GOMAXPROCS != 20 {
		t.Errorf("gomaxprocs outer=%d child=%d, want 2/20", doc.OuterForced.GOMAXPROCS, doc.DockerChildForced.GOMAXPROCS)
	}
	if doc.DockerChildForced.Errors != 1 {
		t.Errorf("child errors = %d, want 1", doc.DockerChildForced.Errors)
	}
	if doc.DockerChildForced.OuterNamespaceCgroup != "" {
		t.Errorf("host cgroup should be empty when the file is missing, got %q", doc.DockerChildForced.OuterNamespaceCgroup)
	}
}

func TestWithin(t *testing.T) {
	cases := []struct {
		path, point string
		want        bool
	}{
		{"/a", "/", true}, // root mount point: the case the old prefix check missed
		{"/", "/", true},
		{"/a/b", "/a", true},
		{"/a", "/a", true},
		{"/ab", "/a", false},  // shared prefix but not a subpath
		{"/a", "/a/b", false}, // path is above the point
	}
	for _, c := range cases {
		if got := within(c.path, c.point); got != c.want {
			t.Errorf("within(%q, %q) = %v, want %v", c.path, c.point, got, c.want)
		}
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
