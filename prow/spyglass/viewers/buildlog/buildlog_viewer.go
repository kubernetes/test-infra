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
	"fmt"
	"html/template"
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/spyglass/viewers"
)

const (
	name  = "BuildLogViewer"
	title = "Build Log"
)

func init() {
	viewers.RegisterViewer(name, title, ViewHandler)
}

// ViewHandler creates a view for a build log (or multiple build logs)
func ViewHandler(artifacts []viewers.Artifact, raw *json.RawMessage) string {
	logViewTmpl := `
	<div style="font-family:monospace;">
	{{range .LogViews}}<h4>{{.LogName}}</h4>
	<ul style="list-style-type:none;padding:0;margin:0;line-height:1.4;color:black;">
		{{range $ix, $e := .LogLines}}
			<li>{{$e}}</li>
		{{end}}
	</ul>{{end}}
</div>`
	var buf bytes.Buffer
	type LogFileView struct {
		LogName  string
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
			logrus.WithError(err).Error("Failed reading lines")
		}
		if string(read) != "" {
			logLines := strings.Split(string(read), "\n")
			for ix, line := range logLines {
				line = fmt.Sprintf("%d\t%s", ix, line)
			}
			buildLogsView.LogViews = append(buildLogsView.LogViews, LogFileView{LogName: a.JobPath(), LogLines: logLines})
		}
	}
	t := template.Must(template.New(name).Parse(logViewTmpl))
	err := t.Execute(&buf, buildLogsView)
	if err != nil {
		logrus.WithError(err).Error("Template failed.")
	}
	return buf.String()
}
