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

const tmplt = `
<style>
.loglines {
	list-style-type: none;
	padding: 0;
	margin:0;
	line-height:1.4;
	color:black;
}
</style>
<div style="font-family:monospace;">
{{range .LogViews}}<h4 style="margin-top:0;"><a href="{{.ArtifactLink}}">{{.ArtifactName}}</a> - {{.ViewMethodDescription}}</h4>
  <ul class="loglines">
    {{range $ix, $e := .LogLines}}
    <li >{{$e}}</li>
    {{end}}
  </ul>
  {{if not .ViewAll}}
  <button onclick="refreshView({{.ViewName}}, '{{index $.RawGetMoreRequests .ArtifactName}}')" class="mdl-button mdl-js-button mdl-button--primary">More Lines Please</button>
  <button onclick="refreshView({{.ViewName}}, '{{index $.RawGetAllRequests .ArtifactName}}')" class="mdl-button mdl-js-button mdl-button--primary">More Lines Please<span style="font-family:monospace;"> -A --force</span></button>
  {{end}}
  {{end}}
</div>`
