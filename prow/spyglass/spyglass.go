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

	// map of regex to relevant artifact viewer
	Eyepiece map[string]ArtifactViewer

	// map of names of views to their corresponding lenses
	Lenses [string]Lens
}

// TODO move the artifact interfaces, registration to a separate package for importing, add utils
// ArtifactViewer generates html views for sets of artifacts
type ArtifactViewer interface {
	View(artifacts []Artifact, raw *RawMessage) string
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
}

// Lens is a single view of a set of artifacts
type Lens struct {
	// Title of view
	title    string
	htmlView string
	reMatch  string
}

// NewSpyglass constructs a default spyglass object that renders build logs and Prow metadata
func NewSpyglass() *SpyGlass {
	ep := map[string]ArtifactViewer{
		"build-log.txt": &BuildLogViewer{
			name: "BuildLogViewer"
			title: "Build Log",
		},
		"started.json|finished.json": &MetadataViewer{
			name: "ProwJobMetadataViewer",
			title: "Job Metadata",
		},
	}
	return &SpyGlass{
		Eyepiece: ep,
		Ready: make(chan string),
		Lenses: make(map[string]Lens),

	}
}

// Views gets all views of all artifact files matching each regexp with a registered viewer
func (s *SpyGlass) Views(artifacts []Artifact) []Lens {
	lenses := []Lens{}
	for re, viewer := range s.Eyepiece {
		matches := []Artifact{}
		r := regexp.MustCompile(re)
		for _, a := range artifacts {
			if r.MatchString(a.JobPath()) {
				matches = append(matches, a)
			}
		}
		lens := Lens{
			title:    viewer.Title,
			htmlView: "",
			reMatch:  re,
		}
		lenses = append(lenses, lens)
		s.Lenses[av.Name] = lens
		go func(av ArtifactViewer) {
			lens.htmlView = av.View(matches, json.Unmarshal(``))
		}(viewer)
	}
	return lenses
}

// Refresh reloads the html view for a given set of objects
func (s *SpyGlass) Refresh(viewName string, artifacts []Artifact, raw *json.RawMessage) Lens{
	lens := s.Lenses[viewName]
	re := lens.reMatch
	viewer := s.Eyepiece[re]
	matches := []Artifact{}
	r := regexp.MustCompile(re)
	for _, a := range artifacts {
		if r.MatchString(a.JobPath()) {
			matches = append(matches, a)
		}
	}
	lens.htmlView := viewer.View(matches, raw)
	return Lens
}

// RegisterViewer registers new viewers
func (s *SpyGlass) RegisterViewer(re string, viewer ArtifactViewer) {
	_, err := regexp.Compile(re)
	if err != nil {
		logrus.Fatal(err)
	}
	s.Eyepiece[re] = viewer
}
