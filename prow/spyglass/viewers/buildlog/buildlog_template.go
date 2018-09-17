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

package buildlog

import (
	"html/template"
)

var buildLogTemplateText = `<style>
.loglines {
	list-style-type: none;
	padding: 0;
	margin:0;
	line-height:1.2;
	color:black;
}
.highlighted {
  background-color: rgba(255, 224, 0, .5);
}
.skipped {
  display: none;
}
td {
  padding: 2px;
}
tr {
  border: none;
}
.linenum {
  user-select: none;
  color: gray;
  text-align: right;
  vertical-align: top;
}
</style>
<div style="font-family:monospace;">
  {{range $log := .LogViews}}
  <div id="{{$log | logID}}">
    <h4 style="margin-top:0;">
      <a href="{{$log.ArtifactLink}}">{{$log.ArtifactName}}</a>
      <button id="{{$log | logID}}-show-all" onclick="showAllLines({{$log | logID}})">Show all hidden lines</button>
    </h4>
    <table class="loglines">
    {{range $g := $log.LineGroups}}
      {{if $g.Skip}}
      <tbody class="show-skipped" id="{{$g | skipID}}">
        <tr>
          <td></td>
          <td><button onclick="showLines({{$g | linesID}}, {{$g | skipID}})">Show {{$g | linesSkipped}} hidden lines</button></td>
        </tr>
      </tbody>
      {{end}}
      <tbody {{if $g.Skip}}class="skipped"{{end}} id="{{$g | linesID}}">
        {{range $line := $g.LogLines}}
        <tr>
          <td class="linenum">{{$line.Number}}</td>
          <td><span {{if $line.Highlighted}}class="highlighted"{{end}}>{{$line.Text}}</span></td>
        </tr>
        {{end}}
      </tbody>
    {{end}}
    </table>
  </div>
  {{end}}
</div>`
var buildLogTemplate = template.Must(template.New("build-log").
	Funcs(template.FuncMap{
		"linesSkipped": linesSkipped,
		"linesID":      linesID,
		"skipID":       skipID,
		"logID":        logID}).
	Parse(buildLogTemplateText))
