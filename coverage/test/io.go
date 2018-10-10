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

package test

import (
	"io/ioutil"
	"os"
	"path"

	"github.com/sirupsen/logrus"
)

//DeleteDir deletes a directory on disk
func DeleteDir(dir string) {
	err := os.RemoveAll(dir)
	if err != nil {
		logrus.Fatalf("Fail to remove artifact '%s': %v", dir, err)
	}
}

func linkInputArt(artifactsDir, artName string) {
	err := os.Symlink(path.Join(InputArtifactsDir, artName),
		path.Join(artifactsDir, artName))

	if err != nil {
		logrus.Fatalf("Error creating Symlink: %v", err)
	}
}

//NewArtifactsDir create an artifact directory on disk
func NewArtifactsDir(dirPrefix string) string {
	os.MkdirAll(tmpArtifactsDir, 0755)
	dir, err := ioutil.TempDir(tmpArtifactsDir, dirPrefix+"_")
	logrus.Infof("Artifacts directory ='%s'", dir)
	if err != nil {
		logrus.Fatalf("Error making TempDir for artifacts: %v", err)
	} else {
		logrus.Infof("Temp artifacts dir created: %s", dir)
	}
	return dir
}
