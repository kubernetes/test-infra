package githubFakes

import (
	"context"
	"path"

	"github.com/google/go-github/github"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/coverage/githubUtil/githubClient"
	"k8s.io/test-infra/coverage/githubUtil/githubPr"
	"k8s.io/test-infra/coverage/test"
)

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

type FakeGithubIssues struct {
	githubClient.Issues
}

type FakeGithubPullRequests struct {
	githubClient.PullRequests
}

func fakeGithubIssues() githubClient.Issues {
	return &FakeGithubIssues{}
}

func fakePullRequests() githubClient.PullRequests {
	return &FakeGithubPullRequests{}
}

func (issues *FakeGithubIssues) CreateComment(ctx context.Context, owner string, repo string,
	number int, comment *github.IssueComment) (*github.IssueComment, *github.Response, error) {
	logrus.Infof("FakeGithubIssues.CreateComment(Ctx, owner=%s, repo=%s, number=%d, "+
		"comment.GetBody()=%s) called\n", owner, repo, number, comment.GetBody())
	return nil, nil, nil
}

func (issues *FakeGithubIssues) DeleteComment(ctx context.Context, owner string, repo string,
	commentID int) (*github.Response, error) {
	logrus.Infof("FakeGithubIssues.DeleteComment(Ctx, owner=%s, repo=%s, commentID=%d) called\n",
		owner, repo, commentID)
	return nil, nil
}

func (issues *FakeGithubIssues) ListComments(ctx context.Context, owner string, repo string, number int,
	opt *github.IssueListCommentsOptions) ([]*github.IssueComment, *github.Response, error) {
	logrus.Infof("FakeGithubIssues.ListComment(Ctx, owner=%s, repo=%s, number=%d, "+
		"opt=%v) called\n", owner, repo, number, opt)
	return nil, nil, nil
}

func (pr *FakeGithubPullRequests) ListFiles(ctx context.Context, owner string, repo string, number int, opt *github.ListOptions) (
	[]*github.CommitFile, *github.Response, error) {
	return testCommitFiles(), nil, nil
}

func FakeRepoData() *githubPr.GithubPr {
	ctx := context.Background()
	logrus.Infof("creating fake repo data \n")

	return &githubPr.GithubPr{
		RepoOwner:     "fakeRepoOwner",
		RepoName:      "fakeRepoName",
		Pr:            7,
		RobotUserName: "fakeCovbot",
		GithubClient:  FakeGithubClient(),
		Ctx:           ctx,
	}
}
