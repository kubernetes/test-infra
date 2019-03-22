/*
Copyright 2019 The Kubernetes Authors.

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

package pjutil

import (
	"errors"
	"reflect"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/test-infra/prow/config"
)

func TestTestAllFilter(t *testing.T) {
	var testCases = []struct {
		name       string
		presubmits []config.Presubmit
		expected   [][]bool
	}{
		{
			name: "test all filter matches jobs which do not require human triggering",
			presubmits: []config.Presubmit{
				{
					JobBase: config.JobBase{
						Name: "always-runs",
					},
					AlwaysRun: true,
				},
				{
					JobBase: config.JobBase{
						Name: "runs-if-changed",
					},
					AlwaysRun: false,
					RegexpChangeMatcher: config.RegexpChangeMatcher{
						RunIfChanged: "sometimes",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "runs-if-triggered",
					},
					Reporter: config.Reporter{
						Context: "runs-if-triggered",
					},
					Trigger:      `(?m)^/test (?:.*? )?trigger(?: .*?)?$`,
					RerunCommand: "/test trigger",
				},
				{
					JobBase: config.JobBase{
						Name: "literal-test-all-trigger",
					},
					Reporter: config.Reporter{
						Context: "runs-if-triggered",
					},
					Trigger:      `(?m)^/test (?:.*? )?all(?: .*?)?$`,
					RerunCommand: "/test all",
				},
			},
			expected: [][]bool{{true, false, false}, {true, false, false}, {false, false, false}, {false, false, false}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if len(testCase.presubmits) != len(testCase.expected) {
				t.Fatalf("%s: have %d presubmits but only %d expected filter outputs", testCase.name, len(testCase.presubmits), len(testCase.expected))
			}
			if err := config.SetPresubmitRegexes(testCase.presubmits); err != nil {
				t.Fatalf("%s: could not set presubmit regexes: %v", testCase.name, err)
			}
			filter := TestAllFilter()
			for i, presubmit := range testCase.presubmits {
				actualFiltered, actualForced, actualDefault := filter(presubmit)
				expectedFiltered, expectedForced, expectedDefault := testCase.expected[i][0], testCase.expected[i][1], testCase.expected[i][2]
				if actualFiltered != expectedFiltered {
					t.Errorf("%s: filter did not evaluate correctly, expected %v but got %v for %v", testCase.name, expectedFiltered, actualFiltered, presubmit.Name)
				}
				if actualForced != expectedForced {
					t.Errorf("%s: filter did not determine forced correctly, expected %v but got %v for %v", testCase.name, expectedForced, actualForced, presubmit.Name)
				}
				if actualDefault != expectedDefault {
					t.Errorf("%s: filter did not determine default correctly, expected %v but got %v for %v", testCase.name, expectedDefault, actualDefault, presubmit.Name)
				}
			}
		})
	}
}

func TestCommandFilter(t *testing.T) {
	var testCases = []struct {
		name       string
		body       string
		presubmits []config.Presubmit
		expected   [][]bool
	}{
		{
			name: "command filter matches jobs whose triggers match the body",
			body: "/test trigger",
			presubmits: []config.Presubmit{
				{
					JobBase: config.JobBase{
						Name: "trigger",
					},
					Trigger:      `(?m)^/test (?:.*? )?trigger(?: .*?)?$`,
					RerunCommand: "/test trigger",
				},
				{
					JobBase: config.JobBase{
						Name: "other-trigger",
					},
					Trigger:      `(?m)^/test (?:.*? )?other-trigger(?: .*?)?$`,
					RerunCommand: "/test other-trigger",
				},
			},
			expected: [][]bool{{true, true, true}, {false, false, true}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if len(testCase.presubmits) != len(testCase.expected) {
				t.Fatalf("%s: have %d presubmits but only %d expected filter outputs", testCase.name, len(testCase.presubmits), len(testCase.expected))
			}
			if err := config.SetPresubmitRegexes(testCase.presubmits); err != nil {
				t.Fatalf("%s: could not set presubmit regexes: %v", testCase.name, err)
			}
			filter := CommandFilter(testCase.body)
			for i, presubmit := range testCase.presubmits {
				actualFiltered, actualForced, actualDefault := filter(presubmit)
				expectedFiltered, expectedForced, expectedDefault := testCase.expected[i][0], testCase.expected[i][1], testCase.expected[i][2]
				if actualFiltered != expectedFiltered {
					t.Errorf("%s: filter did not evaluate correctly, expected %v but got %v for %v", testCase.name, expectedFiltered, actualFiltered, presubmit.Name)
				}
				if actualForced != expectedForced {
					t.Errorf("%s: filter did not determine forced correctly, expected %v but got %v for %v", testCase.name, expectedForced, actualForced, presubmit.Name)
				}
				if actualDefault != expectedDefault {
					t.Errorf("%s: filter did not determine default correctly, expected %v but got %v for %v", testCase.name, expectedDefault, actualDefault, presubmit.Name)
				}
			}
		})
	}
}

func fakeChangedFilesProvider(shouldError bool) config.ChangedFilesProvider {
	return func() ([]string, error) {
		if shouldError {
			return nil, errors.New("error getting changes")
		}
		return nil, nil
	}
}

func TestFilterPresubmits(t *testing.T) {
	var testCases = []struct {
		name                              string
		filter                            Filter
		presubmits                        []config.Presubmit
		changesError                      bool
		expectedToTrigger, expectedToSkip []config.Presubmit
		expectErr                         bool
	}{
		{
			name: "nothing matches, nothing to run or skip",
			filter: func(p config.Presubmit) (shouldRun bool, forcedToRun bool, defaultBehavior bool) {
				return false, false, false
			},
			presubmits: []config.Presubmit{{
				JobBase:  config.JobBase{Name: "ignored"},
				Reporter: config.Reporter{Context: "first"},
			}, {
				JobBase:  config.JobBase{Name: "ignored"},
				Reporter: config.Reporter{Context: "second"},
			}},
			changesError:      false,
			expectedToTrigger: nil,
			expectedToSkip:    nil,
			expectErr:         false,
		},
		{
			name: "everything matches and is forced to run, nothing to skip",
			filter: func(p config.Presubmit) (shouldRun bool, forcedToRun bool, defaultBehavior bool) {
				return true, true, true
			},
			presubmits: []config.Presubmit{{
				JobBase:  config.JobBase{Name: "should-trigger"},
				Reporter: config.Reporter{Context: "first"},
			}, {
				JobBase:  config.JobBase{Name: "should-trigger"},
				Reporter: config.Reporter{Context: "second"},
			}},
			changesError: false,
			expectedToTrigger: []config.Presubmit{{
				JobBase:  config.JobBase{Name: "should-trigger"},
				Reporter: config.Reporter{Context: "first"},
			}, {
				JobBase:  config.JobBase{Name: "should-trigger"},
				Reporter: config.Reporter{Context: "second"},
			}},
			expectedToSkip: nil,
			expectErr:      false,
		},
		{
			name: "error detecting if something should run, nothing to run or skip",
			filter: func(p config.Presubmit) (shouldRun bool, forcedToRun bool, defaultBehavior bool) {
				return true, false, false
			},
			presubmits: []config.Presubmit{{
				JobBase:             config.JobBase{Name: "errors"},
				Reporter:            config.Reporter{Context: "first"},
				RegexpChangeMatcher: config.RegexpChangeMatcher{RunIfChanged: "oopsie"},
			}, {
				JobBase:  config.JobBase{Name: "ignored"},
				Reporter: config.Reporter{Context: "second"},
			}},
			changesError:      true,
			expectedToTrigger: nil,
			expectedToSkip:    nil,
			expectErr:         true,
		},
		{
			name: "some things match and are forced to run, nothing to skip",
			filter: func(p config.Presubmit) (shouldRun bool, forcedToRun bool, defaultBehavior bool) {
				return p.Name == "should-trigger", true, true
			},
			presubmits: []config.Presubmit{{
				JobBase:  config.JobBase{Name: "should-trigger"},
				Reporter: config.Reporter{Context: "first"},
			}, {
				JobBase:  config.JobBase{Name: "ignored"},
				Reporter: config.Reporter{Context: "second"},
			}},
			changesError: false,
			expectedToTrigger: []config.Presubmit{{
				JobBase:  config.JobBase{Name: "should-trigger"},
				Reporter: config.Reporter{Context: "first"},
			}},
			expectedToSkip: nil,
			expectErr:      false,
		},
		{
			name: "everything matches and some things are forced to run, others should be skipped",
			filter: func(p config.Presubmit) (shouldRun bool, forcedToRun bool, defaultBehavior bool) {
				return true, p.Name == "should-trigger", p.Name == "should-trigger"
			},
			presubmits: []config.Presubmit{{
				JobBase:  config.JobBase{Name: "should-trigger"},
				Reporter: config.Reporter{Context: "first"},
			}, {
				JobBase:  config.JobBase{Name: "should-trigger"},
				Reporter: config.Reporter{Context: "second"},
			}, {
				JobBase:  config.JobBase{Name: "should-skip"},
				Reporter: config.Reporter{Context: "third"},
			}, {
				JobBase:  config.JobBase{Name: "should-skip"},
				Reporter: config.Reporter{Context: "fourth"},
			}},
			changesError: false,
			expectedToTrigger: []config.Presubmit{{
				JobBase:  config.JobBase{Name: "should-trigger"},
				Reporter: config.Reporter{Context: "first"},
			}, {
				JobBase:  config.JobBase{Name: "should-trigger"},
				Reporter: config.Reporter{Context: "second"},
			}},
			expectedToSkip: []config.Presubmit{{
				JobBase:  config.JobBase{Name: "should-skip"},
				Reporter: config.Reporter{Context: "third"},
			}, {
				JobBase:  config.JobBase{Name: "should-skip"},
				Reporter: config.Reporter{Context: "fourth"},
			}},
			expectErr: false,
		},
	}

	branch := "foobar"

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actualToTrigger, actualToSkip, err := FilterPresubmits(testCase.filter, fakeChangedFilesProvider(testCase.changesError), branch, testCase.presubmits, logrus.WithField("test-case", testCase.name))
			if testCase.expectErr && err == nil {
				t.Errorf("%s: expected an error filtering presubmits, but got none", testCase.name)
			}
			if !testCase.expectErr && err != nil {
				t.Errorf("%s: expected no error filtering presubmits, but got one: %v", testCase.name, err)
			}
			if !reflect.DeepEqual(actualToTrigger, testCase.expectedToTrigger) {
				t.Errorf("%s: incorrect set of presubmits to skip: %s", testCase.name, diff.ObjectReflectDiff(actualToTrigger, testCase.expectedToTrigger))
			}
			if !reflect.DeepEqual(actualToSkip, testCase.expectedToSkip) {
				t.Errorf("%s: incorrect set of presubmits to skip: %s", testCase.name, diff.ObjectReflectDiff(actualToSkip, testCase.expectedToSkip))
			}
		})
	}
}
