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
	"fmt"
	"os"
	"strconv"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/coverage/artifacts"
	"k8s.io/test-infra/coverage/gcs"
	"k8s.io/test-infra/coverage/githubUtil/githubPR"
	"k8s.io/test-infra/coverage/logUtil"
	"k8s.io/test-infra/coverage/testgrid"
	"k8s.io/test-infra/coverage/workflows"
)

const (
	keyCovProfileFileName    = "key-cov-prof.txt"
	defaultStdoutRedirect    = "stdout.txt"
	defaultCoverageTargetDir = "./pkg/"
	defaultGcsBucket         = "knative-prow"
	defaultPostSubmitJobName = "post-knative-serving-go-coverage"
	defaultCovThreshold      = 80
	defaultLocalPr           = "0"
	defaultLocalBuildStr     = "0"
)

var (
	artifactsDir        = "./artifacts/"
	coverageProfileName = "coverage_profile.txt"
)

func main() {
	fmt.Println("entering code coverage main")

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
	buildStr := os.Getenv("BUILD_NUMBER")
	pr := os.Getenv("PULL_NUMBER")
	pullSha := os.Getenv("PULL_PULL_SHA")
	baseSha := os.Getenv("PULL_BASE_SHA")
	repoOwner := os.Getenv("REPO_OWNER")
	repoName := os.Getenv("REPO_NAME")
	jobType := os.Getenv("JOB_TYPE")
	jobName := os.Getenv("JOB_NAME")

	fmt.Printf("Running coverage for PR %s with PR commit SHA %s and base SHA %s", pr, pullSha, baseSha)

	artifactsDir = *artifactsDirFlag

	localArtifacts := artifacts.NewLocalArtifacts(
		artifactsDir,
		coverageProfileName,
		keyCovProfileFileName,
		defaultStdoutRedirect,
	)

	localArtifacts.ProduceProfileFile(*coverageTargetDir)

	switch jobType {
	case "presubmit", "local-presubmit":
		if jobType == "local-presubmit" {
			if buildStr == "" {
				buildStr = defaultLocalBuildStr
			}
			if pr == "" {
				pr = defaultLocalPr
			}
		}

		build, err := strconv.Atoi(buildStr)
		if err != nil {
			logUtil.LogFatalf("BUILD_NUMBER(%s) cannot be converted to int, err=%v",
				buildStr, err)
		}

		prData := githubPR.New(*githubTokenPath, repoOwner, repoName, pr, *covbotUserName)
		gcsData := &gcs.PresubmitBuild{GcsBuild: gcs.GcsBuild{
			StorageClient: gcs.NewStorageClient(prData.Ctx),
			Bucket:        *gcsBucketName,
			Job:           jobName,
			Build:         build,
			CovThreshold:  *covThresholdFlag,
		},
			PostSubmitJob: *postSubmitJobName,
		}
		presubmit := &gcs.PreSubmit{
			GithubPr:       *prData,
			PresubmitBuild: *gcsData,
		}

		presubmit.Artifacts = *presubmit.MakeGcsArtifacts(*localArtifacts)

		isCoverageLow := workflows.RunPresubmit(presubmit, localArtifacts)

		if isCoverageLow {
			logUtil.LogFatalf("Code coverage is below threshold (%d%%), "+
				"fail presubmit workflow intentionally", *covThresholdFlag)
		}
	case "periodic":
		logrus.Infof("job type is %v, producing testsuite xml...\n", jobType)
		testgrid.ProfileToTestsuiteXML(localArtifacts, *covThresholdFlag)
	}

	fmt.Println("end of code coverage main")
}
