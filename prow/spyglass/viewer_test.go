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

package spyglass

import (
	"testing"
)

func TestBuildLogView(t *testing.T) {
	buildLogArtifact := NewGCSArtifact(fakeGCSBucket.Object(buildLogName), fakeGCSJobSource.JobPath())
	buildLogViewer := BuildLogViewer{
		title: "Build Log",
	}
	testCases := []struct {
		name         string
		artifacts    []Artifact
		expectedView string
	}{
		{
			name:      "Basic Build Log View",
			artifacts: []Artifact{buildLogArtifact},
			expectedView: `
<div>
	<div>
		Oh wow
logs
this is
crazy
	</div>
</div>`,
		},
	}

	for _, tc := range testCases {
		actualView := buildLogViewer.View(tc.artifacts)
		if actualView != tc.expectedView {
			t.Errorf("Test %s failed. Expected:\n%s\nActual:\n%s", tc.name, tc.expectedView, actualView)
		}
	}
}
