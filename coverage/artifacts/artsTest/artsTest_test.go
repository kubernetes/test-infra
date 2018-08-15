package artsTest

import (
	"k8s.io/test-infra/coverage/test"
	"testing"
)

func TestLocalInputArts(t *testing.T) {
	arts := LocalInputArtsForTest()
	if test.FileOrDirExists(arts.Directory()) == false {
		t.Fatalf("FileOrDirExists(arts.Directory()) == false\n")
	}
	if test.FileOrDirExists(arts.ProfilePath()) == false {
		t.Fatalf("Profile File not exist\n")
	}
	if test.FileOrDirExists(arts.KeyProfilePath()) == false {
		t.Fatalf("Key Profile File not exist\n")
	}
	if test.FileOrDirExists(arts.CovStdoutPath()) == false {
		t.Fatalf("FileOrDirExists(arts.CovStdoutPath()) == false\n")
	}
}
