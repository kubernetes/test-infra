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
	"fmt"
	"golang.org/x/tools/cover"
)

// DiffProfiles returns the difference between two sets of coverage profiles.
// The profiles are expected to be from a single execution of the same binary
// (or multiple binaries, if using a merged coverage profile)
func DiffProfiles(before []*cover.Profile, after []*cover.Profile) ([]*cover.Profile, error) {
	var diff []*cover.Profile
	if len(before) != len(after) {
		return nil, fmt.Errorf("before and after have different numbers of profiles (%d vs. %d)", len(before), len(after))
	}
	for i, beforeProfile := range before {
		afterProfile := after[i]
		if err := ensureProfilesMatch(beforeProfile, afterProfile); err != nil {
			return nil, fmt.Errorf("error on profile #%d: %v", i, err)
		}
		diffProfile := cover.Profile{FileName: beforeProfile.FileName, Mode: beforeProfile.Mode}
		for j, beforeBlock := range beforeProfile.Blocks {
			afterBlock := afterProfile.Blocks[j]
			diffBlock := cover.ProfileBlock{
				StartLine: beforeBlock.StartLine,
				StartCol:  beforeBlock.StartCol,
				EndLine:   beforeBlock.EndLine,
				EndCol:    beforeBlock.EndCol,
				NumStmt:   beforeBlock.NumStmt,
				Count:     afterBlock.Count - beforeBlock.Count,
			}
			diffProfile.Blocks = append(diffProfile.Blocks, diffBlock)
		}
		diff = append(diff, &diffProfile)
	}
	return diff, nil
}
