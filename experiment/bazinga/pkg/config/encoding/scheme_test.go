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

package encoding

import (
	"fmt"
	"regexp/syntax"
	"testing"

	"k8s.io/test-infra/experiment/bazinga/pkg/config"
)

func TestLoadCurrent(t *testing.T) {
	cases := []struct {
		TestName     string
		Path         string
		ExpectError  bool
		ExpectConfig config.App
	}{
		{
			TestName:    "No config",
			Path:        "",
			ExpectError: false,
		},
		{
			TestName:    "Invalid path",
			Path:        "./testdata/not-a-file.bogus",
			ExpectError: true,
		},
		{
			TestName:    "Invalid API version",
			Path:        "./testdata/invalid-apiversion.yaml",
			ExpectError: true,
		},
		{
			TestName:    "Invalid bazinga",
			Path:        "./testdata/invalid-bazinga.yaml",
			ExpectError: true,
		},
		{
			TestName:    "Invalid YAML",
			Path:        "./testdata/invalid-yaml.yaml",
			ExpectError: true,
		},
		{
			TestName:    "Invalid Failure Condition Regexp",
			Path:        "./testdata/invalid-regexp.yaml",
			ExpectError: true,
			ExpectConfig: getExpectedConfig(func(c *config.App) {
				c.TestSuites = nil
				c.FailureConditions = []config.FailureCondition{{Category: "FAILURE", Pattern: `^.*\[FAIL(ED|URE)?\]).*$`, Flags: syntax.FoldCase}}
			}),
		},
		{
			TestName:     "No test cases",
			Path:         "./testdata/no-test-cases.yaml",
			ExpectError:  false,
			ExpectConfig: getExpectedConfig(func(c *config.App) { c.TestSuites[0].TestCases = nil }),
		},
		{
			TestName:     "Valid test case",
			Path:         "./testdata/valid-test-case.yaml",
			ExpectError:  false,
			ExpectConfig: getExpectedConfig(nil),
		},
		{
			TestName:     "Invalid test case missing command",
			Path:         "./testdata/invalid-test-case-missing-command.yaml",
			ExpectError:  true,
			ExpectConfig: getExpectedConfig(nil),
		},
	}
	for _, c := range cases {
		t.Run(c.TestName, func(t *testing.T) {
			handleTestCase := func() error {
				actConfig, err := Load(c.Path)
				if err != nil {
					return err
				}
				return assertEqAppConfig(c.ExpectConfig, *actConfig)
			}
			if err := handleTestCase(); err != nil && !c.ExpectError {
				t.Fatalf("unexpected error while loading config: %v", err)
			} else if err == nil && c.ExpectError {
				t.Fatalf("unexpected lack or error while loading config")
			}
		})
	}
}

