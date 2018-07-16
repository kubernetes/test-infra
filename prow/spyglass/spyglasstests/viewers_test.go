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

package spyglasstests

import (
	"testing"

	"k8s.io/test-infra/prow/spyglass"
	"k8s.io/test-infra/prow/spyglass/viewers"
)

func dumpViewHandler(artifacts []viewers.Artifact, raw string) string {
	var view []byte
	for _, a := range artifacts {
		data, err := a.ReadAll()
		if err != nil {
			panic(err)
		}
		view = append(view, data...)
	}
	return string(view)
}

var dumpMetadata = viewers.ViewMetadata{
	Title:    "Dump View",
	Priority: 1,
}

// Tests getting a view from a viewer
func TestView(t *testing.T) {
	junitArtifact := spyglass.NewGCSArtifact(fakeGCSBucket.Object(junitKey), "", junitName)
	buildLogArtifact := spyglass.NewGCSArtifact(fakeGCSBucket.Object(buildLogKey), "", buildLogName)
	err := viewers.RegisterViewer("DumpView", dumpMetadata, dumpViewHandler)
	if err != nil {
		t.Fatal("Failed to register viewer for testing View")
	}
	testCases := []struct {
		name       string
		viewerName string
		artifacts  []viewers.Artifact
		raw        string
		expected   string
		err        error
	}{
		{
			name:       "simple view",
			viewerName: "DumpView",
			artifacts: []viewers.Artifact{
				junitArtifact, buildLogArtifact,
			},
			raw: "",
			expected: `<testsuite tests="1017" failures="1017" time="0.016981535">
<testcase name="BeforeSuite" classname="Kubernetes e2e suite" time="0.006343795">
<failure type="Failure">
test/e2e/e2e.go:137 BeforeSuite on Node 1 failed test/e2e/e2e.go:137
</failure>
</testcase>
</testsuite>Oh wow
logs
this is
crazy`,
			err: nil,
		},
		{
			name:       "fail on unregistered view name",
			viewerName: "MicroverseBattery",
			artifacts:  []viewers.Artifact{},
			raw:        "",
			expected:   "",
			err:        viewers.ErrInvalidViewName,
		},
	}
	for _, tc := range testCases {
		view, err := viewers.View(tc.viewerName, tc.artifacts, tc.raw)
		if tc.err != err {
			t.Errorf("%s expected error %s but got error %s", tc.name, tc.err.Error(), err.Error())
		}
		if view != tc.expected {
			t.Errorf("%s expected view to be %s but got %s", tc.name, tc.expected, view)
		}
	}
	viewers.UnregisterViewer("DumpView")

}

// Test registering a new viewer
func TestRegisterViewer(t *testing.T) {
	testCases := []struct {
		name           string
		viewerName     string
		viewerMetadata viewers.ViewMetadata
		handler        viewers.ViewHandler
		err            error
	}{
		{
			name:           "register dump view",
			viewerName:     "DumpView",
			viewerMetadata: dumpMetadata,
			handler:        dumpViewHandler,
			err:            nil,
		},
	}
	for _, tc := range testCases {
		err := viewers.RegisterViewer(tc.viewerName, tc.viewerMetadata, tc.handler)
		if err != nil {
			if tc.err != nil {
				if err != tc.err {
					t.Errorf("%s expected error %s but got error %s", tc.name, tc.err.Error(), err.Error())
				}
			} else {
				t.Errorf("%s expected no errors but got error %s", tc.name, err.Error())
			}
		} else {
			if tc.err != nil {
				t.Errorf("%s expected error %s but got no errors", tc.name, tc.err.Error())
			}
		}
		viewers.UnregisterViewer(tc.viewerName)
	}
}

// Tests reading last N Lines from files in GCS
func TestGCSReadLastNLines(t *testing.T) {
	buildLogArtifact := spyglass.NewGCSArtifact(fakeGCSBucket.Object(buildLogKey), "", buildLogName)
	//longLogArtifact := spyglass.NewGCSArtifact(fakeGCSBucket.Object(longLogKey), "", longLogName)
	testCases := []struct {
		name     string
		n        int64
		a        *spyglass.GCSArtifact
		expected []string
	}{
		{
			name:     "Read last 2 lines of a 4-line file",
			n:        2,
			a:        buildLogArtifact,
			expected: []string{"this is", "crazy"},
		},
		{
			name:     "Read last 5 lines of a 4-line file",
			n:        5,
			a:        buildLogArtifact,
			expected: []string{"Oh wow", "logs", "this is", "crazy"},
		},
		//{
		//	name:     "Read last 100 lines of a long log file",
		//	n:        100,
		//	a:        longLogArtifact,
		//	expected: longLogLines[len(longLogLines)-100:],
		//},
	}
	for _, tc := range testCases {
		actual, err := viewers.LastNLines(tc.a, tc.n)
		if err != nil {
			t.Fatalf("Test %s failed with error: %s", tc.name, err)
		}
		if len(actual) != len(tc.expected) {
			t.Fatalf("Test %s failed.\nExpected length:\n%d\nActual length:\n%d", tc.name, len(tc.expected), len(actual))
		}
		for ix, line := range tc.expected {
			if line != actual[ix] {
				t.Errorf("Test %s failed.\nExpected:\n%s\nActual:\n%s", tc.name, line, actual[ix])
			}
		}
		for ix, line := range actual {
			if line != tc.expected[ix] {
				t.Errorf("Test %s failed.\nExpected:\n%s\nActual:\n%s", tc.name, tc.expected[ix], line)
			}
		}
	}
}
