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
	"path/filepath"
	"slices"
	"testing"

	"k8s.io/test-infra/releng/prepare-release-branch/internal/release"
	"k8s.io/test-infra/releng/prepare-release-branch/internal/run"
)

// errMockCommand is a sentinel error for testing command failures.
var errMockCommand = errors.New("mock command error")

// mockCommander records Run calls for verification.
type mockCommander struct {
	calls [][]string
	err   error
}

func (m *mockCommander) Run(_ context.Context, name string, args ...string) error {
	m.calls = append(m.calls, append([]string{name}, args...))

	return m.err
}

func TestSuffixes(t *testing.T) {
	t.Parallel()

	suffixes := run.Suffixes()

	expected := []string{"beta", "stable1", "stable2", "stable3", "stable4"}
	if !slices.Equal(suffixes, expected) {
		t.Errorf("Suffixes() = %v, want %v", suffixes, expected)
	}
}

func TestRotateFiles(t *testing.T) {
	t.Parallel()

	mock := &mockCommander{calls: nil, err: nil}
	version := release.Version{Major: 1, Minor: 35}

	err := run.RotateFiles(context.Background(), mock, "/bin/rotator", "/path/to/branch", version)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedCalls := [][]string{
		{"/bin/rotator", "--old", "beta", "--new", "stable1", "--config-file", "/path/to/branch/1.35.yaml"},
		{"/bin/rotator", "--old", "stable1", "--new", "stable2", "--config-file", "/path/to/branch/1.34.yaml"},
		{"/bin/rotator", "--old", "stable2", "--new", "stable3", "--config-file", "/path/to/branch/1.33.yaml"},
		{"/bin/rotator", "--old", "stable3", "--new", "stable4", "--config-file", "/path/to/branch/1.32.yaml"},
	}

	if len(mock.calls) != len(expectedCalls) {
		t.Fatalf("expected %d calls, got %d", len(expectedCalls), len(mock.calls))
	}

	for index, expected := range expectedCalls {
		if !slices.Equal(mock.calls[index], expected) {
			t.Errorf("call %d: got %v, want %v", index, mock.calls[index], expected)
		}
	}
}

func TestRotateFilesError(t *testing.T) {
	t.Parallel()

	mock := &mockCommander{calls: nil, err: errMockCommand}
	version := release.Version{Major: 1, Minor: 35}

	err := run.RotateFiles(context.Background(), mock, "/bin/rotator", "/path/to/branch", version)
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, errMockCommand) {
		t.Errorf("expected errMockCommand, got: %v", err)
	}
}

func TestForkNewFile(t *testing.T) {
	t.Parallel()

	mock := &mockCommander{calls: nil, err: nil}
	version := release.Version{Major: 1, Minor: 35}
	branchDir := t.TempDir()
	jobConfigDir := t.TempDir()

	err := run.ForkNewFile(context.Background(), mock, "/bin/forker", branchDir, jobConfigDir, version, "1.24.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.calls))
	}

	call := mock.calls[0]

	if call[0] != "/bin/forker" {
		t.Errorf("binary = %q, want /bin/forker", call[0])
	}

	absJobConfig, _ := filepath.Abs(jobConfigDir)

	absOutput, _ := filepath.Abs(filepath.Join(branchDir, "1.36.yaml"))

	expected := []string{
		"/bin/forker",
		"--job-config", absJobConfig,
		"--output", absOutput,
		"--version", "1.36",
		"--go-version", "1.24.0",
	}

	if !slices.Equal(call, expected) {
		t.Errorf("got %v, want %v", call, expected)
	}
}

func TestForkNewFileError(t *testing.T) {
	t.Parallel()

	mock := &mockCommander{calls: nil, err: errMockCommand}
	version := release.Version{Major: 1, Minor: 35}

	err := run.ForkNewFile(context.Background(), mock, "/bin/forker", t.TempDir(), t.TempDir(), version, "1.24.0")
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, errMockCommand) {
		t.Errorf("expected errMockCommand, got: %v", err)
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
