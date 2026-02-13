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

package run_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"k8s.io/test-infra/releng/prepare-release-branch/internal/release"
	"k8s.io/test-infra/releng/prepare-release-branch/internal/run"
)

func TestSuffixes(t *testing.T) {
	t.Parallel()

	suffixes := run.Suffixes()

	expected := []string{"beta", "stable1", "stable2", "stable3", "stable4"}
	if !slices.Equal(suffixes, expected) {
		t.Errorf("Suffixes() = %v, want %v", suffixes, expected)
	}
}

func TestFetchGoVersion(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("1.24.0\n"))
	}))

	defer server.Close()

	version, err := run.FetchGoVersion(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if version != "1.24.0" {
		t.Errorf("version = %q, want 1.24.0", version)
	}
}

func TestFetchGoVersionNotFound(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
	}))

	defer server.Close()

	_, err := run.FetchGoVersion(context.Background(), server.URL)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}

	if !errors.Is(err, run.ErrBranchNotFound) {
		t.Errorf("expected ErrBranchNotFound, got: %v", err)
	}
}

func TestFetchGoVersionServerError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
	}))

	defer server.Close()

	_, err := run.FetchGoVersion(context.Background(), server.URL)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}

	if !errors.Is(err, run.ErrHTTPStatus) {
		t.Errorf("expected ErrHTTPStatus, got: %v", err)
	}
}

func TestFetchGoVersionCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := run.FetchGoVersion(ctx, "http://localhost:0")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestGoVersionURL(t *testing.T) {
	t.Parallel()

	version := release.Version{Major: 1, Minor: 35}

	got := run.GoVersionURL(version)
	want := "https://raw.githubusercontent.com/kubernetes/kubernetes/release-1.36/.go-version"

	if got != want {
		t.Errorf("GoVersionURL() = %q, want %q", got, want)
	}
}
