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
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/sirupsen/logrus"
)

//LocalArtifacts sub-type of Artifacts. Represent artifacts stored locally (
// as oppose to artifacts stored in GCS bucket)
type LocalArtifacts struct {
	Artifacts
}

//NewLocalArtifacts constructs LocalArtifacts
func NewLocalArtifacts(directory string, profileName string,
	keyProfileName string, covStdoutName string) *LocalArtifacts {
	return &LocalArtifacts{*New(
		directory,
		profileName,
		keyProfileName,
		covStdoutName)}
}

// ProfileReader creates and returns a ProfileReader by opening the file stored in profile path
func (artifacts *LocalArtifacts) ProfileReader() (io.ReadCloser, error) {
	profilePath := artifacts.ProfilePath()
	f, err := os.Open(profilePath)
	if err != nil {
		err = fmt.Errorf("fail to open profile at '%s', error: %v", profilePath, err)
	}
	return f, err
}

//ProfileName gets name of profile
func (artifacts *LocalArtifacts) ProfileName() string {
	return artifacts.profileName
}

// ProduceProfileFile produce coverage profile (&its stdout) by running go test on target package
// for periodic job, produce junit xml for testgrid in addition
func (artifacts *LocalArtifacts) ProduceProfileFile(covTargetsStr string) error {
	// creates artifacts directory
	artifactsDirPath := artifacts.Directory
	logrus.Infof("Making directory (MkdirAll): path=%s", artifactsDirPath)
	if err := os.MkdirAll(artifactsDirPath, 0755); err != nil {
		return fmt.Errorf("failed os.MkdirAll(path='%s', 0755); err='%v'", artifactsDirPath, err)
	}
	logrus.Infof("artifacts dir (path=%s) created successfully", artifactsDirPath)
	covTargets := composeCmdArgs(covTargetsStr, artifacts.ProfilePath())
	return runProfiling(covTargets, artifacts)
}

func composeCmdArgs(covTargetsStr, profileDestinationPath string) []string {
	// generate the complete list of command line args for producing code coverage profile
	var covTargets []string
	for _, target := range strings.Split(covTargetsStr, " ") {
		covTargets = append(covTargets, "./"+path.Join(target, "..."))
	}
	logrus.Infof("list of coverage targets = %v", covTargets)
	cmdArgs := []string{"test"}
	cmdArgs = append(cmdArgs, covTargets...)
	cmdArgs = append(cmdArgs, []string{"-covermode=count",
		"-coverprofile", profileDestinationPath}...)
	return cmdArgs
}
