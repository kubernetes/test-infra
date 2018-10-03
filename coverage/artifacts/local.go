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

package artifacts

import (
	"os"
	"path"
	"strings"
	"io"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/coverage/logUtil"
)

//LocalArtifacts sub-type of Artifacts. Represent artifacts stored locally (
// as oppose to artifacts stored in GCS bucket)
type LocalArtifacts struct {
	Artifacts
}

//NewLocalArtifacts constructs LocalArtifacts
func NewLocalArtifacts(directory string, ProfileName string,
	KeyProfileName string, CovStdoutName string) *LocalArtifacts {
	return &LocalArtifacts{*New(
		directory,
		ProfileName,
		KeyProfileName,
		CovStdoutName)}
}

// ProfileReader create and returns a ProfileReader by opening the file stored in profile path
func (arts *LocalArtifacts) ProfileReader() (io.ReadCloser, error) {
	f, err := os.Open(arts.ProfilePath())
	if err != nil {
		wd, err := os.Getwd()
		logrus.Debugf("LocalArtifacts.ProfileReader(): os.Open(profilePath) error: %v, cwd=%s", err, wd)
	}
	return f, err
}

//ProfileName gets name of profile
func (arts *LocalArtifacts) ProfileName() string {
	return arts.profileName
}

// KeyProfileCreator creates a key profile file that will be used to hold a
// filtered version of coverage profile that only stores the entries that
// will be displayed by line coverage tool
func (arts *LocalArtifacts) KeyProfileCreator() *os.File {
	keyProfilePath := arts.KeyProfilePath()
	keyProfileFile, err := os.Create(keyProfilePath)
	logrus.Infof("os.Create(keyProfilePath)=%s", keyProfilePath)
	if err != nil {
		logUtil.LogFatalf("file(%s) creation error: %v", keyProfilePath, err)
	}

	return keyProfileFile
}

// ProduceProfileFile produce coverage profile (&its stdout) by running go test on target package
// for periodic job, produce junit xml for testgrid in addition
func (arts *LocalArtifacts) ProduceProfileFile(covTargetsStr string) {
	// creates artifacts directory
	artsDirPath := arts.Directory
	logrus.Infof("Making directory (MkdirAll): path=%s", artsDirPath)
	if err := os.MkdirAll(artsDirPath, 0755); err != nil {
		logrus.Fatalf("Failed os.MkdirAll(path='%s', 0755); err='%v'", artsDirPath, err)
	} else {
		logrus.Infof("artifacts dir (path=%s) created successfully\n", artsDirPath)
	}

	// convert targets from a single string to a lists of strings
	var covTargets []string
	for _, target := range strings.Split(covTargetsStr, " ") {
		covTargets = append(covTargets, "./"+path.Join(target, "..."))
	}
	logrus.Infof("covTargets = %v\n", covTargets)

	runProfiling(covTargets, arts)
}
