package io

import (
	"io/ioutil"
	"k8s.io/test-infra/coverage/test"
	"log"
	"path"
	"testing"
)

func TestWriteToArtifacts(t *testing.T) {
	s := "content to be written on disk"
	artsDir := test.NewArtsDir("TestWriteToArtifacts")
	Write(&s, artsDir, "testWriteToArt.txt")
	content, err := ioutil.ReadFile(path.Join(artsDir, "testWriteToArt.txt"))
	if err != nil {
		log.Fatalf("Cannot read file, err = %v", err)
	}

	test.AssertEqual(t, s, string(content))

	test.DeleteDir(artsDir)
}
