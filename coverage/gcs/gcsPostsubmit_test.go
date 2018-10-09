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

package gcs

import (
	"context"
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/coverage/artifacts/artsTest"
	"k8s.io/test-infra/coverage/gcs/gcsFakes"
	"k8s.io/test-infra/coverage/logUtil"
	"k8s.io/test-infra/coverage/test"
)

const (
	gcsBucketNameForTest = "knative-prow"
	prowjobNameForTest   = "post-fakeRepoOwner-fakeRepoName-go-coverage"
)

func testPostSubmit() (p *postSubmit) {
	logrus.Infof("testPostSubmit() called")

	p = NewPostSubmit(context.Background(), gcsFakes.NewFakeStorageClient(),
		gcsBucketNameForTest, prowjobNameForTest, ArtifactsDirNameOnGcs, artsTest.LocalInputArtsForTest().ProfileName())
	p.Build = -9
	return
}

func TestGetLatestHealthyBuild(t *testing.T) {
	b := testPostSubmit()
	fmt.Printf("latestbuld='%d'\n", b.Build)
}

func TestPostSubmitProfileReader(t *testing.T) {
	b := testPostSubmit()
	fmt.Printf("latest healthy build='%d'\n", b.Build)
	if b.ProfileReader() == nil {
		t.Fatalf("PostSubmit.ProfileReader() is nil")
	}
}

func TestListing(t *testing.T) {
	p := testPostSubmit()
	fmt.Printf("Find builds: ")
	for _, build := range p.listBuilds() {
		fmt.Printf("%v, ", build)
	}
	fmt.Printf("\n")
}

func TestSearch(t *testing.T) {
	p := testPostSubmit()
	actual := p.searchForLatestHealthyBuild()
	t.Logf("latest healthy build = %d\n", actual)
	expected := 9
	test.AssertEqual(t, expected, actual)
}

func TestDirOfArtifacts(t *testing.T) {
	p := testPostSubmit()
	actual := p.dirOfArtifacts(1984)
	t.Logf("directory of artifacts for build 1984 = %s\n", actual)
	expected := "logs/post-fakeRepoOwner-fakeRepoName-go-coverage/1984/artifacts"
	if expected != actual {
		t.Fatalf("failed. Expected = %s", expected)
	}
}

func TestPathToGoodCoverageProfile(t *testing.T) {
	p := testPostSubmit()
	profilePath := p.pathToGoodCoverageProfile()
	fmt.Printf("path to latest healthy build = %s\n", profilePath)
	if !p.StorageClient.DoesObjectExist(p.Ctx, p.Bucket, profilePath) {
		t.Fatalf("path point to no object: %s", profilePath)
	}
}

func TestSearchForLatestHealthyBuildFailure(t *testing.T) {
	p := testPostSubmit()
	p.Bucket = "do-not-exist"

	logFatalSaved := logUtil.LogFatalf
	logUtil.LogFatalf = logrus.Infof
	if p.searchForLatestHealthyBuild() != -1 {
		t.Fatalf("p.searchForLatestHealthyBuild() != -1\n")
	}
	logUtil.LogFatalf = logFatalSaved
}
