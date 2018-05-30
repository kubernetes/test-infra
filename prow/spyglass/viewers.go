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

// This file contains artifact-specific view handlers
package spyglass

import (
	"regexp"

	"github.com/sirupsen/logrus"
)
// An artifact viewer for JUnit tests
type JUnitViewer struct {
	ArtifactViewer
}

// An artifact viewer for build logs
type BuildLogViewer struct {
	ArtifactViewer
}

// An artifact viewer for prow job metadata
type MetadataViewer struct {
	ArtifactViewer
}

// Creates a view for a build log (or multiple build logs)
func (v *BuildLogViewer) View(artifacts []Artifact) Lens {
	//TODO
}

// Creates a view for JUnit tests
func (v *JUnitViewer) View(artifacts []Artifact) Lens {
	//TODO
}

// Creates a view for prow job metadata
func (v *MetdataViewer) View(artifacts []Artifact) Lens {
	//TODO
}


