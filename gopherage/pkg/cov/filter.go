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
	"regexp"
	"strings"
)

// FilterProfilePaths produces a new profile that removes either everything matching or everything
// not matching the provided paths, depending on the value of include.
// Paths are interpreted as regular expressions.
// If include is true, paths is treated as a whitelist; otherwise it is treated as a blacklist.
func FilterProfilePaths(profile []*cover.Profile, paths []string, include bool) ([]*cover.Profile, error) {
	parenPaths := make([]string, len(paths))
	for i, path := range paths {
		parenPaths[i] = "(" + path + ")"
	}
	joined := strings.Join(parenPaths, "|")
	re, err := regexp.Compile(joined)
	if err != nil {
		return nil, err
	}
	result := make([]*cover.Profile, 0, len(profile))
	for _, p := range profile {
		if re.MatchString(p.FileName) == include {
			result = append(result, p)
		}
	}
	return result, nil
}
