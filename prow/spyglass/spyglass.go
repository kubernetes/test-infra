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

// Package spyglass creates views for Prow job artifacts.
package spyglass

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/deck/jobs"
	"k8s.io/test-infra/prow/spyglass/viewers"
)

var (
	// These are artifacts that we guarantee will be created with every job.
	// If they are found ina storage location, the stored ones will be used,
	// otherwise we will try to fetch them from the job runtimeIn the future
	// this might contain job metadata, podspec, etc.
	guaranteedArtifacts = []string{"build-log.txt"}
)

// Spyglass records which sets of artifacts need views for a Prow job. The metaphor
// can be understood as follows: A spyglass receives light from a source through
// an eyepiece, which has a lens that ultimately presents a view of the light source
// to the observer. Spyglass receives light (artifacts) via a
// source (src) through the eyepiece (Eyepiece) and presents the view (what you see
// in your browser) via a lens (Lens).
type Spyglass struct {

	// Lenses is a map of names of views to their corresponding lenses
	Lenses map[string]Lens

	// Eyepieces is a list of ArtifactFetchers used by Spyglass. Whenever spyglass
	// receives a request to view artifacts from a source, it will check to see if it
	// can handle that source with one of its registered eyepieces
	Eyepieces []ArtifactFetcher

	// Ja contains information about the current jobs in deck
	Ja *jobs.JobAgent
}

// Lens is a single view of a set of artifacts
type Lens struct {
	// Name of the view, unique within a job
	Name string
	// Title of view
	Title string
	// Html rendering of the view
	HTMLView string
	// Priority is the relative position of the view on the page
	Priority int
}

// ViewRequest holds data sent by a view
type ViewRequest struct {
	ViewName    string              `json:"name"`
	ViewerCache map[string][]string `json:"viewerCache"`
	ViewData    string              `json:"viewData"`
}

// NewSpyglass constructs a spyglass object from a JobAgent
func New(ja *jobs.JobAgent, eyepieces []ArtifactFetcher) *Spyglass {
	return &Spyglass{
		Lenses:    make(map[string]Lens),
		Ja:        ja,
		Eyepieces: eyepieces,
	}
}

// Views gets all views of all artifact files matching each regexp with a registered viewer
func (s *Spyglass) Views(matchCache map[string][]string) []Lens {
	lenses := []Lens{}
	for viewer, matches := range matchCache {
		if len(matches) != 0 {
			title, err := viewers.Title(viewer)
			if err != nil {
				logrus.WithField("viewName", viewer).WithError(err).Error("Could not find artifact viewer")
				continue
			}
			priority, err := viewers.Priority(viewer)
			if err != nil {
				logrus.WithField("viewName", viewer).WithError(err).Error("Could not find artifact viewer with name ", viewer)
				continue
			}
			lens := Lens{
				Name:     viewer,
				Title:    title,
				HTMLView: "",
				Priority: priority,
			}
			lenses = append(lenses, lens)
			s.Lenses[viewer] = lens
		}
	}
	// Make sure lenses are rendered in order by ascending priority
	sort.Slice(lenses, func(i, j int) bool {
		iname := lenses[i].Name
		jname := lenses[j].Name
		pi := lenses[i].Priority
		pj := lenses[j].Priority
		if pi == pj {
			return iname < jname
		}
		return pi < pj
	})
	return lenses
}

// Refresh reloads the html view for a given set of objects
func (s *Spyglass) Refresh(src string, podName string, sizeLimit int64, viewReq *ViewRequest) Lens {
	lens, ok := s.Lenses[viewReq.ViewName]
	if !ok {
		logrus.Errorf("Could not find Lens with name %s.", viewReq.ViewName)
		return Lens{}
	}
	artifacts, err := s.FetchArtifacts(src, podName, sizeLimit, viewReq.ViewerCache[viewReq.ViewName])
	if err != nil {
		logrus.WithError(err).Error("Error while fetching artifacts.")
	}

	view, err := viewers.View(viewReq.ViewName, artifacts, viewReq.ViewData)
	if err != nil {
		logrus.WithError(err).Error("Could not find a valid artifact viewer.")
		return Lens{}
	}
	lens.HTMLView = view
	return lens
}

// ListArtifacts handles muxing artifact sources to the correct fetcher implementations to list
// all available artifact names
func (s *Spyglass) ListArtifacts(src string) ([]string, error) {
	foundArtifacts := []string{}
	for _, ep := range s.Eyepieces {
		jobSource, err := ep.CreateJobSource(src)
		if err != nil {
			if err == ErrCannotParseSource {
				continue
			}
			logrus.WithError(err).Error("Error creating job source.")
		}

		foundArtifacts = append(foundArtifacts, ep.Artifacts(jobSource)...)
	}
	foundArtifacts = append(foundArtifacts, neededArtifacts(foundArtifacts)...)
	return foundArtifacts, nil
}

func neededArtifacts(haveArtifacts []string) []string {
	needed := guaranteedArtifacts
	for _, name := range haveArtifacts {
		for i := 0; i < len(needed); i++ {
			if name == needed[i] {
				needed = append(needed[:i], needed[i+1:]...)
				break
			}
		}
	}
	return needed
}

// FetchArtifacts constructs and returns Artifact objects for each artifact name in the list.
// This includes getting any handles needed for read write operations, direct artifact links, etc.
func (s *Spyglass) FetchArtifacts(src string, podName string, sizeLimit int64, artifactNames []string) ([]viewers.Artifact, error) {
	foundArtifacts := []viewers.Artifact{}
	foundArtifactNames := []string{}
	for _, ep := range s.Eyepieces {
		jobSource, err := ep.CreateJobSource(src)
		if err != nil {
			if err == ErrCannotParseSource {
				continue
			}
			logrus.WithError(err).Error("Error creating job source.")
		}
		artStart := time.Now()
		for _, name := range artifactNames {
			artifact := ep.Artifact(jobSource, name, sizeLimit)
			foundArtifacts = append(foundArtifacts, artifact)
			foundArtifactNames = append(foundArtifactNames, artifact.JobPath())
		}
		artElapsed := time.Since(artStart)
		logrus.Info("Retrieved artifacts in ", artElapsed)

	}

	// Special-casing the fetching of in-cluster artifacts that havent been found yet
	neededArtifactNames := neededArtifacts(foundArtifactNames)
	for _, ga := range neededArtifactNames {
		switch ga {
		case "build-log.txt":
			if isProwJobSource(src) {
				parsed := strings.Split(strings.TrimPrefix(src, "prowjob/"), "/")
				var jobName string
				var buildID string
				if len(parsed) == 2 {
					jobName = parsed[0]
					buildID = parsed[1]
				} else if len(parsed) == 1 {
					podName = parsed[0]
				} else {
					return foundArtifacts, errors.New("invalid Prowjob source provided")
				}
				podLog := NewPodLogArtifact(jobName, buildID, podName, sizeLimit, s.Ja)
				if podLog.Size() != -1 {
					foundArtifacts = append(foundArtifacts, podLog)
				}

			}
		default:
		}
	}
	return foundArtifacts, nil

}
