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

// Spyglass creates views for build artifacts
package spyglass

import (
	"io"
	"regexp"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/kube"
)

// SpyGlass records which sets of artifacts need views for a prow job
type SpyGlass struct {
	job      kube.ProwJob
	eyepiece map[string]ArtifactViewer
}

// TODO move the artifact interfaces, registration to a separate package for importing, add utils
// ArtifactViewer generates html views for sets of artifacts
type ArtifactViewer interface {
	View(artifacts []Artifact) string
	Title() string
}

// Artifact represents some output of a prow job
type Artifact interface {
	io.ReaderAt
	CanonicalLink() string
	JobPath() string
	ReadAll() ([]byte, error)
	ReadTail(n int64) ([]byte, error)
	Size() int64
	// TODO functionalities
	// Read entire file, error if too big
	// Get last chunk of file
}

// Lens is a single view of a set of artifacts
type Lens struct {
	title    string
	htmlView string
	reMatch  string
}

// Gets all views of all artifact files matching a regexp with a registered viewer
func (s *SpyGlass) Views(artifacts []Artifact) []Lens {
	lenses := []Lens{}
	for re, viewer := range s.eyepiece {
		matches := []Artifact{}
		r := regexp.MustCompile(re)
		for _, a := range artifacts {
			if r.MatchString(a.JobPath()) {
				matches = append(matches, a)
			}
		}
		lens := Lens{
			title:    viewer.Title(),
			htmlView: "",
			reMatch:  re,
		}
		lenses = append(lenses, lens)
		go func() {
			lens.htmlView = viewer.View(matches)
		}()
	}
	return lenses
}

// Registers new viewers
func (s *SpyGlass) RegisterViewer(re string, viewer ArtifactViewer) {
	_, err := regexp.Compile(re)
	if err != nil {
		logrus.Fatal(err)
	}
	s.eyepiece[re] = viewer
}
