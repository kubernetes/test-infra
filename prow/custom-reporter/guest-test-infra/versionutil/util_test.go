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
	"strings"
	"testing"
)

func TestGetLatestVersionTag(t *testing.T) {
	testCase := []struct {
		desc     string
		tags     []string
		expected string
	}{
		{"only one build on latest day", []string{"20190812.00", "20190813.00", "20190812.01"}, "20190813.00"},
		{"multiple builds on latest day", []string{"20190812.00", "20190810.00", "20190812.01"}, "20190812.01"},
	}

	for _, tc := range testCase {
		latest, _ := GetLatestVersionTag(tc.tags)
		if strings.Compare(latest.String(), tc.expected) != 0 {
			t.Errorf("Unexpected output! expected(%s), got(%s)", tc.expected, latest)
		}
	}

}
