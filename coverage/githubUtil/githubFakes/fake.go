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

package githubFakes

import (
	"context"
	"path"

	"github.com/google/go-github/github"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/coverage/githubUtil/githubClient"
	"k8s.io/test-infra/coverage/test"
)

//FakeGithubClient fakes a GithubClient struct
func FakeGithubClient() *githubClient.GithubClient {
	return githubClient.New(fakeGithubIssues(), fakePullRequests())
}

func testCommitFile(filename string) *github.CommitFile {
	filename = path.Join(test.CovTargetRelPath, filename)
	return &github.CommitFile{
		Filename: &filename,
	}
}

func testCommitFiles() (res []*github.CommitFile) {
	return []*github.CommitFile{
		testCommitFile("onlySrcChange.go"),
		testCommitFile("onlyTestChange_test.go"),
		testCommitFile("common.go"),
		testCommitFile("cov-excl.go"),
		testCommitFile("ling-gen_test.go"),
		testCommitFile("newlyAddedFile.go"),
		testCommitFile("newlyAddedFile_test.go"),
	}
}

type githubIssues struct {
	githubClient.Issues
}

type githubPullRequests struct {
	githubClient.PullRequests
}

func fakeGithubIssues() githubClient.Issues {
	return &githubIssues{}
}

func fakePullRequests() githubClient.PullRequests {
	return &githubPullRequests{}
}

//CreateComment implements Issues interface. As a fake function,
// it does nothing but prints the parameters and return nils
func (issues *githubIssues) CreateComment(ctx context.Context, owner string, repo string,
	number int, comment *github.IssueComment) (*github.IssueComment, *github.Response, error) {
	logrus.Infof("githubIssues.CreateComment(Ctx, owner=%s, repo=%s, number=%d, "+
		"comment.GetBody()=%s) called\n", owner, repo, number, comment.GetBody())
	return nil, nil, nil
}

//DeleteComment implements Issues interface. As a fake function,
// it does nothing but prints the parameters and return nils
func (issues *githubIssues) DeleteComment(ctx context.Context, owner string, repo string,
	commentID int) (*github.Response, error) {
	logrus.Infof("githubIssues.DeleteComment(Ctx, owner=%s, repo=%s, commentID=%d) called\n",
		owner, repo, commentID)
	return nil, nil
}

//ListComments implements Issues interface. As a fake function,
// it does nothing but prints the parameters and return nils
func (issues *githubIssues) ListComments(ctx context.Context, owner string, repo string, number int,
	opt *github.IssueListCommentsOptions) ([]*github.IssueComment, *github.Response, error) {
	logrus.Infof("githubIssues.ListComment(Ctx, owner=%s, repo=%s, number=%d, "+
		"opt=%v) called\n", owner, repo, number, opt)
	return nil, nil, nil
}

//ListFiles implements PullRequest interface. As a fake function,
// it does nothing but prints the parameters and return nils
func (pr *githubPullRequests) ListFiles(ctx context.Context, owner string, repo string, number int, opt *github.ListOptions) (
	[]*github.CommitFile, *github.Response, error) {
	return testCommitFiles(), nil, nil
}
