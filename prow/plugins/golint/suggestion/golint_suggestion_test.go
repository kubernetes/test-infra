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

package suggestion

import (
	"go/token"
	"testing"

	"github.com/golang/lint"
)

func TestLintNamesUnderscore(t *testing.T) {
	var testcases = []struct {
		problem            lint.Problem
		expectedSuggestion string
	}{
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "don't use underscores in Go names; func Qux_1 should be Qux1",
				Link:       "http://golang.org/doc/effective_go.html#mixed-caps",
				Category:   "naming",
				LineText:   "func Qux_1() error {",
				Confidence: 100.00,
			},
			expectedSuggestion: "```suggestion\nfunc Qux1() error {```\n",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "don't use underscores in Go names; func Qux_Foo_Func should be QuxFooFunc",
				Link:       "http://golang.org/doc/effective_go.html#mixed-caps",
				Category:   "naming",
				LineText:   "func Qux_Foo_Func() error {",
				Confidence: 100.00,
			},
			expectedSuggestion: "```suggestion\nfunc QuxFooFunc() error {```\n",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "don't use underscores in Go names; func Qux_Foo_Func should be QuxFooFunc",
				Link:       "http://golang.org/doc/effective_go.html#mixed-caps",
				Category:   "naming",
				LineText:   "func QuxCorrectFunc() error {",
				Confidence: 100.00,
			},
			expectedSuggestion: "",
		},
	}
	for _, test := range testcases {
		suggestion := SuggestCodeChange(test.problem)
		if suggestion != test.expectedSuggestion {
			t.Errorf("Excepted code suggestion %s but got %s for LineText %s", test.expectedSuggestion, suggestion, test.problem.LineText)
		}
	}
}

func TestLintNamesAllCaps(t *testing.T) {
	var testcases = []struct {
		problem            lint.Problem
		expectedSuggestion string
	}{
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "don't use ALL_CAPS in Go names; use CamelCase",
				Link:       "",
				Category:   "naming",
				LineText:   "func QUX_FUNC() error {",
				Confidence: 100.00,
			},
			expectedSuggestion: "```suggestion\nfunc QuxFunc() error {```\n",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "don't use ALL_CAPS in Go names; use CamelCase",
				Link:       "http://golang.org/doc/effective_go.html#mixed-caps",
				Category:   "naming",
				LineText:   "func QUX() error {",
				Confidence: 100.00,
			},
			expectedSuggestion: "```suggestion\nfunc Qux() error {```\n",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "don't use ALL_CAPS in Go names; use CamelCase",
				Link:       "http://golang.org/doc/effective_go.html#mixed-caps",
				Category:   "naming",
				LineText:   "func QUX_FOO_FUNC() error {",
				Confidence: 100.00,
			},
			expectedSuggestion: "```suggestion\nfunc QuxFooFunc() error {```\n",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "don't use ALL_CAPS in Go names; use CamelCase",
				Link:       "http://golang.org/doc/effective_go.html#mixed-caps",
				Category:   "naming",
				LineText:   "func QUX_FOO_FUNC_1() error {",
				Confidence: 100.00,
			},
			expectedSuggestion: "```suggestion\nfunc QuxFooFunc1() error {```\n",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "don't use ALL_CAPS in Go names; use CamelCase",
				Link:       "http://golang.org/doc/effective_go.html#mixed-caps",
				Category:   "naming",
				LineText:   "func QuxCorrectFunc() error {",
				Confidence: 100.00,
			},
			expectedSuggestion: "",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "don't use ALL_CAPS in Go names; use CamelCase",
				Link:       "http://golang.org/doc/effective_go.html#mixed-caps",
				Category:   "naming",
				LineText:   "func Qux() error {",
				Confidence: 100.00,
			},
			expectedSuggestion: "",
		},
	}
	for _, test := range testcases {
		suggestion := SuggestCodeChange(test.problem)
		if suggestion != test.expectedSuggestion {
			t.Errorf("Excepted code suggestion %s but got %s for LineText %s", test.expectedSuggestion, suggestion, test.problem.LineText)
		}
	}
}

func TestLintStutter(t *testing.T) {
	var testcases = []struct {
		problem            lint.Problem
		expectedSuggestion string
	}{
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "func name will be used as bar.BarFunc by other packages, and that stutters; consider calling this Func",
				Link:       "https://golang.org/wiki/CodeReviewComments#package-names",
				Category:   "naming",
				LineText:   "func BarFunc() error {",
				Confidence: 100.00,
			},
			expectedSuggestion: "```suggestion\nfunc Func() error {```\n",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "type name will be used as bar.BarMaker by other packages, and that stutters; consider calling this Maker",
				Link:       "https://golang.org/wiki/CodeReviewComments#package-names",
				Category:   "naming",
				LineText:   "type BarMaker struct{}",
				Confidence: 100.00,
			},
			expectedSuggestion: "```suggestion\ntype Maker struct{}```\n",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "type name will be used as bar.Bar by other packages, and that stutters; consider calling this Bar",
				Link:       "https://golang.org/wiki/CodeReviewComments#package-names",
				Category:   "naming",
				LineText:   "type Bar struct{}",
				Confidence: 100.00,
			},
			expectedSuggestion: "",
		},
	}
	for _, test := range testcases {
		suggestion := SuggestCodeChange(test.problem)
		if suggestion != test.expectedSuggestion {
			t.Errorf("Excepted code suggestion %s but got %s for LineText %s", test.expectedSuggestion, suggestion, test.problem.LineText)
		}
	}
}
