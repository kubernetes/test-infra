/*
Copyright 2019 The Kubernetes Authors.

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

package versionutil

import (
	"fmt"
	"sort"

	versioner "k8s.io/test-infra/prow/custom-reporter/guest-test-infra/version"
)

// tags is a list of tag name; returns the latest version tag
func GetLatestVersionTag(tags []string) (versioner.NonSemanticVer, error) {
	var validTags []versioner.NonSemanticVer
	for _, tag := range tags {
		v, err := versioner.NewNonSemanticVer(tag)
		if err == nil {
			validTags = append(validTags, *v)
		}
	}
	if len(validTags) == 0 {
		return versioner.NonSemanticVer{}, fmt.Errorf("No valid version tags found")
	}
	sort.Sort(versioner.VersionSorter(validTags))
	// return the last element since the sorter sorts in increasing order
	return validTags[len(validTags)-1], nil
}
