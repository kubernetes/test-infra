/*
Copyright 2021 The Kubernetes Authors.

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

import (
	"fmt"
	"strings"

	imagebumper "k8s.io/test-infra/experiment/image-bumper/bumper"
)

// Extract image tag from image name
func tagFromName(name string) string {
	parts := strings.Split(name, ":")
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

// Extract prow component name from image
func componentFromName(name string) string {
	s := strings.SplitN(strings.Split(name, ":")[0], "/", 3)
	return s[len(s)-1]
}

func formatTagDate(d string) string {
	if len(d) != 8 {
		return d
	}
	// &#x2011; = U+2011 NON-BREAKING HYPHEN, to prevent line wraps.
	return fmt.Sprintf("%s&#x2011;%s&#x2011;%s", d[0:4], d[4:6], d[6:8])
}

// commitToRef converts git describe part of a tag to a ref (commit or tag).
//
// v0.0.30-14-gdeadbeef => deadbeef
// v0.0.30 => v0.0.30
// deadbeef => deadbeef
func commitToRef(commit string) string {
	tag, _, commit := imagebumper.DeconstructCommit(commit)
	if commit != "" {
		return commit
	}
	return tag
}

// Format variant for PR summary
func formatVariant(variant string) string {
	if variant == "" {
		return ""
	}
	return fmt.Sprintf("(%s)", strings.TrimPrefix(variant, "-"))
}

// Check whether the path is under the given path
func isUnderPath(name string, paths []string) bool {
	for _, p := range paths {
		if p != "" && strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// isBumpedPrefix takes a prefix and a map of new tags resulted from bumping
// : the images using those tags and itterates over the map to find if the
// prefix is found. If it is, this means it has been bumped.
func isBumpedPrefix(prefix prefix, versions map[string][]string) (string, bool) {
	for tag, imageList := range versions {
		for _, image := range imageList {
			if strings.HasPrefix(image, prefix.Prefix) {
				return tag, true
			}
		}
	}
	return "", false
}
