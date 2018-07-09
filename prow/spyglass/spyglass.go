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
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/deck/jobs"
	"k8s.io/test-infra/prow/spyglass/viewers"
)

// SpyGlass records which sets of artifacts need views for a prow job
type Spyglass struct {

	// map of names of views to their corresponding lenses
	Lenses map[string]Lens

	// Job Agent for spyglass
	Ja *jobs.JobAgent
}

// Lens is a single view of a set of artifacts
type Lens struct {
	// Name of the view, unique within a job
	Name string
	// Title of view
	Title string
	// Priority of the view on the page
	Priority float32
	HtmlView string
	ReMatch  string
}

// NewSpyglass constructs a spyglass object from a JobAgent
func NewSpyglass(ja *jobs.JobAgent) *Spyglass {
	return &Spyglass{
		Lenses: make(map[string]Lens),
		Ja:     ja,
	}
}

// Views gets all views of all artifact files matching each regexp with a registered viewer
func (s *Spyglass) Views(artifacts []viewers.Artifact, eyepiece map[string]string) []Lens {
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
func (s *Spyglass) Refresh(viewName string, artifacts []viewers.Artifact, raw *json.RawMessage) Lens {
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

// FetchArtifacts handles muxing artifact sources to the correct fetcher implementations
func (sg *Spyglass) FetchArtifacts(src string, jobId string) ([]viewers.Artifact, error) {
	artifacts := []viewers.Artifact{}
	var jobName string
	// First check src
	if isGCSSource(src) {
		artifactFetcher := NewGCSArtifactFetcher()
		gcsJobSource := NewGCSJobSource(src)
		jobName = gcsJobSource.JobName()
		if jobId == "" {
			jobId = gcsJobSource.JobId()
			logrus.Info("Extracted jobId from source. ", gcsJobSource.JobId())
		}

		artStart := time.Now()
		artifacts = append(artifacts, artifactFetcher.Artifacts(gcsJobSource)...)
		artElapsed := time.Since(artStart)
		logrus.Info("Retrieved GCS artifacts in ", artElapsed)

	} else {
		return []viewers.Artifact{}, errors.New(fmt.Sprintf("Invalid source: %s", src))
	}

	// Then check prowjob id for pod logs, pod spec, etc
	if jobId != "" && jobName != "" {
		logrus.Info("Trying pod logs. ")
		podLog := NewPodLogArtifact(jobName, jobId, sg.Ja)
		if podLog.Size() != -1 {
			artifacts = append(artifacts, podLog)
		}

	}
	return artifacts, nil
}
