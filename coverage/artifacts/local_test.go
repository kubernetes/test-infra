package artifacts

import (
	"bufio"
	"fmt"
	"github.com/kubernetes/test-infra/coverage/logUtil"
	"github.com/kubernetes/test-infra/coverage/test"
	"log"
	"testing"
)

// generates coverage profile by running go test on target package
func TestProfiling(t *testing.T) {
	logUtil.LogFatalf = log.Fatalf

	arts := localArtsForTest("TestProfiling")
	arts.ProduceProfileFile(fmt.Sprintf("../%s/subPkg1/ "+
		"../%s/subPkg2/", test.CovTargetRootRel, test.CovTargetRootRel))

	t.Logf("Verifying profile file...\n")
	expectedFirstLine := "mode: count"
	expectedLine := "github.com/kubernetes/test-infra/coverage/testTarget/subPkg1/common.go:3.19,5.2 0 2"

	scanner := bufio.NewScanner(arts.ProfileReader())
	scanner.Scan()
	if scanner.Text() != expectedFirstLine {
		t.Fatalf("File should start with the line '%s';\nit actually starts with '%s'", expectedFirstLine, scanner.Text())
	}

	for scanner.Scan() {
		if scanner.Text() == expectedLine {
			t.Logf("found expected line, test succeeded")
			return
		}
	}

	t.Fatalf("line not found '%s'", expectedLine)
}

func localArtsForTest(dirPrefix string) *LocalArtifacts {
	return &LocalArtifacts{Artifacts: *NewArtifacts(
		test.NewArtsDir(dirPrefix),
		"cov-profile.txt",
		"key-cov-profile.txt",
		"stdout.txt",
	)}
}
