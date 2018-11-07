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
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/deck/jobs"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/spyglass/viewers"
)

// Key types specify the way Spyglass will fetch artifact handles
const (
	gcsKeyType  = "gcs"
	prowKeyType = "prowjob"
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

	// JobAgent contains information about the current jobs in deck
	JobAgent *jobs.JobAgent

	*GCSArtifactFetcher
	*PodLogArtifactFetcher
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
func New(ja *jobs.JobAgent, c *storage.Client) *Spyglass {
	return &Spyglass{
		Lenses:                make(map[string]Lens),
		JobAgent:              ja,
		PodLogArtifactFetcher: NewPodLogArtifactFetcher(ja),
		GCSArtifactFetcher:    NewGCSArtifactFetcher(c),
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

func splitSrc(src string) (keyType, key string, err error) {
	split := strings.SplitN(src, "/", 2)
	if len(split) < 2 {
		err = fmt.Errorf("invalid src %s: expected <key-type>/<key>", src)
		return
	}
	keyType = split[0]
	key = split[1]
	return
}

// ListArtifacts gets the names of all artifacts available from the given source
func (s *Spyglass) ListArtifacts(src string) ([]string, error) {
	keyType, key, err := splitSrc(src)
	if err != nil {
		return []string{}, fmt.Errorf("error parsing src: %v", err)
	}
	switch keyType {
	case gcsKeyType:
		return s.GCSArtifactFetcher.artifacts(key)
	case prowKeyType:
		gcsKey, err := s.prowToGCS(key)
		if err != nil {
			logrus.Warningf("Failed to get gcs source for prow job: %v", err)
			return []string{}, nil
		}
		artifactNames, err := s.GCSArtifactFetcher.artifacts(gcsKey)
		logFound := false
		for _, name := range artifactNames {
			if name == "build-log.txt" {
				logFound = true
				break
			}
		}
		if err != nil || !logFound {
			artifactNames = append(artifactNames, "build-log.txt")
		}
		return artifactNames, nil
	default:
		return nil, fmt.Errorf("Unrecognized key type for src: %v", src)
	}
}

// JobPath returns a link to the GCS directory for the job specified in src
func (s *Spyglass) JobPath(src string) (string, error) {
	src = strings.TrimSuffix(src, "/")
	keyType, key, err := splitSrc(src)
	if err != nil {
		return "", fmt.Errorf("error parsing src: %v", src)
	}
	split := strings.Split(key, "/")
	switch keyType {
	case gcsKeyType:
		if len(split) < 4 {
			return "", fmt.Errorf("invalid key %s: expected <bucket-name>/<log-type>/.../<job-name>/<build-id>", key)
		}
		// see https://github.com/kubernetes/test-infra/tree/master/gubernator
		bktName := split[0]
		logType := split[1]
		jobName := split[len(split)-2]
		if logType == "logs" {
			return path.Dir(key), nil
		} else if logType == "pr-logs" {
			return path.Join(bktName, "pr-logs/directory", jobName), nil
		}
		return "", fmt.Errorf("unrecognized GCS key: %s", key)
	case prowKeyType:
		if len(split) < 2 {
			return "", fmt.Errorf("invalid key %s: expected <job-name>/<build-id>", key)
		}
		jobName := split[0]
		buildID := split[1]
		job, err := s.jobAgent.GetProwJob(jobName, buildID)
		if err != nil {
			return "", fmt.Errorf("failed to get prow job from src %q: %v", key, err)
		}
		bktName := job.Spec.DecorationConfig.GCSConfiguration.Bucket
		if job.Spec.Type == kube.PresubmitJob {
			return path.Join(bktName, "pr-logs/directory", jobName), nil
		}
		return path.Join(bktName, "logs", jobName), nil
	default:
		return "", fmt.Errorf("unrecognized key type for src: %v", src)
	}
}

// prowToGCS returns the GCS key corresponding to the given prow key
func (s *Spyglass) prowToGCS(prowKey string) (string, error) {
	parsed := strings.Split(prowKey, "/")
	if len(parsed) != 2 {
		return "", fmt.Errorf("Could not get GCS src: prow src %q incorrectly formatted", prowKey)
	}
	jobName := parsed[0]
	buildID := parsed[1]

	job, err := s.jobAgent.GetProwJob(jobName, buildID)
	if err != nil {
		return "", fmt.Errorf("Failed to get prow job from src %q: %v", prowKey, err)
	}

	url := job.Status.URL
	buildIndex := strings.Index(url, "/build/")
	if buildIndex == -1 {
		return "", fmt.Errorf("Unrecognized GCS url: %q", url)
	}
	return url[buildIndex+len("/build/"):], nil
}

// FetchArtifacts constructs and returns Artifact objects for each artifact name in the list.
// This includes getting any handles needed for read write operations, direct artifact links, etc.
func (s *Spyglass) FetchArtifacts(src string, podName string, sizeLimit int64, artifactNames []string) ([]viewers.Artifact, error) {
	artStart := time.Now()
	arts := []viewers.Artifact{}
	keyType, key, err := splitSrc(src)
	if err != nil {
		return arts, fmt.Errorf("error parsing src: %v", err)
	}
	switch keyType {
	case gcsKeyType:
		for _, name := range artifactNames {
			art, err := s.GCSArtifactFetcher.artifact(key, name, sizeLimit)
			if err != nil {
				logrus.Errorf("Failed to fetch artifact %s: %v", name, err)
				continue
			}
			arts = append(arts, art)
		}
	case prowKeyType:
		logFound := false
		if gcsKey, err := s.prowToGCS(key); err == nil {
			for _, name := range artifactNames {
				if name == "build-log.txt" {
					logFound = true
				}
				art, err := s.GCSArtifactFetcher.artifact(gcsKey, name, sizeLimit)
				if err != nil {
					logrus.Errorf("Failed to fetch artifact %s: %v", name, err)
					continue
				}
				arts = append(arts, art)
			}
		} else {
			logrus.Warningln(err)
		}
		if !logFound {
			art, err := s.PodLogArtifactFetcher.artifact(key, sizeLimit)
			if err != nil {
				logrus.Errorf("Failed to fetch pod log: %v", err)
			} else {
				arts = append(arts, art)
			}
		}
	default:
		return nil, fmt.Errorf("Invalid src: %v", src)
	}

	logrus.WithField("duration", time.Since(artStart)).Infof("Retrieved artifacts for %v", src)
	return arts, nil
}
