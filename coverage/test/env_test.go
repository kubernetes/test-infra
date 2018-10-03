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

package test

import (
	"os"
	"path"
	"testing"

	"go/build"
)

func Test(t *testing.T) {
	pathRelToProj := "middle/of/nowhere"
	var projAbsolutePath string
	testSrcDir := os.Getenv("TEST_SRCDIR")
	if testSrcDir != "" {
		projAbsolutePath = path.Join(testSrcDir, projectPathLessTestSrc)
	} else {
		projAbsolutePath = path.Join(build.Default.GOPATH, projectPathLessGoPath)
	}

	expected := path.Join(projAbsolutePath, pathRelToProj)
	actual := absPath(pathRelToProj)
	AssertEqual(t, expected, actual)
}
