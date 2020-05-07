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
	"fmt"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/github"
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
		name              string
		filter            Filter
		presubmits        []config.Presubmit
		changesError      bool
		expectedToTrigger []config.Presubmit
		expectErr         bool
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
			expectErr: false,
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
			expectErr: false,
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
			expectErr: false,
		},
		{
			name: "everything matches and some that are forces to run supercede some that are skipped due to shared contexts",
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
				JobBase:  config.JobBase{Name: "should-not-skip"},
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
			expectErr: false,
		},
	}

	branch := "foobar"

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actualToTrigger, err := FilterPresubmits(testCase.filter, fakeChangedFilesProvider(testCase.changesError), branch, testCase.presubmits, logrus.WithField("test-case", testCase.name))
			if testCase.expectErr && err == nil {
				t.Errorf("%s: expected an error filtering presubmits, but got none", testCase.name)
			}
			if !testCase.expectErr && err != nil {
				t.Errorf("%s: expected no error filtering presubmits, but got one: %v", testCase.name, err)
			}
			if !reflect.DeepEqual(actualToTrigger, testCase.expectedToTrigger) {
				t.Errorf("%s: incorrect set of presubmits to skip: %s", testCase.name, diff.ObjectReflectDiff(actualToTrigger, testCase.expectedToTrigger))
			}
		})
	}
}

type orgRepoRef struct {
	org, repo, ref string
}

type fakeContextGetter struct {
	status map[orgRepoRef]*github.CombinedStatus
	errors map[orgRepoRef]error
}

func (f *fakeContextGetter) getContexts(key orgRepoRef) (sets.String, sets.String, error) {
	allContexts := sets.NewString()
	failedContexts := sets.NewString()
	if err, exists := f.errors[key]; exists {
		return failedContexts, allContexts, err
	}
	combinedStatus, exists := f.status[key]
	if !exists {
		return failedContexts, allContexts, fmt.Errorf("failed to find status for %s/%s@%s", key.org, key.repo, key.ref)
	}
	for _, status := range combinedStatus.Statuses {
		allContexts.Insert(status.Context)
		if status.State == github.StatusError || status.State == github.StatusFailure {
			failedContexts.Insert(status.Context)
		}
	}
	return failedContexts, allContexts, nil
}

