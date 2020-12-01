/*
Copyright 2020 The Kubernetes Authors.

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

// version holds variables that identify a Prow binary's name and version
package version

import (
	"fmt"
	"regexp"
	"time"
)

var (
	// Name is the colloquial identifier for the compiled component
	Name = "unset"
	// Version is a concatenation of the commit SHA and date for the build
	Version = "0"
	// reVersion is a regex expression for extracting build time.
	// Version derived from "v${build_date}-${git_commit}" as in /hack/print-workspace-status.sh
	reVersion = regexp.MustCompile(`v(\d+)-.*`)
)

// UserAgent exposes the component's name and version for user-agent header
func UserAgent() string {
	return Name + "/" + Version
}

// UserAgentFor exposes the component's name and version for user-agent header
// while embedding the additional identifier
func UserAgentWithIdentifier(identifier string) string {
	return Name + "." + identifier + "/" + Version
}

// VersionTimestamp returns the timestamp of date derived from version
func VersionTimestamp() (int64, error) {
	var ver int64
	m := reVersion.FindStringSubmatch(Version)
	if len(m) < 2 {
		return ver, fmt.Errorf("version expected to be in form 'v${build_date}-${git_commit}': %q", Version)
	}
	t, err := time.Parse("20060102", m[1])
	if err != nil {
		return ver, err
	}
	return t.Unix(), nil
}
