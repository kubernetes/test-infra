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

package workflows

import (
	"context"
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/coverage/artifacts/artsTest"
	"k8s.io/test-infra/coverage/gcs"
	"k8s.io/test-infra/coverage/gcs/gcsFakes"
	"k8s.io/test-infra/coverage/githubUtil/githubFakes"
	"k8s.io/test-infra/coverage/githubUtil/githubPR"
	"k8s.io/test-infra/coverage/io"
	"k8s.io/test-infra/coverage/test"
)

const (
	testPresubmitBuild      = 787
	gcsBucketNameForTest    = "knative-prow"
	presubmitJobNameForTest = "pull-fakeRepoOwner-fakeRepoName-go-coverage"
	postsubmitJobName       = "post-fakeRepoOwner-fakeRepoName-go-coverage"
)

func repoDataForTest() *githubPR.GithubPr {
	ctx := context.Background()
	logrus.Infof("creating fake repo data \n")

	return &githubPR.GithubPr{
		RepoOwner:     "fakeRepoOwner",
		RepoName:      "fakeRepoName",
		Pr:            7,
		RobotUserName: "fakeCovbot",
		GithubClient:  githubFakes.FakeGithubClient(),
		Ctx:           ctx,
	}
}

func gcsArtifactsForTest() *gcs.GcsArtifacts {
	return &gcs.GcsArtifacts{
		Ctx:       context.Background(),
		Bucket:    "fakeBucket",
		Client:    gcsFakes.NewFakeStorageClient(),
		Artifacts: artsTest.LocalArtsForTest("gcsArts-").Artifacts,
	}
}

func preSubmitForTest() (data *gcs.PreSubmit) {
	repoData := repoDataForTest()
	build := gcs.GcsBuild{
		StorageClient: gcsFakes.NewFakeStorageClient(),
		Bucket:        gcsBucketNameForTest,
		Job:           presubmitJobNameForTest,
		Build:         testPresubmitBuild,
	}
	pbuild := gcs.PresubmitBuild{
		GcsBuild:      build,
		Artifacts:     *gcsArtifactsForTest(),
		PostSubmitJob: postsubmitJobName,
	}
	data = &gcs.PreSubmit{
		GithubPr:       *repoData,
		PresubmitBuild: pbuild,
	}
	logrus.Info("finished preSubmitForTest()")
	return
}

func TestRunPresubmit(t *testing.T) {
	logrus.Info("Starting TestRunPresubmit")
	arts := artsTest.LocalArtsForTest("TestRunPresubmit")
	arts.ProduceProfileFile("../" + test.CovTargetRelPath)
	p := preSubmitForTest()
	RunPresubmit(p, arts)
	if !io.FileOrDirExists(arts.LineCovFilePath()) {
		t.Fatalf("No line cov file found in %s\n", arts.LineCovFilePath())
	}
}

// tests the construction of gcs url from PreSubmit
func TestK8sGcsAddress(t *testing.T) {
	data := preSubmitForTest()
	data.Build = 1286
	actual := data.UrlGcsLineCovLinkWithMarker(3)

	expected := fmt.Sprintf("https://storage.cloud.google.com/%s/pr-logs/pull/"+
		"%s_%s/%s/%s/%s/artifacts/line-cov.html#file3",
		gcsBucketNameForTest, data.RepoOwner, data.RepoName, data.PrStr(), presubmitJobNameForTest, data.BuildStr())
	if actual != expected {
		t.Fatal(test.StrFailure("", expected, actual))
	}
	fmt.Printf("line cov link=%s", actual)
}
