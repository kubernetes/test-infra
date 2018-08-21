package line

import (
	"testing"

	"k8s.io/test-infra/coverage/artifacts/artsTest"
	"k8s.io/test-infra/coverage/test"
)

func TestCreateLineCovFile(t *testing.T) {
	arts := artsTest.LocalArtsForTest("TestCreateLineCovFile")
	test.LinkInputArts(arts.Directory(), "key-cov-profile.txt")

	err := CreateLineCovFile(arts)
	if err != nil {
		t.Fatalf("CreateLineCovFile(arts=%v) failed, err=%v", arts, err)
	}
	test.DeleteDir(arts.Directory())
}

func TestCreateLineCovFileFailure(t *testing.T) {
	arts := artsTest.LocalArtsForTest_KeyfileNotExist("TestCreateLineCovFileFailure")
	if CreateLineCovFile(arts) == nil {
		t.Fatalf("CreateLineCovFile(arts=%v) should fail, but not", arts)
	}
}
