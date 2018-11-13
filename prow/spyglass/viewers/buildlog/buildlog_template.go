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
	margin: 0;
	line-height:1.2;
  color: #e8e8e8;
  background-color: #212121;
}
.loglines td {
  padding-left: 0;
}
.line-highlighted {
 color: rgba(255, 224, 0, 1.0);
}
.match-highlighted {
  color: rgba(255, 0, 0, 1.0);
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
  color: rgba(255,255,2552,0.6);
  text-align: right;
  vertical-align: top;
}
/* ansi colors from https://en.wikipedia.org/wiki/ANSI_escape_code#Colors */
.ansi-0 { color: #000000; }  /* Black */
.ansi-1 { color: #c23621; }  /* Red */
.ansi-2 { color: #25bc26; }  /* Green */
.ansi-3 { color: #adad27; }  /* Brown */
.ansi-4 { color: #492ee1; }  /* Blue */
.ansi-5 { color: #d338d3; }  /* Magenta */
.ansi-6 { color: #33bbc8; }  /* Cyan */
.ansi-7 { color: #cbcccd; }  /* Gray */
/* Bright */
.ansi-8 { color: #818383; }  /* Darkgray */
.ansi-9 { color: #fc391f; }  /* Red */
.ansi-10 { color: #31e722; }  /* Green */
.ansi-11 { color: #eaec23; }  /* Yellow */
.ansi-12 { color: #5833ff; }  /* Blue */
.ansi-13 { color: #f935f8; }  /* Magenta */
.ansi-14 { color: #14f0f0; }  /* Cyan */
.ansi-15 { color: #e9ebeb; }  /* White */
</style>
<div style="font-family:monospace;">
  {{range $log := .LogViews}}
  <div id="{{$log | logID}}">
    <h4 style="margin: 0;">
      <a href="{{$log.ArtifactLink}}">{{$log.ArtifactName}}<i class="material-icons" style="font-size: 1em; vertical-align: middle;">link</i></a>
      <button id="{{$log | logID}}-show-all" onclick="showAllLines({{$log | logID}})">Show all hidden lines</button>
    </h4>
    <table class="loglines">
    {{range $g := $log.LineGroups}}
      {{if $g.Skip}}
      <tbody class="show-skipped" id="{{$g | skipID}}">
        <tr>
          <td></td>
          <td><button onclick="showLines({{$log | logID}}, {{$g | linesID}}, {{$g | skipID}})">Show {{$g | linesSkipped}} hidden lines</button></td>
        </tr>
      </tbody>
      {{end}}
      <tbody {{if $g.Skip}}class="skipped"{{else}}class="shown"{{end}} id="{{$g | linesID}}">
        {{range $line := $g.LogLines}}
        <tr>
          <td class="linenum">{{$line.Number}}</td>
          <td>
            <span {{if $line.Highlighted}}class="line-highlighted"{{end}}>{{range $s := $line.SubLines}}<span {{if $s.Highlighted}}class="match-highlighted"{{end}}>{{$s.Text}}</span>{{end}}</span>
          </td>
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
