/*
Copyright 2018 The Kubernetes Authors.

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

package cov

import (
	"errors"
	"fmt"
	"golang.org/x/tools/cover"
	"sort"
)

// MergeProfiles merges two coverage profiles.
// The profiles are expected to be similar - that is, from multiple invocations of a
// single binary, or multiple binaries using the same codebase.
// In particular, any source files with the same path must have had identical content
// when building the binaries.
// MergeProfiles expects its arguments to be sorted: Profiles in alphabetical order,
// and lines in files in the order those lines appear. These are standard constraints for
// Go coverage profiles. The resulting profile will also obey these constraints.
func MergeProfiles(a []*cover.Profile, b []*cover.Profile) ([]*cover.Profile, error) {
	var result []*cover.Profile
	files := make(map[string]*cover.Profile, len(a))
	for _, profile := range a {
		np := deepCopyProfile(*profile)
		result = append(result, &np)
		files[np.FileName] = &np
	}

	needsSort := false
	// Now merge b into the result
	for _, profile := range b {
		dest, ok := files[profile.FileName]
		if ok {
			if err := ensureProfilesMatch(profile, dest); err != nil {
				return nil, fmt.Errorf("error merging %s: %v", profile.FileName, err)
			}
			for i, block := range profile.Blocks {
				db := &dest.Blocks[i]
				db.Count += block.Count
			}
		} else {
			// If we get some file we haven't seen before, we just append it.
			// We need to sort this later to ensure the resulting profile is still correctly sorted.
			np := deepCopyProfile(*profile)
			files[np.FileName] = &np
			result = append(result, &np)
			needsSort = true
		}
	}
	if needsSort {
		sort.Slice(result, func(i, j int) bool { return result[i].FileName < result[j].FileName })
	}
	return result, nil
}

// MergeMultipleProfiles merges more than two profiles together.
// MergeMultipleProfiles is equivalent to calling MergeProfiles on pairs of profiles
// until only one profile remains.
func MergeMultipleProfiles(profiles [][]*cover.Profile) ([]*cover.Profile, error) {
	if len(profiles) < 1 {
		return nil, errors.New("can't merge zero profiles")
	}
	result := profiles[0]
	for _, profile := range profiles[1:] {
		var err error
		if result, err = MergeProfiles(result, profile); err != nil {
			return nil, err
		}
	}
	return result, nil
}
