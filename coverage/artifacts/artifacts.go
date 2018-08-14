// Package artifacts is responsible for generating and structuring artifacts
// directory
package artifacts

import (
	"path"
)

const (
	CovProfileCompletionMarker = "profile-completed"
	JunitXmlForTestgrid        = "junit_bazel.xml"
	LineCovFileName            = "line-cov.html"
)

type Intf interface {
	ProfilePath() string
	KeyProfilePath() string
	ProfileReader() *ProfileReader
}

type Artifacts struct {
	directory      string
	profileName    string
	keyProfileName string
	covStdoutName  string
}

func New(directory string, profileName string, keyProfileName string,
	covStdoutName string) *Artifacts {
	return &Artifacts{
		directory,
		profileName,
		keyProfileName,
		covStdoutName}
}

func (arts *Artifacts) SetDirectory(dir string) {
	arts.directory = dir
}

func (arts *Artifacts) Directory() string {
	return arts.directory
}

func (arts *Artifacts) ProfilePath() string {
	return path.Join(arts.directory, arts.profileName)
}

func (arts *Artifacts) KeyProfilePath() string {
	return path.Join(arts.directory, arts.keyProfileName)
}

func (arts *Artifacts) CovStdoutPath() string {
	return path.Join(arts.directory, arts.covStdoutName)
}

func (arts *Artifacts) JunitXmlForTestgridPath() string {
	return path.Join(arts.directory, JunitXmlForTestgrid)
}

func LineCovFilePath(directory string) string {
	return path.Join(directory, LineCovFileName)
}

func (arts *Artifacts) LineCovFilePath() string {
	return LineCovFilePath(arts.directory)
}
