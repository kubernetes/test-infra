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
	"os"
)

const (
	keyCovProfileFileName    = "key-cov-prof.txt"
	defaultStdoutRedirect    = "stdout.txt"
	defaultCoverageTargetDir = "./pkg/"
	defaultGcsBucket         = "knative-prow"
	defaultPostSubmitJobName = "post-knative-serving-go-coverage"
	defaultCovThreshold      = 80
)

var (
	artifactsDir        = "./artifacts/"
	coverageProfileName = "coverage_profile.txt"
)

func main() {
	logrus.Infoln("Entering code coverage main")

	gcsBucketName := flag.String("postsubmit-gcs-bucket", defaultGcsBucket, "gcs bucket name")
	postSubmitJobName := flag.String("postsubmit-job-name", defaultPostSubmitJobName, "name of the prow job")
	artifactsDirFlag := flag.String("artifacts", artifactsDir, "directory for artifacts")
	coverageTargetDir := flag.String("cov-target", defaultCoverageTargetDir, "target directory for test coverage")
	flag.StringVar(&coverageProfileName, "profile-name", coverageProfileName, "file name for coverage profile")
	githubTokenPath := flag.String("github-token", "", "path to token to access github repo")
	covThresholdFlag := flag.Int("cov-threshold-percentage", defaultCovThreshold, "token to access github repo")
	covbotUserName := flag.String("covbot-username", "covbot", "github user name for coverage robot")
	flag.Parse()

	logrus.Infof("container flag list: postsubmit-gcs-bucket=%s; postSubmitJobName=%s; "+
		"artifacts=%s; cov-target=%s; profile-name=%s; github-token=%s; "+
		"cov-threshold-percentage=%d; covbot-username=%s;",
		*gcsBucketName, *postSubmitJobName, *artifactsDirFlag, *coverageTargetDir, coverageProfileName,
		*githubTokenPath, *covThresholdFlag, *covbotUserName)

	logrus.Info("Getting env values")
	pr := os.Getenv("PULL_NUMBER")
	pullSha := os.Getenv("PULL_PULL_SHA")
	baseSha := os.Getenv("PULL_BASE_SHA")

	logrus.Printf("Running coverage for PR %s with PR commit SHA %s and base SHA %s", pr, pullSha, baseSha)

	artifactsDir = *artifactsDirFlag

	localArtifacts := artifacts.NewLocalArtifacts(
		artifactsDir,
		coverageProfileName,
		keyCovProfileFileName,
		defaultStdoutRedirect,
	)

	localArtifacts.ProduceProfileFile(*coverageTargetDir)

	logrus.Infoln("Finished running code coverage main")
}
