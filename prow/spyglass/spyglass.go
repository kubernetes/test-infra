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
	"k8s.io/prow/artifact-fetcher"
	"k8s.io/prow/kube"
)

// SpyGlass records which sets of artifacts need views for a prow job
type SpyGlass struct {
	job      kube.ProwJob
	eyepiece map[Regexp]ArtifactViewer
}

// TODO move the artifact interfaces, registration to a separate package for importing, add utils
// ArtifactViewer generates html views for sets of artifacts
type ArtifactViewer interface {
	View(artifacts []Artifact) Lens
}

// Artifact represents some output of a prow job
type Artifact interface {
	io.ReadSeeker
	CanonicalLink() string
	JobPath() string
	ReadAll() string
}

// Lens is a single view of a set of artifacts
type Lens struct {
	title    string
	htmlView string
	match    Regexp
}

// Gets all views of all artifact files matching a regexp with a registered viewer
func (s *SpyGlass) Views(artifacts []Artifact) []Lens {
	// TODO: Separate out types of artifacts by regexp, handle each list of artifacts with the handler, generate a view, return the views
}

// Registers new viewers
func (s *SpyGlass) RegisterViewer(re Regexp, viewer *ArtifactViewer) {
	s.eyepiece[re] = viewer
}