func TestPresubmitFilter(t *testing.T) {
	statuses := &github.CombinedStatus{Statuses: []github.Status{
		{
			Context: "existing-successful",
			State:   github.StatusSuccess,
		},
		{
			Context: "existing-pending",
			State:   github.StatusPending,
		},
		{
			Context: "existing-error",
			State:   github.StatusError,
		},
		{
			Context: "existing-failure",
			State:   github.StatusFailure,
		},
	}}
	var testCases = []struct {
		name                 string
		honorOkToTest        bool
		body, org, repo, ref string
		presubmits           []config.Presubmit
		expected             [][]bool
		statusErr, expectErr bool
	}{
		{
			name: "test all comment selects all tests that don't need an explicit trigger",
			body: "/test all",
			org:  "org",
			repo: "repo",
			ref:  "ref",
			presubmits: []config.Presubmit{
				{
					JobBase: config.JobBase{
						Name: "always-runs",
					},
					AlwaysRun: true,
					Reporter: config.Reporter{
						Context: "always-runs",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "runs-if-changed",
					},
					Reporter: config.Reporter{
						Context: "runs-if-changed",
					},
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
			},
			expected: [][]bool{{true, false, false}, {true, false, false}, {false, false, false}},
		},
		{
			name:          "honored ok-to-test comment selects all tests that don't need an explicit trigger",
			body:          "/ok-to-test",
			honorOkToTest: true,
			org:           "org",
			repo:          "repo",
			ref:           "ref",
			presubmits: []config.Presubmit{
				{
					JobBase: config.JobBase{
						Name: "always-runs",
					},
					AlwaysRun: true,
					Reporter: config.Reporter{
						Context: "always-runs",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "runs-if-changed",
					},
					Reporter: config.Reporter{
						Context: "runs-if-changed",
					},
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
			},
			expected: [][]bool{{true, false, false}, {true, false, false}, {false, false, false}},
		},
		{
			name:          "not honored ok-to-test comment selects no tests",
			body:          "/ok-to-test",
			honorOkToTest: false,
			org:           "org",
			repo:          "repo",
			ref:           "ref",
			presubmits: []config.Presubmit{
				{
					JobBase: config.JobBase{
						Name: "always-runs",
					},
					AlwaysRun: true,
					Reporter: config.Reporter{
						Context: "always-runs",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "runs-if-changed",
					},
					Reporter: config.Reporter{
						Context: "runs-if-changed",
					},
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
			},
			expected: [][]bool{{false, false, false}, {false, false, false}, {false, false, false}},
		},
		{
			name:       "statuses are not gathered unless retest is specified (will error but we should not see it)",
			body:       "not a command",
			org:        "org",
			repo:       "repo",
			ref:        "ref",
			presubmits: []config.Presubmit{},
			expected:   [][]bool{},
			statusErr:  true,
			expectErr:  false,
		},
		{
			name:       "statuses are gathered when retest is specified and gather error is propagated",
			body:       "/retest",
			org:        "org",
			repo:       "repo",
			ref:        "ref",
			presubmits: []config.Presubmit{},
			expected:   [][]bool{},
			statusErr:  true,
			expectErr:  true,
		},
		{
			name: "retest command selects for errored or failed contexts and required but missing contexts",
			body: "/retest",
			org:  "org",
			repo: "repo",
			ref:  "ref",
			presubmits: []config.Presubmit{
				{
					JobBase: config.JobBase{
						Name: "successful-job",
					},
					Reporter: config.Reporter{
						Context: "existing-successful",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "pending-job",
					},
					Reporter: config.Reporter{
						Context: "existing-pending",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "failure-job",
					},
					Reporter: config.Reporter{
						Context: "existing-failure",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "error-job",
					},
					Reporter: config.Reporter{
						Context: "existing-error",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "missing-always-runs",
					},
					Reporter: config.Reporter{
						Context: "missing-always-runs",
					},
					AlwaysRun: true,
				},
			},
			expected: [][]bool{{false, false, false}, {false, false, false}, {true, false, true}, {true, false, true}, {true, false, true}},
		},
		{
			name: "explicit test command filters for jobs that match",
			body: "/test trigger",
			org:  "org",
			repo: "repo",
			ref:  "ref",
			presubmits: []config.Presubmit{
				{
					JobBase: config.JobBase{
						Name: "always-runs",
					},
					AlwaysRun: true,
					Reporter: config.Reporter{
						Context: "always-runs",
					},
					Trigger:      `(?m)^/test (?:.*? )?trigger(?: .*?)?$`,
					RerunCommand: "/test trigger",
				},
				{
					JobBase: config.JobBase{
						Name: "runs-if-changed",
					},
					Reporter: config.Reporter{
						Context: "runs-if-changed",
					},
					RegexpChangeMatcher: config.RegexpChangeMatcher{
						RunIfChanged: "sometimes",
					},
					Trigger:      `(?m)^/test (?:.*? )?trigger(?: .*?)?$`,
					RerunCommand: "/test trigger",
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
						Name: "always-runs",
					},
					AlwaysRun: true,
					Reporter: config.Reporter{
						Context: "always-runs",
					},
					Trigger:      `(?m)^/test (?:.*? )?other-trigger(?: .*?)?$`,
					RerunCommand: "/test other-trigger",
				},
				{
					JobBase: config.JobBase{
						Name: "runs-if-changed",
					},
					Reporter: config.Reporter{
						Context: "runs-if-changed",
					},
					RegexpChangeMatcher: config.RegexpChangeMatcher{
						RunIfChanged: "sometimes",
					},
					Trigger:      `(?m)^/test (?:.*? )?other-trigger(?: .*?)?$`,
					RerunCommand: "/test other-trigger",
				},
				{
					JobBase: config.JobBase{
						Name: "runs-if-triggered",
					},
					Reporter: config.Reporter{
						Context: "runs-if-triggered",
					},
					Trigger:      `(?m)^/test (?:.*? )?other-trigger(?: .*?)?$`,
					RerunCommand: "/test other-trigger",
				},
			},
			expected: [][]bool{{true, true, true}, {true, true, true}, {true, true, true}, {false, false, false}, {false, false, false}, {false, false, false}},
		},
		{
			name: "comments matching more than one case will select the union of presubmits",
			body: `/test trigger
/test all
/retest`,
			org:  "org",
			repo: "repo",
			ref:  "ref",
			presubmits: []config.Presubmit{
				{
					JobBase: config.JobBase{
						Name: "always-runs",
					},
					AlwaysRun: true,
					Reporter: config.Reporter{
						Context: "existing-successful",
					},
					Trigger:      `(?m)^/test (?:.*? )?other-trigger(?: .*?)?$`,
					RerunCommand: "/test other-trigger",
				},
				{
					JobBase: config.JobBase{
						Name: "runs-if-changed",
					},
					Reporter: config.Reporter{
						Context: "existing-successful",
					},
					RegexpChangeMatcher: config.RegexpChangeMatcher{
						RunIfChanged: "sometimes",
					},
					Trigger:      `(?m)^/test (?:.*? )?other-trigger(?: .*?)?$`,
					RerunCommand: "/test other-trigger",
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
						Name: "successful-job",
					},
					Reporter: config.Reporter{
						Context: "existing-successful",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "pending-job",
					},
					Reporter: config.Reporter{
						Context: "existing-pending",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "failure-job",
					},
					Reporter: config.Reporter{
						Context: "existing-failure",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "error-job",
					},
					Reporter: config.Reporter{
						Context: "existing-error",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "missing-always-runs",
					},
					AlwaysRun: true,
					Reporter: config.Reporter{
						Context: "missing-always-runs",
					},
				},
			},
			expected: [][]bool{{true, false, false}, {true, false, false}, {true, true, true}, {false, false, false}, {false, false, false}, {true, false, true}, {true, false, true}, {true, false, true}},
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
			fsg := &fakeContextGetter{
				errors: map[orgRepoRef]error{},
				status: map[orgRepoRef]*github.CombinedStatus{},
			}
			key := orgRepoRef{org: testCase.org, repo: testCase.repo, ref: testCase.ref}
			if testCase.statusErr {
				fsg.errors[key] = errors.New("failure")
			} else {
				fsg.status[key] = statuses
			}

			fakeContextGetter := func() (sets.String, sets.String, error) {

				return fsg.getContexts(key)
			}

			filter, err := PresubmitFilter(testCase.honorOkToTest, fakeContextGetter, testCase.body, logrus.WithField("test-case", testCase.name))

			if testCase.expectErr && err == nil {
				t.Errorf("%s: expected an error creating the filter, but got none", testCase.name)
			}
			if !testCase.expectErr && err != nil {
				t.Errorf("%s: expected no error creating the filter, but got one: %v", testCase.name, err)
			}
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
