// Package artifacts is responsible for generating and structuring artifacts
// directory
package artifacts

import (
	"path"
)

const (
	//CovProfileCompletionMarker is the file name of the completion marker for coverage profiling
	CovProfileCompletionMarker = "profile-completed"
	junitXmlForTestgrid        = "junit_bazel.xml"
	//LineCovFileName is the file name for line coverage html
	LineCovFileName            = "line-cov.html"
)

//Artifacts stores information about artifacts directory and files in it
type Artifacts struct {
	directory      string
	profileName    string
	keyProfileName string
	covStdoutName  string
}

//New Artifact object
func New(directory, profileName, keyProfileName, covStdoutName string) *Artifacts {
	return &Artifacts{
		directory,
		profileName,
		keyProfileName,
		covStdoutName}
}

//SetDirectory sets directory to given string
func (arts *Artifacts) SetDirectory(dir string) {
	arts.directory = dir
}

//Directory gets directory string
func (arts *Artifacts) Directory() string {
	return arts.directory
}

//ProfilePath returns profile path
func (arts *Artifacts) ProfilePath() string {
	return path.Join(arts.directory, arts.profileName)
}

//KeyProfilePath returns key profile path
func (arts *Artifacts) KeyProfilePath() string {
	return path.Join(arts.directory, arts.keyProfileName)
}

//CovStdoutPath returns stdout path
func (arts *Artifacts) CovStdoutPath() string {
	return path.Join(arts.directory, arts.covStdoutName)
}

//JunitXmlForTestgridPath returns path for xml used by testgrid
func (arts *Artifacts) JunitXmlForTestgridPath() string {
	return path.Join(arts.directory, junitXmlForTestgrid)
}

//LineCovFilePath returns path for line coverage
func (arts *Artifacts) LineCovFilePath() string {
	return path.Join(arts.directory, LineCovFileName)
}
