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
