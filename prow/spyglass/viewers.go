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

// An artifact viewer for JUnit tests
type JUnitViewer struct {
	ArtifactViewer
	title string
}

// An artifact viewer for build logs
type BuildLogViewer struct {
	ArtifactViewer
	title string
}

// An artifact viewer for prow job metadata
type MetadataViewer struct {
	ArtifactViewer
	title string
}

// Gets the title of the JUnit View
func (v *JUnitViewer) Title() string {
	return v.title
}

// Gets the title of the JUnit View
func (v *BuildLogViewer) Title() string {
	return v.title
}

// Gets the title of the Metadata View
func (v *MetadataViewer) Title() string {
	return v.title
}

// Creates a view for a build log (or multiple build logs)
func (v *BuildLogViewer) View(artifacts []Artifact) Lens {
	lens := Lens{}
	for _, a := range artifacts {
		if a.Size() > 1000 {
			//TODO
		}
	}
	return lens
}

// Creates a view for JUnit tests
func (v *JUnitViewer) View(artifacts []Artifact) Lens {
	//TODO
	return Lens{}
}

// Creates a view for prow job metadata
func (v *MetadataViewer) View(artifacts []Artifact) Lens {
	//TODO
	return Lens{}
}
