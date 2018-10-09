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
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/deck/jobs"
	"k8s.io/test-infra/prow/spyglass/viewers"
)

var (
	// These are artifacts that we guarantee will be created with every job.
	// If they are found in a storage location, the stored ones will be used,
	// otherwise we will try to fetch them from the job runtime. In the future
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

	// JobAgent contains information about the current jobs in deck
	JobAgent *jobs.JobAgent
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
	ViewName    string   `json:"name"`
	ViewMatches []string `json:"viewMatches"`
	ViewData    string   `json:"viewData"`
}

// New constructs a Spyglass object from a JobAgent and a list of ArtifactFetchers
func New(ja *jobs.JobAgent, eyepieces []ArtifactFetcher) *Spyglass {
	return &Spyglass{
		Lenses:    make(map[string]Lens),
		JobAgent:  ja,
		Eyepieces: eyepieces,
	}
}

// Views gets all views of all artifact files matching each regexp with a registered viewer
func (s *Spyglass) Views(matchCache map[string][]string) []Lens {
	lenses := []Lens{}
	for viewer, matches := range matchCache {
		if len(matches) == 0 {
			continue
		}
		title, err := viewers.Title(viewer)
		if err != nil {
			logrus.WithField("viewName", viewer).WithError(err).Error("Could not find artifact viewer")
			continue
		}
		priority, err := viewers.Priority(viewer)
		if err != nil {
			logrus.WithField("viewName", viewer).WithError(err).Error("Could not find artifact viewer")
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

// Refresh reloads the html view for a given view and a set of objects
func (s *Spyglass) Refresh(src string, podName string, sizeLimit int64, viewReq *ViewRequest) (Lens, error) {
	lens, ok := s.Lenses[viewReq.ViewName]
	if !ok {
		return Lens{}, fmt.Errorf("error finding view with provided name %s", viewReq.ViewName)
	}
	artifacts, err := s.FetchArtifacts(src, podName, sizeLimit, viewReq.ViewMatches)
	if err != nil {
		return Lens{}, fmt.Errorf("error fetching artifacts: %v", err)
	}

	view, err := viewers.View(viewReq.ViewName, artifacts, viewReq.ViewData)
	if err != nil {
		return Lens{}, fmt.Errorf("error getting view for viewer %s", viewReq.ViewName)
	}
	lens.HTMLView = view
	return lens, nil
}

// ListArtifacts gets the names of all artifacts available from the given source by checking
// if any eyepiece in the Spyglass can receive artifacts from that source.
func (s *Spyglass) ListArtifacts(src string) ([]string, error) {
	foundArtifacts := []string{}
	var parsed bool
	for _, ep := range s.Eyepieces {
		jobSource, err := ep.createJobSource(src)
		if err == ErrCannotParseSource {
			continue
		} else if err != nil {
			return nil, err
		} else {
			parsed = true
			foundArtifacts = append(foundArtifacts, ep.artifacts(jobSource)...)
		}
	}
	if !parsed {
		return nil, fmt.Errorf("unable to parse source: %s, your source string may be incorrectly formatted or Spyglass does not have a compatible eyepiece for that source.", src)
	}
	foundArtifacts = append(foundArtifacts, missingArtifacts(foundArtifacts)...)
	return foundArtifacts, nil
}

func missingArtifacts(haveArtifacts []string) []string {
	var res []string
	for _, guaranteedArtifact := range guaranteedArtifacts {
		var found bool
		for _, haveArtifact := range haveArtifacts {
			if haveArtifact == guaranteedArtifact {
				found = true
			}
		}
		if !found {
			res = append(res, guaranteedArtifact)
		}
	}
	return res
}

// FetchArtifacts constructs and returns Artifact objects for each artifact name in the list.
// This includes getting any handles needed for read write operations, direct artifact links, etc.
func (s *Spyglass) FetchArtifacts(src string, podName string, sizeLimit int64, artifactNames []string) ([]viewers.Artifact, error) {
	foundArtifacts := []viewers.Artifact{}
	foundArtifactNames := []string{}
	for _, ep := range s.Eyepieces {
		jobSource, err := ep.createJobSource(src)
		if err == ErrCannotParseSource {
			continue
		} else if err != nil {
			logrus.WithField("src", src).WithError(err).Error("Error creating job source.")
			continue
		}
		artStart := time.Now()
		for _, name := range artifactNames {
			artifact := ep.artifact(jobSource, name, sizeLimit)
			foundArtifacts = append(foundArtifacts, artifact)
			foundArtifactNames = append(foundArtifactNames, artifact.JobPath())
		}
		artElapsed := time.Since(artStart)
		logrus.WithField("duration", artElapsed).Infof("Retrieved artifacts for %s", src)

	}

	// Special-casing the fetching of in-cluster artifacts that havent been found yet
	neededArtifactNames := missingArtifacts(foundArtifactNames)
	for _, artifactName := range neededArtifactNames {
		switch artifactName {
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
				_, err := s.JobAgent.GetProwJob(jobName, buildID)
				if err != nil {
					continue
				}
				podLog, err := NewPodLogArtifact(jobName, buildID, podName, sizeLimit, s.JobAgent)
				if err != nil {
					logrus.WithField("src", src).WithError(err).Error("Error accessing pod log from given source.")
					continue
				}
				foundArtifacts = append(foundArtifacts, podLog)

			}
		default:
		}
	}
	return foundArtifacts, nil

}
