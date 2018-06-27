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

// Package metadata provides a metadata viewer for Spyglass
package metadata

import (
	"encoding/json"

	"k8s.io/test-infra/prow/spyglass/viewers"
)

// An artifact viewer for build logs
type MetadataViewer struct {
	ViewName  string
	ViewTitle string
}

// View creates a view for prow job metadata
func (v *MetadataViewer) View(artifacts []viewers.Artifact, raw *json.RawMessage) string {
	//TODO
	return ""
}

// Title gets the title of the viewer
func (v *MetadataViewer) Title() string {
	return v.ViewTitle
}

// Name gets the unique name of the viewer within the job
func (v *MetadataViewer) Name() string {
	return v.ViewName
}
