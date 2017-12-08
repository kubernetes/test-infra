/*
Copyright 2017 The Kubernetes Authors.

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

package mungers

import (
	"regexp"
	"strings"
	"testing"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
	"k8s.io/test-infra/mungegithub/github"
)

type testLogFinder string

func (lf testLogFinder) FoundLog(branch, logString string, regexSearch bool) (bool, string) {
	if branch != "branch" {
		glog.Fatalf("Expected branch name \"branch\", got %q.", branch)
	}
	if regexSearch {
		if match := regexp.MustCompile("(?m)" + logString).FindString(string(lf)); match != "" {
			return true, string(lf)
		}
	} else {
		if strings.Contains(string(lf), logString) {
			return true, string(lf)
		}
	}
	return false, ""
}

func TestFoundByScript(t *testing.T) {
	sampleLogs := `ab4109707b03616094b8ccfb9697c19bff9a4149
Merge pull request #3570 from spxtr/logconsistently
Make logging in splice and tot consistent with the rest.
8fd41ffcf14a8621d53bdd8282835073a85beec5
Merge pull request #3594 from krzyzacy/kops-url-refix
append JOB_NAME to kops build path
1872c306b68919592b21ca8984f4bc603e7b2e2a
Make logging in splice and tot consistent with the rest.

0d07f5a827c575b9f6a1c4a3c28cf02a7003e944
Forgot append JOB_NAME to kops build path

c8cf39417fc81c522205c0a061ceac42285e44a3
Merge pull request #48791 from luxas/automated-cherry-pick-of-#48594-#48538-upstream-release-1.7
Automatic merge from submit-queue

Automated cherry pick of #48594 #48538

Cherry pick of #48594 #48538 on release-1.7.

#48594: Add node-name flag to ` + "`init`" + ` phase
#48538: Add node-name flag to ` + "`join`" + ` phase
753266cb7d77456c2395521bece25eca51bfedcc
Merge pull request #3592 from shyamjvs/enable-logexporter
Enable logexporter for gce-scale tests
1ccb815bcdfc02ba972abc80dd125cdc50fe8037
Enable logexporter for gce-scale tests
`
	c := &ClearPickAfterMerge{logs: testLogFinder(sampleLogs)}

	tests := []struct {
		num     int
		matches bool
	}{
		{48538, true},
		{77, false},
		{48594, true},
		{3570, false},
		{3594, false},
		{48791, false},
	}
	for _, test := range tests {
		if test.matches != c.foundByScript(&github.MungeObject{Issue: &githubapi.Issue{Number: &test.num}}, "branch") {
			var not string
			if !test.matches {
				not = "not "
			}
			t.Errorf("Error: Expected PR #%d to %sbe found in logs!", test.num, not)
		}
	}
}
