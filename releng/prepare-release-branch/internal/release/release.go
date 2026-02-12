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

// Package release handles version discovery, validation, and stale branch
// cleanup for release branch job configurations.
package release

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	// MaxConfigCount is the maximum number of release branch config files.
	MaxConfigCount = 5

	// minConfigCount is the minimum number of release branch config files.
	minConfigCount = 3

	// staleVersionOffset is how many versions back to delete when pruning.
	staleVersionOffset = 3

	// versionParts is the number of parts in a major.minor version string.
	versionParts = 2
)

var (
	// ErrIncorrectConfigCount is returned when the config file count is out of range.
	ErrIncorrectConfigCount = errors.New("incorrect release config count")

	// ErrNonSequential is returned when version files have gaps.
	ErrNonSequential = errors.New("branches are not sequential")

	// ErrUnexpectedFilename is returned when a config filename cannot be parsed.
	ErrUnexpectedFilename = errors.New("unexpected filename format")
)

// Version represents a Kubernetes release version (major.minor).
type Version struct {
	Major int
	Minor int
}

// String returns the version as "major.minor".
func (v Version) String() string {
	return fmt.Sprintf("%d.%d", v.Major, v.Minor)
}

// Filename returns the version as "major.minor.yaml".
func (v Version) Filename() string {
	return v.String() + ".yaml"
}

// GetConfigFiles returns all .yaml files in the given directory.
func GetConfigFiles(dir string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, fmt.Errorf("globbing config files: %w", err)
	}

	return files, nil
}

// parseVersionFile extracts a Version from a config filename like "1.35.yaml".
func parseVersionFile(file string) (Version, error) {
	base := strings.TrimSuffix(filepath.Base(file), ".yaml")

	parts := strings.SplitN(base, ".", versionParts)
	if len(parts) != versionParts {
		return Version{}, fmt.Errorf("%w: %s", ErrUnexpectedFilename, file)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return Version{}, fmt.Errorf("parsing major version from %s: %w", file, err)
	}

	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return Version{}, fmt.Errorf("parsing minor version from %s: %w", file, err)
	}

	return Version{Major: major, Minor: minor}, nil
}

// CheckVersion parses version filenames, validates the count and sequential
// ordering, and returns the highest version found.
func CheckVersion(dir string) (Version, error) {
	files, err := GetConfigFiles(dir)
	if err != nil {
		return Version{}, err
	}

	if len(files) < minConfigCount || len(files) > MaxConfigCount {
		return Version{}, fmt.Errorf(
			"%w: expected between %d and %d yaml files in %s, but found %d",
			ErrIncorrectConfigCount, minConfigCount, MaxConfigCount, dir, len(files),
		)
	}

	versions := make([]Version, 0, len(files))

	for _, file := range files {
		ver, err := parseVersionFile(file)
		if err != nil {
			return Version{}, err
		}

		versions = append(versions, ver)
	}

	sort.Slice(versions, func(i, j int) bool {
		if versions[i].Major != versions[j].Major {
			return versions[i].Major < versions[j].Major
		}

		return versions[i].Minor < versions[j].Minor
	})

	lowest := versions[0]

	for idx, ver := range versions {
		if ver.Minor != lowest.Minor+idx {
			return Version{}, fmt.Errorf("%w: gap at %s", ErrNonSequential, ver.String())
		}
	}

	return versions[len(versions)-1], nil
}

// DeleteStaleBranch removes the config file for the version that is
// staleVersionOffset versions behind the current version.
func DeleteStaleBranch(dir string, ver Version) error {
	stale := Version{Major: ver.Major, Minor: ver.Minor - staleVersionOffset}
	stalePath := filepath.Join(dir, stale.Filename())

	if err := os.Remove(stalePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("removing stale config %s: %w", stalePath, err)
	}

	return nil
}
