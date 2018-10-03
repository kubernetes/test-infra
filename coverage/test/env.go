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

	"github.com/sirupsen/logrus"
)

const projectPathLessGoPath = "src/k8s.io/test-infra/coverage"
const projectPathLessTestSrc = "__main__/coverage"

//ProjDir returns the absolute project directory in os
func ProjDir() string {
	testSrcDir := os.Getenv("TEST_SRCDIR")
	if testSrcDir != "" {
		logrus.Infof("Env var TEST_SRCDIR=%s", testSrcDir)
		return path.Join(testSrcDir, projectPathLessTestSrc)
	}

	gopath := os.Getenv("GOPATH")
	logrus.Infof("Env var GOPATH=%s", gopath)
	return path.Join(gopath, projectPathLessGoPath)
}

func absPath(pathRelToProj string) string {
	return path.Join(ProjDir(), pathRelToProj)
}
