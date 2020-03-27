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

// Package junit provides a junit viewer for Spyglass
package junit

import (
	"testing"

	"k8s.io/test-infra/prow/spyglass/lenses"
)

const (
	fakeResourceDir = "resources-for-tests"
)

func TestGetJvd(t *testing.T) {
	fakeLog := &lenses.FakeArtifact{
		Path: "log.txt",
		Content: []byte(`
		<testsuites>
			<testsuite>
				<testcase classname="v1beta1" name="TestUpdateConfigurationMetadata">
					<failure message="Failed" type=""> It failed </failure>
				</testcase>
			</testsuite>
			<testsuite>
				<testcase classname="v1beta1" name="TestUpdateConfigurationMetadata"></testcase>
			</testsuite>
		</testsuites>
		`),
		SizeLimit: 500e6,
	}
	// expJvd := JVD{
	// 	NumTests: 1,
	// 	Passed: nil,
	// 	Failed: nil,
	// 	Skipped: nil,
	// 	Flaky:[]junit.TestResult{

	// 	}
	// }
	l := Lens{}
	out := l.getJvd([]lenses.Artifact{fakeLog})
	t.Fatal(out)
}
