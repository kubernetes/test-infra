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

// Package buildlog provides a build log viewer for Spyglass
package buildlog

import (
	"bytes"
	"encoding/json"
	"html/template"
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/spyglass/viewers"
)

// An artifact viewer for build logs
type BuildLogViewer struct {
	ViewName  string
	ViewTitle string
}

// Name gets the unique name of the viewer within the job
func (v *BuildLogViewer) Name() string {
	return v.ViewName
}

// Title gets the title of the viewer
func (v *BuildLogViewer) Title() string {
	return v.ViewTitle
}

// View creates a view for a build log (or multiple build logs)
func (v *BuildLogViewer) View(artifacts []viewers.Artifact, raw *json.RawMessage) string {
	logViewTmpl := `
	<div style="font-family:monospace;">
	{{range .LogViews}}<ul style="list-style-type:none;padding:0;margin:0;line-height:1;">
		{{range $ix, $e := .LogLines}}
			<li>{{$e}}</li>
		{{end}}
	</ul>{{end}}
</div>`
	var buf bytes.Buffer
	type LogFileView struct {
		LogLines []string
	}
	type BuildLogsView struct {
		LogViews []LogFileView
	}
	var buildLogsView BuildLogsView
	for _, a := range artifacts {
		//logLines := LastNLines(a, 100)
		read, err := a.ReadAll()
		if err != nil {
			logrus.Error("Failed reading lines")
		}
		logLines := strings.Split(string(read), "\n")
		logrus.Info("loglines", logLines)
		buildLogsView.LogViews = append(buildLogsView.LogViews, LogFileView{LogLines: logLines})
	}
	t := template.Must(template.New("BuildLogView").Parse(logViewTmpl))
	err := t.Execute(&buf, buildLogsView)
	if err != nil {
		logrus.Errorf("Template failed with error: %s", err)
	}
	return buf.String()
}
