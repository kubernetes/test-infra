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
	"bytes"
	"encoding/json"
	"time"

	"fmt"
	"github.com/sirupsen/logrus"
	"html/template"
	"k8s.io/test-infra/prow/deck/jobs"
	"k8s.io/test-infra/prow/spyglass/lenses"
	"path/filepath"
)

const (
	name     = "metadata"
	title    = "Metadata"
	priority = 0
)

// Lens is the implementation of a metadata-rendering Spyglass lens.
type Lens struct{}

func init() {
	lenses.RegisterLens(Lens{})
}

// Title returns the title.
func (lens Lens) Title() string {
	return title
}

// Name returns the name.
func (lens Lens) Name() string {
	return name
}

// Priority returns the priority.
func (lens Lens) Priority() int {
	return priority
}

// Header renders the <head> from template.html.
func (lens Lens) Header(artifacts []lenses.Artifact, resourceDir string) string {
	t, err := template.ParseFiles(filepath.Join(resourceDir, "template.html"))
	if err != nil {
		return fmt.Sprintf("<!-- FAILED LOADING HEADER: %v -->", err)
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "header", nil); err != nil {
		return fmt.Sprintf("<!-- FAILED EXECUTING HEADER TEMPLATE: %v -->", err)
	}
	return buf.String()
}

// Callback does nothing.
func (lens Lens) Callback(artifacts []lenses.Artifact, resourceDir string, data string) string {
	return ""
}

// Derived contains metadata derived from provided metadata for insertion into the template
type Derived struct {
	StartTime        time.Time
	FinishedTime     time.Time
	Elapsed          time.Duration
	Done             bool
	Status           string
	GCSArtifactsLink string
}

// Body creates a view for prow job metadata.
// TODO: os image,
func (lens Lens) Body(artifacts []lenses.Artifact, resourceDir string, data string) string {
	var buf bytes.Buffer
	type MetadataViewData struct {
		Started  jobs.Started
		Finished jobs.Finished
		Derived  Derived
	}
	var metadataViewData MetadataViewData
	for _, a := range artifacts {
		read, err := a.ReadAll()
		if err != nil {
			logrus.WithError(err).Error("Failed reading from artifact.")
		}
		if a.JobPath() == "started.json" {
			s := jobs.Started{}
			if err = json.Unmarshal(read, &s); err != nil {
				logrus.WithError(err).Error("Error unmarshaling started.json")
			}
			metadataViewData.Derived.StartTime = time.Unix(s.Timestamp, 0)
			metadataViewData.Started = s

		} else if a.JobPath() == "finished.json" {
			metadataViewData.Derived.Done = true
			f := jobs.Finished{}
			if err = json.Unmarshal(read, &f); err != nil {
				logrus.WithError(err).Error("Error unmarshaling finished.json")
			}
			metadataViewData.Derived.FinishedTime = time.Unix(f.Timestamp, 0)
			metadataViewData.Finished = f
		}

	}

	if metadataViewData.Derived.Done {
		metadataViewData.Derived.Status = metadataViewData.Finished.Result
		metadataViewData.Derived.Elapsed =
			metadataViewData.Derived.FinishedTime.Sub(metadataViewData.Derived.StartTime)
	} else {
		metadataViewData.Derived.Status = "In Progress"
		metadataViewData.Derived.Elapsed = time.Now().Sub(metadataViewData.Derived.StartTime)
	}

	metadataTemplate, err := template.ParseFiles(filepath.Join(resourceDir, "template.html"))
	if err != nil {
		return fmt.Sprintf("Failed to load template: %v", err)
	}

	if err := metadataTemplate.ExecuteTemplate(&buf, "body", metadataViewData); err != nil {
		logrus.WithError(err).Error("Error executing template.")
	}
	return buf.String()
}