func assertEqAppConfig(expConfig, actConfig config.App) error {
	if actConfig.Output != expConfig.Output {
		return fmt.Errorf("unexpected output: exp=%s act=%s", expConfig.Output, actConfig.Output)
	}
	if err := assertEqFailConds(expConfig.FailureConditions, actConfig.FailureConditions); err != nil {
		return fmt.Errorf("unexpected failureCondition: %s", err)
	}
	if len(actConfig.TestSuites) != len(expConfig.TestSuites) {
		return fmt.Errorf("unexpected test suite count: exp=%d act=%d", len(expConfig.TestSuites), len(actConfig.TestSuites))
	}
	for i, expSuite := range expConfig.TestSuites {
		actSuite := actConfig.TestSuites[i]
		if actSuite.Name != expSuite.Name {
			return fmt.Errorf("unexpected test suite %d name: exp=%s act=%s", i, expSuite.Name, actSuite.Name)
		}
		if err := assertEqFailConds(expSuite.FailureConditions, actSuite.FailureConditions); err != nil {
			return fmt.Errorf("unexpected test suite %d failureCondition: %s", i, err)
		}
		if len(actSuite.TestCases) != len(expSuite.TestCases) {
			return fmt.Errorf("unexpected test case count: exp=%d act=%d", len(expSuite.TestCases), len(actSuite.TestCases))
		}
		for j, expCase := range expSuite.TestCases {
			actCase := actSuite.TestCases[j]
			if actCase.Name != expCase.Name {
				return fmt.Errorf("unexpected test case %d %d name: exp=%s act=%s", i, j, expCase.Name, actCase.Name)
			}
			if actCase.Name != expCase.Name {
				return fmt.Errorf("unexpected test case %d %d class: exp=%s act=%s", i, j, expCase.Class, actCase.Class)
			}
			if actCase.Command != expCase.Command {
				return fmt.Errorf("unexpected test case %d %d command: exp=%s act=%s", i, j, expCase.Command, actCase.Command)
			}
			if err := assertEqStringSlice(expCase.Args, actCase.Args); err != nil {
				return fmt.Errorf("unexpected test case %d %d args: %s", i, j, err)
			}
			if err := assertEqStringSlice(expCase.Env, actCase.Env); err != nil {
				return fmt.Errorf("unexpected test case %d %d env: %s", i, j, err)
			}
			if actCase.EnvClean != expCase.EnvClean {
				return fmt.Errorf("unexpected test case %d %d envClean: exp=%v act=%v", i, j, expCase.EnvClean, actCase.EnvClean)
			}
			if err := assertEqFailConds(expCase.FailureConditions, actCase.FailureConditions); err != nil {
				return fmt.Errorf("unexpected test case %d %d failureCondition: %s", i, j, err)
			}
		}
	}
	return nil
}

func assertEqStringSlice(exp, act []string) error {
	if len(exp) != len(act) {
		return fmt.Errorf("len exp=%d act=%d", len(exp), len(act))
	}
	for i := range exp {
		e := exp[i]
		a := act[i]
		if e != a {
			return fmt.Errorf("%d exp=%s act=%s", i, e, a)
		}
	}
	return nil
}

func assertEqFailConds(exp, act []config.FailureCondition) error {
	if len(exp) != len(act) {
		return fmt.Errorf("len exp=%d act=%d", len(exp), len(act))
	}
	for i := range exp {
		e := exp[i]
		a := act[i]
		if e.Category != a.Category {
			return fmt.Errorf("%d category exp=%s act=%s", i, e.Category, a.Category)
		}
		if e.Pattern != a.Pattern {
			return fmt.Errorf("%d pattern exp=%s act=%s", i, e.Pattern, a.Pattern)
		}
		if e.Flags != a.Flags {
			return fmt.Errorf("%d flags exp=%d act=%d", i, e.Flags, a.Flags)
		}
		if e.Pattern != "" {
			_, err := syntax.Parse(e.Pattern, e.Flags)
			if err != nil {
				return fmt.Errorf("%d invalid regexp pattern=%s: %v", i, e.Pattern, err)
			}
		}
	}
	return nil
}

func getExpectedConfig(modify func(*config.App)) config.App {
	appConfig := config.App{
		Output: "junit_1.xml",
		TestSuites: []config.TestSuite{
			{
				Name: "bazinga",
				TestCases: []config.TestCase{
					{
						Class:    "go",
						Name:     "test",
						Command:  "make",
						Args:     []string{"test"},
						Env:      []string{"PATH=/go/bin"},
						EnvClean: true,
						FailureConditions: []config.FailureCondition{
							{
								Category: "FAILURE",
								Pattern:  `^.*\[FAIL(ED|URE)?\].*$`,
								Flags:    syntax.FoldCase,
							},
							{
								Category: "ERROR",
								Pattern:  `^.*\[ERROR].*$`,
								Flags:    syntax.FoldCase,
							},
						},
					},
				},
			},
		},
	}
	if modify != nil {
		modify(&appConfig)
	}
	return appConfig
}
