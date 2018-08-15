package githubUtil

import (
	"k8s.io/test-infra/coverage/githubUtil/githubFakes"
	"k8s.io/test-infra/coverage/test"
	"path"
	"testing"
)

func TestGetConcernedFiles(t *testing.T) {
	data := githubFakes.FakeRepoData()
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
