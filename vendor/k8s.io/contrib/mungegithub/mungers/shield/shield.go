/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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

package shield

// shield provides a local version of the shields.io service.

import (
	"bytes"
	"html/template"
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
// text in the given color.
func Make(subject, status, color string) []byte {
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
		panic("Invalid color " + color)
	}
	var buf bytes.Buffer
	svgTemplate.Execute(&buf, p)
	return buf.Bytes()
}
