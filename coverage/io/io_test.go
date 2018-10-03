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

package io

import (
	"io/ioutil"
	"path"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/coverage/test"
)

func TestWriteToArtifacts(t *testing.T) {
	s := "content to be written on disk"
	artsDir := test.NewArtsDir("TestWriteToArtifacts")
	Write(&s, artsDir, "testWriteToArt.txt")
	content, err := ioutil.ReadFile(path.Join(artsDir, "testWriteToArt.txt"))
	if err != nil {
		logrus.Fatalf("Cannot read file, err = %v", err)
	}

	test.AssertEqual(t, s, string(content))

	test.DeleteDir(artsDir)
}
