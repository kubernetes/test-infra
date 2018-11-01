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

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/deck/jobs"
	"k8s.io/test-infra/prow/spyglass/viewers"
)

const (
	name     = "metadata-viewer"
	title    = "Metadata"
	priority = 0
)

func init() {
	viewers.RegisterViewer(name, viewers.ViewMetadata{
		Title:    title,
		Priority: priority,
	}, ViewHandler)
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

// ViewHandler creates a view for prow job metadata
// TODO: os image,
func ViewHandler(artifacts []viewers.Artifact, raw string) string {
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
	if err := metadataTemplate.Execute(&buf, metadataViewData); err != nil {
		logrus.WithError(err).Error("Error executing template.")
	}
	return buf.String()
}
