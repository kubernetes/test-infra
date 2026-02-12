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

package release_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/test-infra/releng/prepare-release-branch/internal/release"
)

func createVersionFiles(t *testing.T, dir string, names ...string) {
	t.Helper()

	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte{}, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGetConfigFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	createVersionFiles(t, dir, "1.32.yaml", "1.33.yaml", "1.34.yaml", "1.35.yaml")

	// Non-yaml file should not be matched.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}

	files, err := release.GetConfigFiles(dir)
	if err != nil {
		t.Fatalf("GetConfigFiles failed: %v", err)
	}

	if len(files) != 4 {
		t.Errorf("expected 4 files, got %d", len(files))
	}
}

func TestCheckVersion(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	createVersionFiles(t, dir, "1.32.yaml", "1.33.yaml", "1.34.yaml", "1.35.yaml")

	ver, err := release.CheckVersion(dir)
	if err != nil {
		t.Fatalf("CheckVersion failed: %v", err)
	}

	if ver.Major != 1 || ver.Minor != 35 {
		t.Errorf("expected 1.35, got %s", ver.String())
	}
}

func TestCheckVersionThreeFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	createVersionFiles(t, dir, "1.33.yaml", "1.34.yaml", "1.35.yaml")

	ver, err := release.CheckVersion(dir)
	if err != nil {
		t.Fatalf("CheckVersion failed: %v", err)
	}

	if ver.Major != 1 || ver.Minor != 35 {
		t.Errorf("expected 1.35, got %s", ver.String())
	}
}

func TestCheckVersionFiveFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	createVersionFiles(t, dir,
		"1.31.yaml", "1.32.yaml", "1.33.yaml", "1.34.yaml", "1.35.yaml",
	)

	ver, err := release.CheckVersion(dir)
	if err != nil {
		t.Fatalf("CheckVersion failed: %v", err)
	}

	if ver.Major != 1 || ver.Minor != 35 {
		t.Errorf("expected 1.35, got %s", ver.String())
	}
}

func TestCheckVersionTooFew(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	createVersionFiles(t, dir, "1.34.yaml", "1.35.yaml")

	_, err := release.CheckVersion(dir)
	if err == nil {
		t.Fatal("expected error for too few config files")
	}

	if !errors.Is(err, release.ErrIncorrectConfigCount) {
		t.Errorf("expected ErrIncorrectConfigCount, got: %v", err)
	}
}

func TestCheckVersionTooMany(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	createVersionFiles(t, dir,
		"1.30.yaml", "1.31.yaml", "1.32.yaml",
		"1.33.yaml", "1.34.yaml", "1.35.yaml",
	)

	_, err := release.CheckVersion(dir)
	if err == nil {
		t.Fatal("expected error for too many config files")
	}

	if !errors.Is(err, release.ErrIncorrectConfigCount) {
		t.Errorf("expected ErrIncorrectConfigCount, got: %v", err)
	}
}

func TestCheckVersionNonSequential(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	createVersionFiles(t, dir, "1.32.yaml", "1.34.yaml", "1.35.yaml")

	_, err := release.CheckVersion(dir)
	if err == nil {
		t.Fatal("expected error for non-sequential versions")
	}

	if !errors.Is(err, release.ErrNonSequential) {
		t.Errorf("expected ErrNonSequential, got: %v", err)
	}
}

func TestCheckVersionEmpty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	_, err := release.CheckVersion(dir)
	if err == nil {
		t.Fatal("expected error for empty directory")
	}

	if !errors.Is(err, release.ErrIncorrectConfigCount) {
		t.Errorf("expected ErrIncorrectConfigCount, got: %v", err)
	}
}

func TestDeleteStaleBranch(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	createVersionFiles(t, dir, "1.32.yaml", "1.33.yaml", "1.34.yaml", "1.35.yaml")

	ver := release.Version{Major: 1, Minor: 35}
	if err := release.DeleteStaleBranch(dir, ver); err != nil {
		t.Fatalf("DeleteStaleBranch failed: %v", err)
	}

	// 1.32.yaml should be deleted (35 - 3 = 32).
	if _, err := os.Stat(filepath.Join(dir, "1.32.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Error("expected 1.32.yaml to be deleted")
	}

	// Other files should remain.
	for _, name := range []string{"1.33.yaml", "1.34.yaml", "1.35.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected %s to still exist", name)
		}
	}
}

func TestDeleteStaleBranchMissing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	ver := release.Version{Major: 1, Minor: 35}
	if err := release.DeleteStaleBranch(dir, ver); err != nil {
		t.Fatalf("DeleteStaleBranch should not error on missing file: %v", err)
	}
}

func TestVersionString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ver  release.Version
		want string
	}{
		{release.Version{Major: 1, Minor: 35}, "1.35"},
		{release.Version{Major: 1, Minor: 0}, "1.0"},
		{release.Version{Major: 2, Minor: 10}, "2.10"},
	}

	for _, tc := range tests {
		if got := tc.ver.String(); got != tc.want {
			t.Errorf("Version{%d,%d}.String() = %q, want %q",
				tc.ver.Major, tc.ver.Minor, got, tc.want)
		}
	}
}

func TestVersionFilename(t *testing.T) {
	t.Parallel()

	ver := release.Version{Major: 1, Minor: 35}
	if got := ver.Filename(); got != "1.35.yaml" {
		t.Errorf("Filename() = %q, want 1.35.yaml", got)
	}
}
