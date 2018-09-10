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

package line

import (
	"testing"

	"k8s.io/test-infra/coverage/artifacts"
	"k8s.io/test-infra/coverage/artifacts/artsTest"
	"k8s.io/test-infra/coverage/test"
	"os"
)

const (
	profileName = "cov-profile.txt"
	stdoutName  = "stdout.txt"
)

func LocalArtsForTest_KeyfileNotExist(dirPrefix string) *artifacts.LocalArtifacts {
	return &artifacts.LocalArtifacts{Artifacts: *artifacts.New(
		test.NewArtsDir(dirPrefix),
		profileName,
		"key-cov-profile-dne.txt",
		stdoutName,
	)}
}

func TestCreateLineCovFile(t *testing.T) {
	if os.Getenv("GOPATH") != "" {
		arts := artsTest.LocalArtsForTest("TestCreateLineCovFile")
		test.LinkInputArts(arts.Directory(), "key-cov-profile.txt")

		err := CreateLineCovFile(arts)
		if err != nil {
			t.Fatalf("CreateLineCovFile(arts=%v) failed, err=%v", arts, err)
		}
	}
}

func TestCreateLineCovFileFailure(t *testing.T) {
	arts := LocalArtsForTest_KeyfileNotExist("TestCreateLineCovFileFailure")
	if CreateLineCovFile(arts) == nil {
		t.Fatalf("CreateLineCovFile(arts=%v) should fail, but not", arts)
	}
}
