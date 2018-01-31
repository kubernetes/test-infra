/*
Copyright 2017 The Kubernetes Authors.

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

package main

import "regexp"
import "strings"

var /* const */ versionRegexp = regexp.MustCompile("^v[0-9].*")

// RemoveDisambiguationInfixes removes (from pod/container names) version strings and hashes inserted replication controllers and the like.
func RemoveDisambiguationInfixes(podAndContainer string) string {
	split := strings.SplitN(podAndContainer, "/", 2)
	if len(split) < 2 {
		return podAndContainer
	}
	pod, container := split[0], split[1]
	pieces := strings.Split(pod, "-")
	var last string
	for i, piece := range pieces {
		if looksLikeHash(piece) || versionRegexp.MatchString(piece) {
			break
		}
		last = strings.Join(pieces[:i+1], "-")
	}
	return strings.Join([]string{last, container}, "/")
}

// looksLikeHash returns true if piece seems to be one of those pseudo-random disambiguation strings
func looksLikeHash(piece string) bool {
	return len(piece) >= 4 && !strings.ContainsAny(piece, "eyuioa")
}
