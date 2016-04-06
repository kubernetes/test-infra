/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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

package fake

import "k8s.io/contrib/mungegithub/mungers/e2e"

// FakeE2ETester always reports builds as stable.
type FakeE2ETester struct {
	JobNames           []string
	WeakStableJobNames []string
}

// GCSBasedStable is always true.
func (e *FakeE2ETester) GCSBasedStable() bool { return true }

// GCSWeakStable is always true.
func (e *FakeE2ETester) GCSWeakStable() bool { return true }

// Stable is always true.
func (e *FakeE2ETester) Stable() bool { return true }

// GetBuildStatus reports "Stable" and a latest build of "1" for each build.
func (e *FakeE2ETester) GetBuildStatus() map[string]e2e.BuildInfo {
	out := map[string]e2e.BuildInfo{}
	for _, name := range e.JobNames {
		out[name] = e2e.BuildInfo{"Stable", "1"}
	}
	for _, name := range e.WeakStableJobNames {
		out[name] = e2e.BuildInfo{"Stable", "1"}
	}
	return out
}
