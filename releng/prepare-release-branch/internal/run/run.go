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

// Package run handles job config rotation, forking, and external HTTP requests
// for the release branch preparation workflow.
package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	forker "k8s.io/test-infra/releng/config-forker/pkg"
	rotator "k8s.io/test-infra/releng/config-rotator/pkg"
	"k8s.io/test-infra/releng/prepare-release-branch/internal/release"
)

// goVersionURLBase is the base URL template for fetching the .go-version file
// from a kubernetes/kubernetes release branch.
const goVersionURLBase = "https://raw.githubusercontent.com/kubernetes/kubernetes/release-%s/.go-version"

var (
	// ErrHTTPStatus is returned when the HTTP response has an unexpected status code.
	ErrHTTPStatus = errors.New("unexpected HTTP status")

	// ErrBranchNotFound is returned when the release branch does not exist yet.
	ErrBranchNotFound = errors.New("release branch not found")
)

// Suffixes returns the ordered tier suffixes used for release branch rotation.
// Each release is rotated through these tiers: beta → stable1 → … → stable4.
func Suffixes() []string {
	return []string{"beta", "stable1", "stable2", "stable3", "stable4"}
}

// RotateFiles calls config-rotator for each tier, from the current version
// backwards. Each version's config is rotated to the next stability tier.
func RotateFiles(branchDir string, version release.Version) error {
	suffixes := Suffixes()

	for index := range len(suffixes) - 1 {
		target := release.Version{Major: version.Major, Minor: version.Minor - index}

		if err := rotator.Run(rotator.Options{
			ConfigFile: filepath.Join(branchDir, target.Filename()),
			OldVersion: suffixes[index],
			NewVersion: suffixes[index+1],
		}); err != nil {
			return fmt.Errorf("rotating %s (%s to %s): %w",
				target.Filename(), suffixes[index], suffixes[index+1], err)
		}
	}

	return nil
}

// ForkNewFile calls config-forker to create a config for the next version.
func ForkNewFile(
	branchDir, jobConfigDir string,
	version release.Version,
	goVersion string,
) error {
	next := release.Version{Major: version.Major, Minor: version.Minor + 1}

	absJobConfig, err := filepath.Abs(jobConfigDir)
	if err != nil {
		return fmt.Errorf("resolving job config path: %w", err)
	}

	absOutput, err := filepath.Abs(filepath.Join(branchDir, next.Filename()))
	if err != nil {
		return fmt.Errorf("resolving output path: %w", err)
	}

	if err := forker.Run(forker.Options{
		JobConfig:  absJobConfig,
		OutputPath: absOutput,
		Version:    next.String(),
		GoVersion:  goVersion,
	}); err != nil {
		return fmt.Errorf("forking %s: %w", next.Filename(), err)
	}

	return nil
}

// GoVersionURL returns the URL for fetching the .go-version file from
// the given release branch version.
func GoVersionURL(version release.Version) string {
	next := release.Version{Major: version.Major, Minor: version.Minor + 1}

	return fmt.Sprintf(goVersionURLBase, next.String())
}

// FetchGoVersion fetches the Go version string from the given URL.
// Returns ErrBranchNotFound if the release branch does not exist (HTTP 404).
func FetchGoVersion(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching Go version: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("%w: %s", ErrBranchNotFound, url)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: %d", ErrHTTPStatus, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading Go version response: %w", err)
	}

	return strings.TrimSpace(string(body)), nil
}
