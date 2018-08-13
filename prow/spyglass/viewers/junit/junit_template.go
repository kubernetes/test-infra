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
  {{if gt $numF 0}}
  <div id="failed-container">
    <table id="failed-table" class="mdl-data-table mdl-js-data-table mdl-shadow--2dp">
      <thead id="failed-theader" onclick="var failedTBody = document.getElementById('failed-tbody');
		failedTBody.classList.toggle('hidden');
		if (failedTBody.classList.contains('hidden')) {
			document.getElementById('failed-expander').innerHTML = 'expand_more';
		}
		else {
			document.getElementById('failed-expander').innerHTML = 'expand_less';
		}">
		<tr style="background-color:#FF0000;color:white;font-weight:bold;font-size:1.5em;">
          <td class="mdl-data-table__cell--non-numeric"><h6>{{len .Failed}}/{{.NumTests}} Tests Failed.</h6></td>
	  <td colspan="3">&nbsp;</td>
          <td class="mdl-data-table__cell">
            <i id="failed-expander" class="icon-button material-icons arrow-icon">expand_less</i>
          </td>
	</tr>
      </thead>
      <tbody id="failed-tbody">
      {{range .Failed}}
        <tr>
          <td class="mdl-data-table__cell--non-numeric">{{.Junit.Name}}</td>
          <td class="mdl-data-table__cell--non-numeric">{{.Junit.Status}}</td>
          <td class="mdl-data-table__cell--non-numeric">{{.Junit.Duration}}</td>
          <td class="mdl-data-table__cell--non-numeric">{{.Junit.Error.Message}}</td>
          <td class="mdl-data-table__cell--non-numeric"><a href="{{.Link}}">Link</a></td>
        </tr>
      {{end}}
      </tbody>
    </table>
  </div>
  {{end}}
  <div id="passed-container">
    <table id="passed-table" class="mdl-data-table mdl-js-data-table mdl-shadow--2dp">
      <thead id="passed-theader" onclick="var passedTBody = document.getElementById('passed-tbody');
		passedTBody.classList.toggle('hidden');
		if (passedTBody.classList.contains('hidden')) {
			document.getElementById('passed-expander').innerHTML = 'expand_more';
		}
		else {
			document.getElementById('passed-expander').innerHTML = 'expand_less';
		}">
		<tr style="background-color:#00FF00;color:black;font-weight:bold;font-size:1.5em;">
          <td class="mdl-data-table__cell--non-numeric"><h6>{{len .Passed}}/{{.NumTests}} Tests Passed!</h6></td>
	  <td colspan="2">&nbsp;</td>
          <td class="mdl-data-table__cell">
            <i id="passed-expander" class="icon-button material-icons arrow-icon">expand_more</i>
          </td>
	</tr>
      </thead>
      <tbody id="passed-tbody" class="hidden">
      {{range .Passed}}
        <tr>
          <td class="mdl-data-table__cell--non-numeric">{{.Junit.Name}}</td>
          <td class="mdl-data-table__cell--non-numeric">{{.Junit.Status}}</td>
          <td class="mdl-data-table__cell--non-numeric">{{.Junit.Duration}}</td>
          <td class="mdl-data-table__cell--non-numeric"><a href="{{.Link}}">Link</a></td>
        </tr>
      {{end}}
      </tbody>
    </table>
  </div>
  {{if gt $numS 0}}
  <div id="skipped-container">
    <table id="skipped-table" class="mdl-data-table mdl-js-data-table mdl-shadow--2dp">
      <thead id="skipped-theader" onclick="var skippedTBody = document.getElementById('skipped-tbody');
		skippedTBody.classList.toggle('hidden');
		if (skippedTBody.classList.contains('hidden')) {
			document.getElementById('skipped-expander').innerHTML = 'expand_more';
		}
		else {
			document.getElementById('skipped-expander').innerHTML = 'expand_less';
		}">
        <tr style="font-weight:bold;font-size:1.5em;">
          <td class="mdl-data-table__cell--non-numeric"><h6>{{len .Skipped}}/{{.NumTests}} Tests Skipped.</h6></td>
	  <td colspan="2">&nbsp;</td>
          <td class="mdl-data-table__cell">
            <i id="skipped-expander" class="icon-button material-icons arrow-icon">expand_more</i>
          </td>
	</tr>
      </thead>
      <tbody id="skipped-tbody" class="hidden">
      {{range .Skipped}}
        <tr>
          <td class="mdl-data-table__cell--non-numeric">{{.Junit.Name}}</td>
          <td class="mdl-data-table__cell--non-numeric">{{.Junit.Status}}</td>
          <td class="mdl-data-table__cell--non-numeric">{{.Junit.Duration}}</td>
          <td class="mdl-data-table__cell--non-numeric"><a href="{{.Link}}">Link</a></td>
        </tr>
      {{end}}
      </tbody>
    </table>
  </div>
  {{end}}
</div>`

var junitTemplate = template.Must(template.New("junit").Parse(junitTemplateText))
