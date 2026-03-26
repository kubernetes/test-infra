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

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadGitHubTokenFromFile(t *testing.T) {
	t.Parallel()

	tokenPath := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenPath, []byte(" token-from-file \n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	token, err := loadGitHubToken(&options{githubTokenPath: tokenPath})
	if err != nil {
		t.Fatalf("loadGitHubToken() returned error: %v", err)
	}
	if token != "token-from-file" {
		t.Fatalf("loadGitHubToken() = %q, want %q", token, "token-from-file")
	}
}

func TestLoadGitHubTokenFromEmptyFile(t *testing.T) {
	t.Parallel()

	tokenPath := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenPath, []byte(" \n "), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	if _, err := loadGitHubToken(&options{githubTokenPath: tokenPath}); err == nil {
		t.Fatal("loadGitHubToken() error = nil, want error")
	}
}

func TestLoadGitHubTokenFromEnv(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "token-from-env")

	token, err := loadGitHubToken(&options{})
	if err != nil {
		t.Fatalf("loadGitHubToken() returned error: %v", err)
	}
	if token != "token-from-env" {
		t.Fatalf("loadGitHubToken() = %q, want %q", token, "token-from-env")
	}
}
