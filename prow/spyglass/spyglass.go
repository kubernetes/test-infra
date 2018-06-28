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
)

// SpyGlass records which sets of artifacts need views for a prow job
type SpyGlass struct {

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
	return &SpyGlass{
		Lenses: make(map[string]Lens),
	}
}

// Views gets all views of all artifact files matching each regexp with a registered viewer
func (s *SpyGlass) Views(artifacts []viewers.Artifact, eyepiece map[string]string) []Lens {
	lenses := []Lens{}
	for re, viewer := range eyepiece {
		matches := []viewers.Artifact{}
		r, err := regexp.Compile(re)
		if err != nil {
			logrus.Errorf("Regexp %s failed to compile.", re)
			continue
		}
		for _, a := range artifacts {
			if r.MatchString(a.JobPath()) {
				matches = append(matches, a)
			}
		}
		title, err := viewers.Title(viewer)
		if err != nil {
			logrus.Error("Could not find artifact viewer with name ", viewer)
		}
		lens := Lens{
			Name:     viewer,
			Title:    title,
			HtmlView: "",
			ReMatch:  re,
		}
		lenses = append(lenses, lens)
		s.Lenses[viewer] = lens
	}
	return lenses
}

// Refresh reloads the html view for a given set of objects
func (s *SpyGlass) Refresh(viewName string, artifacts []viewers.Artifact, raw *json.RawMessage) Lens {
	lens, ok := s.Lenses[viewName]
	if !ok {
		logrus.Errorf("Could not find Lens with name %s", viewName)
		return Lens{}
	}
	re := lens.ReMatch
	matches := []viewers.Artifact{}
	r := regexp.MustCompile(re)
	for _, a := range artifacts {
		if r.MatchString(a.JobPath()) {
			matches = append(matches, a)
		}
	}
	view, err := viewers.View(viewName, matches, raw)
	if err != nil {
		logrus.Errorf("Could not find registered artifact viewer for name=%s", viewName)
		return Lens{}
	}
	lens.HtmlView = view
	return lens
}
