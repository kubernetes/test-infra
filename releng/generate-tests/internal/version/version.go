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

// Package version handles Kubernetes version discovery from the
// release-branch-jobs directory.
package version

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
)

// NumTiers is the number of version tiers (beta, stable1-4).
const NumTiers = 5

// ErrNoVersionFiles is returned when no version files are found.
var ErrNoVersionFiles = errors.New("no version files found")

// Version represents a Kubernetes version (major.minor).
type Version struct {
	Major int
	Minor int
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d", v.Major, v.Minor)
}

// Tier maps a version marker name to a version and interval.
type Tier struct {
	Marker   string // e.g., "beta", "stable1"
	Version  Version
	Interval string // e.g., "1h", "2h"
}

// TierDef defines a tier's marker name and polling interval.
type TierDef struct {
	Marker   string
	Interval string
}

// TierDefs returns the tier definitions (marker + interval) for each tier.
func TierDefs() []TierDef {
	return []TierDef{
		{"beta", "1h"},
		{"stable1", "2h"},
		{"stable2", "6h"},
		{"stable3", "24h"},
		{"stable4", "24h"},
	}
}

// DiscoverVersions scans the release-branch-jobs directory for version files
// and returns the version tiers (beta, stable1, ..., stable4).
func DiscoverVersions(dir string) ([]Tier, error) {
	versionFileRegex := regexp.MustCompile(`^(\d+)\.(\d+)\.yaml$`)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading release-branch-jobs directory: %w", err)
	}

	var versions []Version

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		matches := versionFileRegex.FindStringSubmatch(entry.Name())
		if matches == nil {
			continue
		}

		major, _ := strconv.Atoi(matches[1])
		minor, _ := strconv.Atoi(matches[2])
		versions = append(versions, Version{Major: major, Minor: minor})
	}

	if len(versions) == 0 {
		return nil, fmt.Errorf("%w in %s", ErrNoVersionFiles, dir)
	}

	// Sort descending by version.
	sort.Slice(versions, func(i, j int) bool {
		if versions[i].Major != versions[j].Major {
			return versions[i].Major > versions[j].Major
		}

		return versions[i].Minor > versions[j].Minor
	})

	maxVersion := versions[0]
	defs := TierDefs()

	tiers := make([]Tier, NumTiers)
	for i, def := range defs {
		tiers[i] = Tier{
			Marker:   def.Marker,
			Version:  Version{Major: maxVersion.Major, Minor: maxVersion.Minor - i},
			Interval: def.Interval,
		}
	}

	return tiers, nil
}

// Args returns the version extract args for a given version.
func Args(v Version) []string {
	return []string{
		"--extract=ci/latest-" + v.String(),
	}
}
