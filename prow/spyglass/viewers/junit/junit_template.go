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

const tmplt = `
<div class="failed-container">
  <div class="failed">
    <i class="icon-button material-icons status-icon failed"></>
    <h6>{{len .Failed}}/{{.NumTests}} Tests Failed.</h6>
  </div>
  <ul class="mdl-list">
  {{range .Failed}}
    <li class="mdl-list__item">{{.TestName}}</li>
  {{end}}
  </ul>
</div>
<div class="passed-container">
  <div class="passed">
    <i class="icon-button material-icons status-icon passed"></>
    <h6>{{len .Passed}}/{{.NumTests}} Tests Passed.</h6>
  </div>
  <ul class="mdl-list">
  {{range .Passed}}
    <li class="mdl-list__item">{{.TestName}}</li>
  {{end}}
  </ul>
</div>
<script>

</script>
`
