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

package main

import (
	"bytes"
	"html/template"

	"path/filepath"
	"sort"
	"strings"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

var svg = `<svg xmlns="http://www.w3.org/2000/svg" width="{{.Width}}" height="20">
<linearGradient id="a" x2="0" y2="100%">
  <stop offset="0" stop-color="#bbb" stop-opacity=".1"/>
  <stop offset="1" stop-opacity=".1"/>
</linearGradient>
<rect rx="3" width="100%" height="20" fill="#555"/>
<g fill="{{.Color}}">
  <rect rx="3" x="{{.RightStart}}" width="{{.RightWidth}}" height="20"/>
  <path d="M{{.RightStart}} 0h4v20h-4z"/>
</g>
<rect rx="3" width="100%" height="20" fill="url(#a)"/>
<g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="11">
<g fill="#010101" opacity=".3">
<text x="{{.XposLeft}}" y="15">{{.Subject}}</text>
<text x="{{.XposRight}}" y="15">{{.Status}}</text>
</g>
<text x="{{.XposLeft}}" y="14">{{.Subject}}</text>
<text x="{{.XposRight}}" y="14">{{.Status}}</text>
</g>
</svg>`

var svgTemplate = template.Must(template.New("svg").Parse(svg))

// Make a small SVG badge that looks like `[subject | status]`, with the status
// text in the given color. Provides a local version of the shields.io service.
func makeShield(subject, status, color string) []byte {
	// TODO(rmmh): Use better font-size metrics for prettier badges-- estimating
	// character widths as 6px isn't very accurate.
	// See also: https://github.com/badges/shields/blob/master/measure-text.js
	p := struct {
		Width, RightStart, RightWidth int
		XposLeft, XposRight           float64
		Subject, Status               string
		Color                         string
	}{
		Subject:    subject,
		Status:     status,
		RightStart: 13 + 6*len(subject),
		RightWidth: 13 + 6*len(status),
	}
	p.Width = p.RightStart + p.RightWidth
	p.XposLeft = float64(p.RightStart) * 0.5
	p.XposRight = float64(p.RightStart) + float64(p.RightWidth-2)*0.5
	switch color {
	case "brightgreen":
		p.Color = "#4c1"
	case "red":
		p.Color = "#e05d44"
	default:
		p.Color = color
	}
	var buf bytes.Buffer
	svgTemplate.Execute(&buf, p)
	return buf.Bytes()
}

// pickLatestJobs returns the most recent run of each job matching the selector,
// which is comma-separated list of globs, for example "ci-ti-*,ci-other".
// jobs will be sorted by StartTime in reverse order to display recent jobs first
func pickLatestJobs(jobs []prowapi.ProwJob, selector string) []prowapi.ProwJob {
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[j].Status.StartTime.Before(&jobs[i].Status.StartTime)
	})
	var out []prowapi.ProwJob
	have := make(map[string]bool)
	want := strings.Split(selector, ",")
	for _, job := range jobs {
		if have[job.Spec.Job] {
			continue // already have the latest result for this job
		}
		for _, pat := range want {
			if match, _ := filepath.Match(pat, job.Spec.Job); match {
				have[job.Spec.Job] = true
				out = append(out, job)
				break
			}
		}
	}
	return out
}

func renderBadge(jobs []prowapi.ProwJob) (string, string, []byte) {
	color := "brightgreen"
	status := "passing"
	if len(jobs) == 0 {
		color = "darkgrey"
		status = "no results"
	} else {
		failedJobs := []string{}
		for _, job := range jobs {
			if job.Status.State == "failure" {
				failedJobs = append(failedJobs, job.Spec.Job)
			}
		}
		sort.Strings(failedJobs)
		if len(failedJobs) > 3 {
			failedJobs = append(failedJobs[:3], "...")
		}
		if len(failedJobs) > 0 {
			color = "red"
			status = "failing " + strings.Join(failedJobs, ", ")
		}
	}

	return status, color, makeShield("build", status, color)
}
