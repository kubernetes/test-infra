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
	"golang.org/x/tools/cover"
)

// AggregateProfiles takes multiple coverage profiles and produces a new
// coverage profile that counts the number of profiles that hit a block at least
// once.
func AggregateProfiles(profiles [][]*cover.Profile) ([]*cover.Profile, error) {
	setProfiles := make([][]*cover.Profile, 0, len(profiles))
	for _, p := range profiles {
		c := countToBoolean(p)
		setProfiles = append(setProfiles, c)
	}
	aggregateProfiles, err := MergeMultipleProfiles(setProfiles)
	if err != nil {
		return nil, err
	}
	return aggregateProfiles, nil
}

// countToBoolean converts a profile containing hit counts to instead contain
// only 1s or 0s.
func countToBoolean(profile []*cover.Profile) []*cover.Profile {
	setProfile := make([]*cover.Profile, 0, len(profile))
	for _, p := range profile {
		pc := deepCopyProfile(*p)
		for i := range pc.Blocks {
			if pc.Blocks[i].Count > 0 {
				pc.Blocks[i].Count = 1
			}
		}
		setProfile = append(setProfile, &pc)
	}
	return setProfile
}
