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
	"encoding/json"
	"regexp"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/spyglass/viewers"
	"k8s.io/test-infra/prow/spyglass/viewers/buildlog"
	"k8s.io/test-infra/prow/spyglass/viewers/metadata"
)

// SpyGlass records which sets of artifacts need views for a prow job
type SpyGlass struct {

	// map of regex to relevant artifact viewer
	Eyepiece map[string]viewers.Viewer

	// map of names of views to their corresponding lenses
	Lenses map[string]Lens
}

// Lens is a single view of a set of artifacts
type Lens struct {
	// Name of the view, unique within a job
	Name string
	// Title of view
	Title    string
	HtmlView string
	ReMatch  string
}

// NewSpyglass constructs a default spyglass object that renders build logs and Prow metadata
func NewSpyGlass() *SpyGlass {
	ep := map[string]viewers.Viewer{
		"build-log.txt": &buildlog.BuildLogViewer{
			ViewName:  "BuildLogViewer",
			ViewTitle: "Build Log",
		},
		"started.json|finished.json": &metadata.MetadataViewer{
			ViewName:  "ProwJobMetadataViewer",
			ViewTitle: "Job Metadata",
		},
	}
	return &SpyGlass{
		Eyepiece: ep,
		Lenses:   make(map[string]Lens),
	}
}

// Views gets all views of all artifact files matching each regexp with a registered viewer
func (s *SpyGlass) Views(artifacts []viewers.Artifact) []Lens {
	lenses := []Lens{}
	for re, viewer := range s.Eyepiece {
		matches := []viewers.Artifact{}
		r := regexp.MustCompile(re)
		for _, a := range artifacts {
			if r.MatchString(a.JobPath()) {
				matches = append(matches, a)
			}
		}
		lens := Lens{
			Name:     viewer.Name(),
			Title:    viewer.Title(),
			HtmlView: "",
			ReMatch:  re,
		}
		lenses = append(lenses, lens)
		s.Lenses[viewer.Name()] = lens
	}
	return lenses
}

// Refresh reloads the html view for a given set of objects
func (s *SpyGlass) Refresh(viewName string, artifacts []viewers.Artifact, raw *json.RawMessage) Lens {
	lens, ok := s.Lenses[viewName]
	if !ok {
		logrus.Errorf("Could not find Lens with name %s.", viewName)
		return Lens{}
	}
	re := lens.ReMatch
	viewer, ok := s.Eyepiece[re]
	if !ok {
		logrus.Errorf("Could not find registered artifact viewer for regexp %s.", re)
		return Lens{}
	}

	matches := []viewers.Artifact{}
	r := regexp.MustCompile(re)
	for _, a := range artifacts {
		if r.MatchString(a.JobPath()) {
			matches = append(matches, a)
		}
	}
	lens.HtmlView = viewer.View(matches, raw)
	return lens
}

// RegisterViewer registers new viewers
func (s *SpyGlass) RegisterViewer(re string, viewer viewers.Viewer) {
	_, err := regexp.Compile(re)
	if err != nil {
		logrus.Fatal(err)
	}
	s.Eyepiece[re] = viewer
}
