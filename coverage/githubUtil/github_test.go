package githubUtil

import (
	"testing"
)

func TestFilePathProfileToGithub(t *testing.T) {
	input := "github.com/myRepoOwner/myRepoName/pkg/ab/cde"
	expectedOutput := "pkg/ab/cde"
	actualOutput := FilePathProfileToGithub(input)
	if actualOutput != expectedOutput {
		t.Fatalf("input=%s; expected output=%s; actual output=%s", input, expectedOutput,
			actualOutput)
	}
}
