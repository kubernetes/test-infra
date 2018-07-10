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

	"html/template"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/spyglass/viewers"
)

const (
	name     = "MetadataViewer"
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
	TimestampString string            `json:"timestamp"`
	Timestamp       time.Time         `json:"-"`
	Repos           map[string]string `json:"repos"`
	Pull            string            `json:"pull"`
}

// Finished is used to mirror the finished.json artifact
type Finished struct {
	TimestampString string    `json:"timestamp"`
	Timestamp       time.Time `json:"-"`
	Result          string    `json:"result"`
	Metadata        string    `json:"metadata"`
}

// ViewHandler creates a view for prow job metadata
func ViewHandler(artifacts []viewers.Artifact, raw string) string {
	metadataViewTmpl := `
	<div>
	{{ .Started}}
	</div>
	<div>
	{{ .Finished}}
	</div>
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
