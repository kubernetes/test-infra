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
	"k8s.io/test-infra/bazel-test-infra/external/go_sdk/src/html/template"
	"k8s.io/test-infra/prow/spyglass/viewers"
)

// An artifact viewer for build logs
type MetadataViewer struct {
	ViewName  string
	ViewTitle string
}

// Started is used to mirror the started.json artifact
type Started struct {
	TimestampString string `json:"timestamp"`
	Timestamp       time.Time
	Repos           map[string]string `json:"repos"`
	Pull            string            `json:"pull"`
}

// Finished is used to mirror the finished.json artifact
type Finished struct {
	TimestampString string `json:"timestamp"`
	Timestamp       time.Time
	Result          string `json:"result"`
	Metadata        string `json:"metadata"`
}

// View creates a view for prow job metadata
func (v *MetadataViewer) View(artifacts []viewers.Artifact, raw *json.RawMessage) string {
	metadataViewTmpl := `
	<div>
	{{ .Started}}
	</div>
	{{ .Finished}}
	`
	var buf bytes.Buffer
	type MetadataView struct {
		Started  Started
		Finished Finished
	}
	var metadataView MetadataView
	for _, a := range artifacts {
		read, err := a.ReadAll()
		if err != nil {
			logrus.Error("Failed reading file")
		}
		if a.JobPath() == "started.json" {
			s := Started{}
			json.Unmarshal(read, s)
			metadataView.Started = s

		} else if a.JobPath() == "finished.json" {
			f := Finished{}
			json.Unmarshal(read, f)
			metadataView.Finished = f
		}

	}
	t := template.Must(template.New("MetadataView").Parse(metadataViewTmpl))
	err := t.Execute(&buf, metadataView)
	if err != nil {
		logrus.Errorf("Template failed with error: %s", err)
	}
	return buf.String()
}

// Title gets the title of the viewer
func (v *MetadataViewer) Title() string {
	return v.ViewTitle
}

// Name gets the unique name of the viewer within the job
func (v *MetadataViewer) Name() string {
	return v.ViewName
}
