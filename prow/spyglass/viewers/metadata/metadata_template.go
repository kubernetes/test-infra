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

package metadata

const tmplt = `
<table class="mdl-data-table mdl-js-data-table mdl-shadow--2dp">
  <tbody>
  {{if eq .Finished.Result "SUCCESS"}}
  <tr style="background-color:#B2FF59;">
  {{else if eq .Finished.Result "FAILURE"}}
  <tr style="background-color:#FF6E40">
  {{else}}
    <tr>
  {{end}}
      <td class="mdl-data-table__cell--non-numeric">Result</td>
      <td class="mdl-data-table__cell--non-numeric">{{.Finished.Result}}<td>
    </tr>
    <tr>
      <td class="mdl-data-table__cell--non-numeric">Tests</td>
      <td class="mdl-data-table__cell--non-numeric">TODO</td>
    </tr>
    <tr>
      <td class="mdl-data-table__cell--non-numeric">Started</td>
      <td class="mdl-data-table__cell--non-numeric">{{.Started.Timestamp}}</td>
    </tr>
    <tr>
      <td class="mdl-data-table__cell--non-numeric">Elapsed</td>
      <td class="mdl-data-table__cell--non-numeric">{{.Derived.Elapsed}}</td>
    </tr>
    <tr>
      <td class="mdl-data-table__cell--non-numeric">Version</td>
      <td class="mdl-data-table__cell--non-numeric">{{.Finished.Version}}</td>
    </tr>
    <tr>
      <td class="mdl-data-table__cell--non-numeric">Node</td>
      <td class="mdl-data-table__cell--non-numeric">{{.Started.Node}}</td>
    </tr>
    <tr>
      <td class="mdl-data-table__cell--non-numeric">Job Version</td>
      <td class="mdl-data-table__cell--non-numeric">{{.Finished.JobVersion}}</td>
    </tr>
    {{range $k, $v := .Finished.Metadata}}<tr>
      <td class="mdl-data-table__cell--non-numeric">{{$k}}</td>
      <td class="mdl-data-table__cell--non-numeric">{{$v}}</td>
    </tr>{{end}}
  </tbody>
</table>`
