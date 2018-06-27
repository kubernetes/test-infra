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

package viewers

import (
	"encoding/json"
	"testing"

	"k8s.io/test-infra/prow/spyglass"
	"k8s.io/test-infra/prow/spyglass/viewers"
)

// TODO how do we test viewers? (uppercase this to test)
func testBuildLogView(t *testing.T) {
	buildLogArtifact := NewGCSArtifact(fakeGCSBucket.Object(buildLogName), spyglass.fakeGCSJobSource.JobPath())
	buildLogViewer := BuildLogViewer{
		ViewTitle: "Build Log",
		ViewName:  "BuildLogViewer",
	}
	testCases := []struct {
		name         string
		artifacts    []viewers.Artifact
		expectedView string
	}{
		{
			name:      "Basic Build Log View",
			artifacts: []viewers.Artifact{buildLogArtifact},
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
		var msg *json.RawMessage
		msg.UnmarshalJSON([]byte(``))
		actualView := buildLogViewer.View(tc.artifacts, msg)
		if actualView != tc.expectedView {
			t.Errorf("Test %s failed. Expected:\n%s\nActual:\n%s", tc.name, tc.expectedView, actualView)
		}
	}
}
