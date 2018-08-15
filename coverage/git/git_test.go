package git

import (
	"fmt"
	"k8s.io/test-infra/coverage/test"
	"os"
	"path"
	"testing"
)

var (
	lingGenFilePath = path.Join(test.CovTargetDir, "ling-gen.go")
	covExclFilePath = path.Join(test.CovTargetDir, "cov-excl.go")
	noAttrFilePath  = path.Join(test.CovTargetDir, "no-attr.go")
)

func TestHasGitAttrLingGenPositive(t *testing.T) {
	fmt.Printf("getenv=*%v*\n", os.Getenv("GOPATH"))
	fmt.Printf("filePath=*%s*\n", lingGenFilePath)

	if !hasGitAttr(gitAttrLinguistGenerated, lingGenFilePath) {
		t.Fail()
	}
}

func TestHasGitAttrLingGenNegative(t *testing.T) {
	if hasGitAttr(gitAttrLinguistGenerated, noAttrFilePath) {
		t.Fail()
	}
}

func TestHasGitAttrCovExcPositive(t *testing.T) {
	if !hasGitAttr(gitAttrCoverageExcluded, covExclFilePath) {
		t.Fail()
	}
}

func TestHasGitAttrCovExcNegative(t *testing.T) {
	if hasGitAttr(gitAttrCoverageExcluded, noAttrFilePath) {
		t.Fail()
	}
}
