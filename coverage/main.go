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

package main

import (
	"flag"
	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/coverage/artifacts"
)

const (
	keyCovProfileFileName    = "key-cov-prof.txt"
	defaultStdoutRedirect    = "stdout.txt"
	defaultCoverageTargetDir = "."
)

var (
	artifactsDir        = "./artifacts/"
	coverageProfileName = "coverage_profile.txt"
)

func main() {
	artifactsDirFlag := flag.String("artifacts", artifactsDir, "local directory to store and retrieve artifacts")
	coverageTargetDir := flag.String("cov-target", defaultCoverageTargetDir, "target directory to run test coverage against")
	flag.StringVar(&coverageProfileName, "profile-name", coverageProfileName, "file name for coverage profile")
	flag.Parse()

	artifactsDir = *artifactsDirFlag

	localArtifacts := artifacts.NewLocalArtifacts(
		artifactsDir,
		coverageProfileName,
		keyCovProfileFileName,
		defaultStdoutRedirect,
	)

	err := localArtifacts.ProduceProfileFile(*coverageTargetDir)
	if err != nil {
		logrus.Fatalf("Failed producing profile file: %v", err)
	}

	logrus.Infoln("Finished running code coverage main")
}
