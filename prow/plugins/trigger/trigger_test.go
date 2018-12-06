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

package trigger

import (
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/prow/config"
)

func TestShouldRun(t *testing.T) {
	var testCases = []struct {
		name             string
		fileChanges      []string
		fileError        error
		job              config.Presubmit
		ref              string
		forceRunContexts sets.String
		body             string
		expectedRun      bool
		expectedErr      bool
	}{
		{
			name: "job skipped on the branch won't run",
			job: config.Presubmit{
				Brancher: config.Brancher{
					SkipBranches: []string{"master"},
				},
			},
			ref:         "master",
			expectedRun: false,
		},
		{
			name: "job running only on other branches won't run",
			job: config.Presubmit{
				Brancher: config.Brancher{
					Branches: []string{"master"},
				},
			},
			ref:         "release-1.11",
			expectedRun: false,
		},
		{
			name: "job with always_run: true should run",
			job: config.Presubmit{
				AlwaysRun: true,
			},
			ref:         "master",
			expectedRun: true,
		},
		{
			name: "job in the force run contexts should run",
			job: config.Presubmit{
				Context: "context",
			},
			ref:              "master",
			forceRunContexts: sets.NewString("context"),
			expectedRun:      true,
		},
		{
			name: "job with body matching trigger should run",
			job: config.Presubmit{
				Trigger:      `(?m)^/test (?:.*? )?foo(?: .*?)?$`,
				RerunCommand: "/test foo",
			},
			ref:         "master",
			body:        "/test foo",
			expectedRun: true,
		},
		{
			name: "job with always_run: false and no run_if_changed should not run",
			job: config.Presubmit{
				AlwaysRun:    false,
				Trigger:      `(?m)^/test (?:.*? )?foo(?: .*?)?$`,
				RerunCommand: "/test foo",
				RegexpChangeMatcher: config.RegexpChangeMatcher{
					RunIfChanged: "",
				},
			},
			ref:         "master",
			expectedRun: false,
		},
		{
			name: "job with run_if_changed but file get errors should not run",
			job: config.Presubmit{
				Trigger:      `(?m)^/test (?:.*? )?foo(?: .*?)?$`,
				RerunCommand: "/test foo",
				RegexpChangeMatcher: config.RegexpChangeMatcher{
					RunIfChanged: "file",
				},
			},
			ref:         "master",
			fileError:   errors.New("oops"),
			expectedRun: false,
			expectedErr: true,
		},
		{
			name: "job with run_if_changed not matching should not run",
			job: config.Presubmit{
				Trigger:      `(?m)^/test (?:.*? )?foo(?: .*?)?$`,
				RerunCommand: "/test foo",
				RegexpChangeMatcher: config.RegexpChangeMatcher{
					RunIfChanged: "^file$",
				},
			},
			ref:         "master",
			fileChanges: []string{"something"},
			expectedRun: false,
		},
		{
			name: "job with run_if_changed matching should run",
			job: config.Presubmit{
				Trigger:      `(?m)^/test (?:.*? )?foo(?: .*?)?$`,
				RerunCommand: "/test foo",
				RegexpChangeMatcher: config.RegexpChangeMatcher{
					RunIfChanged: "^file$",
				},
			},
			ref:         "master",
			fileChanges: []string{"file"},
			expectedRun: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			jobs := []config.Presubmit{testCase.job}
			if err := config.SetPresubmitRegexes(jobs); err != nil {
				t.Fatalf("%s: failed to set presubmit regexes: %v", testCase.name, err)
			}
			jobShouldRun, err := shouldRun(func() ([]string, error) {
				return testCase.fileChanges, testCase.fileError
			}, jobs[0], testCase.ref, testCase.forceRunContexts, testCase.body)

			if err == nil && testCase.expectedErr {
				t.Errorf("%s: expected an error and got none", testCase.name)
			}
			if err != nil && !testCase.expectedErr {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
			}
			if jobShouldRun != testCase.expectedRun {
				t.Errorf("%s: did not determine if job should run correctly, expected %v but got %v", testCase.name, testCase.expectedRun, jobShouldRun)
			}
		})
	}
}
