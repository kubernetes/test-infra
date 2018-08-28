package githubUtil

import (
	"context"
	"os"
	"path"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/coverage/githubUtil/githubFakes"
	"k8s.io/test-infra/coverage/githubUtil/githubPR"
	"k8s.io/test-infra/coverage/test"
)

func fakeRepoData() *githubPR.GithubPr {
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

func TestGetConcernedFiles(t *testing.T) {
	if os.Getenv("GOPATH") != "" {
		data := fakeRepoData()
		actualConcernMap := GetConcernedFiles(data, test.ProjDir())
		t.Logf("concerned files for PR %v:%v", data.Pr, actualConcernMap)
		expectedConcerns := test.MakeStringSet()

		for _, fileName := range []string{
			"common.go",
			"onlySrcChange.go",
			"onlyTestChange.go",
			"newlyAddedFile.go",
		} {
			expectedConcerns.Add(path.Join(test.CovTargetDir, fileName))
		}

		t.Logf("expected concerns=%v", expectedConcerns.AllMembers())

		for fileName, actual := range *actualConcernMap {
			expected := expectedConcerns.Has(fileName)
			if actual != expected {
				t.Fatalf("filename=%s, isConcerned: expected=%v; actual=%v\n", fileName, expected, actual)
			}
		}
	}
}

func TestSourceFilePath(t *testing.T) {
	input := "pkg/fake_test.go"
	actual := sourceFilePath(input)
	expected := "pkg/fake.go"
	if actual != expected {
		t.Fatalf(test.StrFailure(input, actual, expected))
	}

	input = "pkg/fake_2.go"
	actual = sourceFilePath(input)
	expected = "pkg/fake_2.go"
	if actual != expected {
		t.Fatalf(test.StrFailure(input, actual, expected))
	}
}
