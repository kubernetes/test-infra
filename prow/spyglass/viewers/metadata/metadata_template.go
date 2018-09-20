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

import (
	"html/template"
)

var metadataTemplateText = `{{$passed := eq .Finished.Result "SUCCESS"}}
{{$failed := eq .Finished.Result "FAILURE" "FAILED"}}
<style>
.test-row {
	font-weight: bold;
	font-size: 1.1em;
}
.failed-row {
	background-color: #FF0000;
	color: white;
}
.passed-row {
	background-color:#00FF00;
	color: black;
}
.mdl-data-table .metadata-header th {
	font-size: 1.2em;
}
.metadata-table {
	height: unset;
	padding: 0;
	margin: 0;
}
</style>
<div class="mdl-grid">
  <div class="mdl-cell mdl-cell--6-col">
    <table class="mdl-data-table mdl-js-data-table mdl-shadow--2dp metadata-table">
      <thead class="metadata-header">
        <tr>
	  <th class="mdl-data-table__cell--non-numeric">Prow Metadata</td>
	  <th class="mdl-data-table__cell--non-numeric">&nbsp</td>
	</tr>
      </thead>
      <tbody>
        {{if $passed}}
        <tr class="test-row passed-row">
        {{else if $failed}}
        <tr class="test-row failed-row">
        {{else}}
        <tr class="test-row">
        {{end}}
          <td class="mdl-data-table__cell--non-numeric">Status</td>
          <td class="mdl-data-table__cell--non-numeric">{{.Derived.Status}}</td>
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
          <td class="mdl-data-table__cell--non-numeric">Node</td>
          <td class="mdl-data-table__cell--non-numeric">{{.Started.Node}}</td>
        </tr>
      </tbody>
    </table>
  </div>
  {{if .Derived.Done}}
  <div class="mdl-cell mdl-cell--6-col">
    <table class="mdl-data-table mdl-js-data-table mdl-shadow--2dp" style="height:unset;">
      <thead class="metadata-header">
        <tr>
	  <th class="mdl-data-table__cell--non-numeric">Job-Provided Metadata</td>
	  <th class="mdl-data-table__cell--non-numeric">&nbsp</td>
	</tr>
      </thead>
      <tbody>
	{{range $k, $v := .Finished.Metadata}}
	<tr>
          <td class="mdl-data-table__cell--non-numeric">{{$k}}</td>
          <td class="mdl-data-table__cell--non-numeric">{{$v}}</td>
        </tr>{{end}}
      </tbody>
    </table>
  </div>
  {{end}}
</div>`

var metadataTemplate = template.Must(template.New("metadata").Parse(metadataTemplateText))
