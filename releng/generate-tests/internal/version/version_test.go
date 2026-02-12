/*
Copyright 2026 The Kubernetes Authors.

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

package version_test

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/test-infra/releng/generate-tests/internal/version"
)

const testVersionString = "1.35"

func TestVersionString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		v    version.Version
		want string
	}{
		{version.Version{1, 35}, testVersionString},
		{version.Version{1, 0}, "1.0"},
		{version.Version{2, 10}, "2.10"},
	}
	for _, tc := range tests {
		if got := tc.v.String(); got != tc.want {
			t.Errorf("Version{%d,%d}.String() = %q, want %q", tc.v.Major, tc.v.Minor, got, tc.want)
		}
	}
}

func TestArgs(t *testing.T) {
	t.Parallel()

	args := version.Args(version.Version{1, 35})
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}

	if args[0] != "--extract=ci/latest-"+testVersionString {
		t.Errorf("got %q, want --extract=ci/latest-%s", args[0], testVersionString)
	}
}

func TestArgsFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		v    version.Version
		want string
	}{
		{version.Version{1, 35}, "--extract=ci/latest-" + testVersionString},
		{version.Version{1, 0}, "--extract=ci/latest-1.0"},
		{version.Version{2, 5}, "--extract=ci/latest-2.5"},
	}
	for _, tc := range tests {
		args := version.Args(tc.v)
		if args[0] != tc.want {
			t.Errorf("version.Args(%v) = %q, want %q", tc.v, args[0], tc.want)
		}
	}
}

func TestDiscoverVersions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	for _, name := range []string{"1.32.yaml", "1.33.yaml", "1.34.yaml", "1.35.yaml"} {
		err := os.WriteFile(filepath.Join(dir, name), []byte("periodics: []\n"), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}
	// Non-version file should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("test\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tiers, err := version.DiscoverVersions(dir)
	if err != nil {
		t.Fatalf("DiscoverVersions failed: %v", err)
	}

	if len(tiers) != version.NumTiers {
		t.Fatalf("expected %d tiers, got %d", version.NumTiers, len(tiers))
	}

	expected := []struct {
		marker   string
		version  string
		interval string
	}{
		{"beta", testVersionString, "1h"},
		{"stable1", "1.34", "2h"},
		{"stable2", "1.33", "6h"},
		{"stable3", "1.32", "24h"},
		{"stable4", "1.31", "24h"},
	}

	for idx, exp := range expected {
		if tiers[idx].Marker != exp.marker {
			t.Errorf("tier[%d] marker: got %q, want %q", idx, tiers[idx].Marker, exp.marker)
		}

		if tiers[idx].Version.String() != exp.version {
			t.Errorf(
				"tier[%d] version: got %q, want %q",
				idx, tiers[idx].Version.String(), exp.version,
			)
		}

		if tiers[idx].Interval != exp.interval {
			t.Errorf("tier[%d] interval: got %q, want %q", idx, tiers[idx].Interval, exp.interval)
		}
	}
}

func TestDiscoverVersionsEmpty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	_, err := version.DiscoverVersions(dir)
	if err == nil {
		t.Fatal("expected error for empty directory")
	}
}

func TestDiscoverVersionsSingleVersion(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "1.35.yaml"), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}

	tiers, err := version.DiscoverVersions(dir)
	if err != nil {
		t.Fatalf("DiscoverVersions failed: %v", err)
	}

	if len(tiers) != version.NumTiers {
		t.Fatalf("expected %d tiers, got %d", version.NumTiers, len(tiers))
	}

	if tiers[0].Version.String() != testVersionString {
		t.Errorf("beta version: got %q, want %s", tiers[0].Version.String(), testVersionString)
	}

	if tiers[4].Version.String() != "1.31" {
		t.Errorf("stable4 version: got %q, want 1.31", tiers[4].Version.String())
	}
}

func TestDiscoverVersionsNonContiguous(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for _, name := range []string{"1.30.yaml", "1.35.yaml"} {
		err := os.WriteFile(filepath.Join(dir, name), []byte{}, 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	tiers, err := version.DiscoverVersions(dir)
	if err != nil {
		t.Fatalf("DiscoverVersions failed: %v", err)
	}

	if tiers[0].Version.String() != testVersionString {
		t.Errorf("beta version: got %q, want %s", tiers[0].Version.String(), testVersionString)
	}
}

func TestDiscoverVersionsIgnoresNonYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "1.35.yaml"), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "1.34.txt"), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(filepath.Join(dir, "1.33.yaml"), 0o750); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "not-a-version.yaml"), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}

	tiers, err := version.DiscoverVersions(dir)
	if err != nil {
		t.Fatalf("DiscoverVersions failed: %v", err)
	}

	if tiers[0].Version.String() != testVersionString {
		t.Errorf("beta version: got %q, want %s", tiers[0].Version.String(), testVersionString)
	}
}

func TestDiscoverVersionsInvalidDir(t *testing.T) {
	t.Parallel()

	_, err := version.DiscoverVersions("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
}

func TestDiscoverVersionsSortsCorrectly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Write files in non-sorted order.
	for _, name := range []string{"1.33.yaml", "1.35.yaml", "1.32.yaml", "1.34.yaml"} {
		err := os.WriteFile(filepath.Join(dir, name), []byte{}, 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	tiers, err := version.DiscoverVersions(dir)
	if err != nil {
		t.Fatalf("DiscoverVersions failed: %v", err)
	}
	// Max should be 1.35 regardless of file creation order.
	if tiers[0].Version.String() != testVersionString {
		t.Errorf("beta version: got %q, want %s", tiers[0].Version.String(), testVersionString)
	}
}
