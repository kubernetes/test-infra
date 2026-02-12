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

// Package run handles subprocess execution and external HTTP requests
// for the release branch preparation workflow.
package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"

	"k8s.io/test-infra/releng/prepare-release-branch/internal/release"
)

// GoVersionURL is the URL to fetch the current Go version from kubernetes/kubernetes master.
const GoVersionURL = "https://raw.githubusercontent.com/kubernetes/kubernetes/master/.go-version"

// ErrHTTPStatus is returned when the HTTP response has an unexpected status code.
var ErrHTTPStatus = errors.New("unexpected HTTP status")

// Suffixes returns the ordered tier suffixes used for release branch rotation.
// Each release is rotated through these tiers: beta → stable1 → … → stable4.
func Suffixes() []string {
	return []string{"beta", "stable1", "stable2", "stable3", "stable4"}
}

// Commander executes external commands.
type Commander interface {
	Run(ctx context.Context, name string, args ...string) error
}

// ExecCommander runs commands via os/exec with connected stdout and stderr.
type ExecCommander struct {
	Stdout io.Writer
	Stderr io.Writer
}

// Run executes the named command with the given arguments.
func (c *ExecCommander) Run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = c.Stdout
	cmd.Stderr = c.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(name), err)
	}

	return nil
}

// RotateFiles calls config-rotator for each tier, from the current version
// backwards. Each version's config is rotated to the next stability tier.
func RotateFiles(
	ctx context.Context, commander Commander, rotatorBin, branchDir string, version release.Version,
) error {
	suffixes := Suffixes()

	for index := range len(suffixes) - 1 {
		target := release.Version{Major: version.Major, Minor: version.Minor - index}

		if err := commander.Run(ctx, rotatorBin,
			"--old", suffixes[index],
			"--new", suffixes[index+1],
			"--config-file", filepath.Join(branchDir, target.Filename()),
		); err != nil {
			return fmt.Errorf("rotating %s (%s to %s): %w",
				target.Filename(), suffixes[index], suffixes[index+1], err)
		}
	}

	return nil
}

// ForkNewFile calls config-forker to create a config for the next version.
func ForkNewFile(
	ctx context.Context,
	commander Commander,
	forkerBin, branchDir, jobConfigDir string,
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

	if err := commander.Run(ctx, forkerBin,
		"--job-config", absJobConfig,
		"--output", absOutput,
		"--version", next.String(),
		"--go-version", goVersion,
	); err != nil {
		return fmt.Errorf("forking %s: %w", next.Filename(), err)
	}

	return nil
}

// RegenerateFiles calls generate-tests for the given branch directory.
func RegenerateFiles(ctx context.Context, commander Commander, generateTestsBin, branchDir string) error {
	absBranchDir, err := filepath.Abs(branchDir)
	if err != nil {
		return fmt.Errorf("resolving branch dir path: %w", err)
	}

	if err := commander.Run(ctx, generateTestsBin,
		"--release-branch-dir", absBranchDir,
	); err != nil {
		return fmt.Errorf("regenerating files: %w", err)
	}

	return nil
}

// FetchGoVersion fetches the Go version string from the given URL.
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

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: %d", ErrHTTPStatus, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading Go version response: %w", err)
	}

	return strings.TrimSpace(string(body)), nil
}
