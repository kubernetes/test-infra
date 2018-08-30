/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
