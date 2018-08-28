package artsTest

import (
	"path"
	"testing"

	"k8s.io/test-infra/coverage/io"
)

func TestLocalInputArts(t *testing.T) {
	arts := LocalInputArtsForTest()
	if io.FileOrDirExists(arts.Directory()) == false {
		t.Fatalf("FileOrDirExists(arts.Directory()) == false\n")
	}
	if io.FileOrDirExists(arts.ProfilePath()) == false {
		t.Fatalf("FileOrDirExists(%s) == false\n", arts.ProfilePath())
	}
	if io.FileOrDirExists(arts.ProfilePath()) == false {
		t.Fatalf("Profile File not exist\n")
	}
	if io.FileOrDirExists(arts.KeyProfilePath()) == false {
		t.Fatalf("Key Profile File not exist\n")
	}
	if io.FileOrDirExists(arts.CovStdoutPath()) == false {
		t.Fatalf("FileOrDirExists(arts.CovStdoutPath()) == false\n")
	}
}

func TestLocalArtsDirExistence(t *testing.T) {
	arts := LocalArtsForTest("ba")
	content := "lol"
	io.Write(&content, arts.Directory(), "helloworld")
	filePath := path.Join(arts.Directory(), "helloworld")
	if !io.FileOrDirExists(arts.Directory()) {
		t.Fatalf("arts dir dne")
	} else if !io.FileOrDirExists(filePath) {
		t.Fatalf("profile dne: path=%s", filePath)

	}
}
