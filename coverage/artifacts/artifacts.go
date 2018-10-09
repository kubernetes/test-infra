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

// Package artifacts is responsible for generating and structuring artifacts
// directory
package artifacts

import (
	"path"
)

const (
	//CovProfileCompletionMarker is the file name of the completion marker for coverage profiling
	CovProfileCompletionMarker = "profile-completed"
)

//Artifacts stores information about artifacts directory and files in it
type Artifacts struct {
	Directory      string
	profileName    string
	keyProfileName string
	covStdoutName  string
}

//New Artifact object
func New(directory, profileName, keyProfileName, covStdoutName string) *Artifacts {
	return &Artifacts{
		Directory:directory,
		profileName:profileName,
		keyProfileName:keyProfileName,
		covStdoutName:covStdoutName}
}

//ProfilePath returns profile path on disk, accessible by file io functions
func (artifacts *Artifacts) ProfilePath() string {
	return path.Join(artifacts.Directory, artifacts.profileName)
}

//KeyProfilePath returns key profile path on disk, accessible by file io functions
func (artifacts *Artifacts) KeyProfilePath() string {
	return path.Join(artifacts.Directory, artifacts.keyProfileName)
}

//CovStdoutPath returns stdout path
func (artifacts *Artifacts) CovStdoutPath() string {
	return path.Join(artifacts.Directory, artifacts.covStdoutName)
}
