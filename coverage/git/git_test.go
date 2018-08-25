package git

import (
	"fmt"
	"os"
	"path"
	"testing"

	"k8s.io/test-infra/coverage/test"
)

var (
	lingGenFilePath = path.Join(test.CovTargetDir, "ling-gen.go")
	covExclFilePath = path.Join(test.CovTargetDir, "cov-excl.go")
	noAttrFilePath  = path.Join(test.CovTargetDir, "no-attr.go")
)

func TestHasGitAttrLingGenPositive(t *testing.T) {
	gopath := os.Getenv("GOPATH")
	fmt.Printf("gopath=*%v*\n", gopath)
	fmt.Printf("filePath=*%s*\n", lingGenFilePath)

	if gopath != "" && !hasGitAttr(gitAttrLinguistGenerated, lingGenFilePath) {
		t.Fail()
	}
}

func TestHasGitAttrLingGenNegative(t *testing.T) {
	gopath := os.Getenv("GOPATH")
	fmt.Printf("gopath=*%v*\n", gopath)
	if gopath != "" && hasGitAttr(gitAttrLinguistGenerated, noAttrFilePath) {
		t.Fail()
	}
}

func TestHasGitAttrCovExcPositive(t *testing.T) {
	gopath := os.Getenv("GOPATH")
	fmt.Printf("gopath=*%v*\n", gopath)
	if gopath != "" && !hasGitAttr(gitAttrCoverageExcluded, covExclFilePath) {
		t.Fail()
	}
}

func TestHasGitAttrCovExcNegative(t *testing.T) {
	gopath := os.Getenv("GOPATH")
	fmt.Printf("gopath=*%v*\n", gopath)
	if gopath != "" && hasGitAttr(gitAttrCoverageExcluded, noAttrFilePath) {
		t.Fail()
	}
}
