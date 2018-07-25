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
	"fmt"
	"time"

	"html/template"

	"github.com/sirupsen/logrus"
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

// Started is used to mirror the started.json artifact
type Started struct {
	TimestampRaw int64             `json:"timestamp"`
	Timestamp    time.Time         `json:"-"`
	Node         string            `json:"node"`
	Repos        map[string]string `json:"repos"`
	Pull         string            `json:"pull"`
}

// Finished is used to mirror the finished.json artifact
type Finished struct {
	TimestampRaw int64             `json:"timestamp"`
	Timestamp    time.Time         `json:"-"`
	Version      string            `json:"version"`
	JobVersion   string            `json:"job-version"`
	Passed       bool              `json:"passed"`
	Result       string            `json:"result"`
	Metadata     map[string]string `json:"metadata"`
}

// Derived contains metadata derived from provided metadata for insertion into the template
type Derived struct {
	Elapsed          time.Duration
	GCSArtifactsLink string
}

// ViewHandler creates a view for prow job metadata
// TODO: os image,
func ViewHandler(artifacts []viewers.Artifact, raw string) string {
	var buf bytes.Buffer
	type MetadataViewData struct {
		Started  Started
		Finished Finished
		Derived  Derived
	}
	var metadataViewData MetadataViewData
	for _, a := range artifacts {
		read, err := a.ReadAll()
		if err != nil {
			logrus.WithError(err).Error("Failed reading from artifact.")
		}
		if a.JobPath() == "started.json" {
			s := Started{}
			if err = json.Unmarshal(read, &s); err != nil {
				logrus.WithError(err).Error("Error unmarshaling started.json")
			}
			s.Timestamp = time.Unix(s.TimestampRaw, 0)
			metadataViewData.Started = s

		} else if a.JobPath() == "finished.json" {
			f := Finished{}
			if err = json.Unmarshal(read, &f); err != nil {
				logrus.WithError(err).Error("Error unmarshaling finished.json")
			}
			f.Timestamp = time.Unix(f.TimestampRaw, 0)
			metadataViewData.Finished = f
		}

	}
	d := Derived{
		Elapsed: metadataViewData.Finished.Timestamp.Sub(metadataViewData.Started.Timestamp),
	}
	metadataViewData.Derived = d
	t := template.Must(template.New(fmt.Sprintf("%sTemplate", name)).Parse(tmplt))
	if err := t.Execute(&buf, metadataViewData); err != nil {
		logrus.WithError(err).Error("Error executing template.")
	}
	return buf.String()
}
