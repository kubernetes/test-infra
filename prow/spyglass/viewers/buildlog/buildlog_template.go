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
<div style="font-family:monospace;">
  {{range .LogViews}}<h4><a href="{{.ArtifactLink}}">{{.ArtifactName}}</a> - {{.ViewMethodDescription}}</h4>
  <ul style="list-style-type:none;padding:0;margin:0;line-height:1.4;color:black;">
    {{range $ix, $e := .LogLines}}
    <li>{{$e}}</li>
    {{end}}
  </ul>
  <button onclick="refreshView({{.ViewName}}, '{{index $.RawRefreshRequests .ArtifactName}}')" class="mdl-button mdl-js-button mdl-button--primary">More Lines Please</button>
  {{end}}
</div>`
