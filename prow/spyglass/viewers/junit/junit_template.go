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

package junit

import (
	"html/template"
)

var junitTemplateText = `{{$numF := len .Failed}}
{{$numP := len .Passed}}
{{$numS := len .Skipped}}
<div id="junit-container">
  <style>
  .hidden-tests {
    visibility: collapse;
  }
  </style>
  <table id="junit-table" class="mdl-data-table mdl-js-data-table mdl-shadow--2dp" style="white-space: pre-wrap;">
  {{if gt $numF 0}}
      <thead id="failed-theader" onclick="toggleExpansion('failed-tbody', 'failed-expander')">
		<tr style="background-color:#FF0000;color:white;font-weight:bold;font-size:1.5em;">
          <td class="mdl-data-table__cell--non-numeric" colspan="4"><h6>{{len .Failed}}/{{.NumTests}} Tests Failed.</h6></td>
	  <td>&nbsp;</td>
          <td class="mdl-data-table__cell">
            <i id="failed-expander" class="icon-button material-icons arrow-icon">expand_less</i>
          </td>
	</tr>
      </thead>
      <tbody id="failed-tbody">
      {{range $ix, $test := .Failed}}
        <tr onclick="toggleExpansion('failed-test-body-{{$ix}}', 'failed-test-expander-{{$ix}}')">
          <td class="mdl-data-table__cell--non-numeric">{{$test.Junit.Name}}</td>
          <td class="mdl-data-table__cell--non-numeric">{{$test.Junit.Status}}</td>
          <td class="mdl-data-table__cell--non-numeric">{{$test.Junit.Duration}}</td>
          <td class="mdl-data-table__cell--non-numeric"><a href="{{$test.Link}}">Link</a></td>
          <td class="mdl-data-table__cell--non-numeric">{{$test.Junit.Error.Message}}</td>
          <td class="mdl-data-table__cell--non-numeric">{{$test.Junit.Error.Body}}</td>
        </tr>
      {{end}}
      </tbody>
  {{end}}
      <thead id="passed-theader" onclick="toggleExpansion('passed-tbody', 'passed-expander')">
		<tr style="background-color:#00FF00;color:black;font-weight:bold;font-size:1.5em;">
          <td class="mdl-data-table__cell--non-numeric" colspan="4"><h6>{{len .Passed}}/{{.NumTests}} Tests Passed!</h6></td>
	  <td>&nbsp;</td>
          <td class="mdl-data-table__cell">
            <i id="passed-expander" class="icon-button material-icons arrow-icon">expand_more</i>
          </td>
	</tr>
      </thead>
      <tbody id="passed-tbody" class="hidden-tests">
      {{range .Passed}}
        <tr>
          <td class="mdl-data-table__cell--non-numeric">{{.Junit.Name}}</td>
          <td class="mdl-data-table__cell--non-numeric">{{.Junit.Status}}</td>
          <td class="mdl-data-table__cell--non-numeric">{{.Junit.Duration}}</td>
          <td class="mdl-data-table__cell--non-numeric"><a href="{{.Link}}">Link</a></td>
          <td class="mdl-data-table__cell--non-numeric"></td>
          <td class="mdl-data-table__cell--non-numeric"></td>
        </tr>
      {{end}}
      </tbody>
  {{if gt $numS 0}}
      <thead id="skipped-theader" onclick="toggleExpansion('skipped-tbody', 'skipped-expander')">
        <tr style="font-weight:bold;font-size:1.5em;">
          <td class="mdl-data-table__cell--non-numeric" colspan="4"><h6>{{len .Skipped}}/{{.NumTests}} Tests Skipped.</h6></td>
	  <td>&nbsp;</td>
          <td class="mdl-data-table__cell">
            <i id="skipped-expander" class="icon-button material-icons arrow-icon">expand_more</i>
          </td>
	</tr>
      </thead>
      <tbody id="skipped-tbody" class="hidden-tests">
      {{range .Skipped}}
        <tr>
          <td class="mdl-data-table__cell--non-numeric">{{.Junit.Name}}</td>
          <td class="mdl-data-table__cell--non-numeric">{{.Junit.Status}}</td>
          <td class="mdl-data-table__cell--non-numeric">{{.Junit.Duration}}</td>
          <td class="mdl-data-table__cell--non-numeric"><a href="{{.Link}}">Link</a></td>
          <td class="mdl-data-table__cell--non-numeric"></td>
          <td class="mdl-data-table__cell--non-numeric"></td>
        </tr>
      {{end}}
      </tbody>
  {{end}}
  </table>
</div>`

var junitTemplate = template.Must(template.New("junit").Parse(junitTemplateText))
