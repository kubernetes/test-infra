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

	"golang.org/x/lint"
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

func TestLintErrorf(t *testing.T) {
	var testcases = []struct {
		problem            lint.Problem
		expectedSuggestion string
	}{
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "should replace errors.New(fmt.Sprintf(...)) with fmt.Errorf(...)",
				Link:       "",
				Category:   "error",
				LineText:   "        return errors.New(fmt.Sprintf(\"something %d\", x))",
				Confidence: 100.00,
			},
			expectedSuggestion: "```suggestion\n        return fmt.Errorf(\"something %d\", x)```\n",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "should replace errors.New(fmt.Sprintf(...)) with fmt.Errorf(...)",
				Link:       "",
				Category:   "error",
				LineText:   "        return errors.New(fmt.Sprintf(\"something %s %d\", fooFunc(foo), bar))",
				Confidence: 100.00,
			},
			expectedSuggestion: "```suggestion\n        return fmt.Errorf(\"something %s %d\", fooFunc(foo), bar)```\n",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "should replace errors.New(fmt.Sprintf(...)) with fmt.Errorf(...)",
				Link:       "",
				Category:   "error",
				LineText:   "        return errors.New(fmt.Sprintf(\"something %s %d\", fooFunc(barFunc(foo), string(x)), bar))",
				Confidence: 100.00,
			},
			expectedSuggestion: "```suggestion\n        return fmt.Errorf(\"something %s %d\", fooFunc(barFunc(foo), string(x)), bar)```\n",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "should replace errors.New(fmt.Sprintf(...)) with fmt.Errorf(...)",
				Link:       "",
				Category:   "error",
				LineText:   "        return fmt.Errorf(\"something %d\", x)",
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

func TestLintLoopRanges(t *testing.T) {
	var testcases = []struct {
		problem            lint.Problem
		expectedSuggestion string
	}{
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "should omit values from range; this loop is equivalent to `for range ...`",
				Link:       "",
				Category:   "range-loop",
				LineText:   "for _ = range m {",
				Confidence: 100.00,
			},
			expectedSuggestion: "```suggestion\nfor range m {```\n",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "should omit values from range; this loop is equivalent to `for range ...`",
				Link:       "",
				Category:   "range-loop",
				LineText:   "for _, _ = range m {",
				Confidence: 100.00,
			},
			expectedSuggestion: "```suggestion\nfor range m {```\n",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "should omit 2nd value from range; this loop is equivalent to `for y = range ...`",
				Link:       "",
				Category:   "range-loop",
				LineText:   "for y, _ = range m {",
				Confidence: 100.00,
			},
			expectedSuggestion: "```suggestion\nfor y = range m {```\n",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "should omit 2nd value from range; this loop is equivalent to `for yVar1 = range ...`",
				Link:       "",
				Category:   "range-loop",
				LineText:   "for yVar1, _ = range m {",
				Confidence: 100.00,
			},
			expectedSuggestion: "```suggestion\nfor yVar1 = range m {```\n",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "should omit 2nd value from range; this loop is equivalent to `for y := range ...`",
				Link:       "",
				Category:   "range-loop",
				LineText:   "for y, _ := range m {",
				Confidence: 100.00,
			},
			expectedSuggestion: "```suggestion\nfor y := range m {```\n",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "should omit values from range; this loop is equivalent to `for range ...`",
				Link:       "",
				Category:   "range-loop",
				LineText:   "for range m {",
				Confidence: 100.00,
			},
			expectedSuggestion: "",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "should omit 2nd value from range; this loop is equivalent to `for y = range ...`",
				Link:       "",
				Category:   "range-loop",
				LineText:   "for y = range m {",
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

func TestLintVarDecl(t *testing.T) {
	var testcases = []struct {
		problem            lint.Problem
		expectedSuggestion string
	}{
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "should omit type int from declaration of var myInt; it will be inferred from the right-hand side",
				Link:       "",
				Category:   "type-inference",
				LineText:   "var myInt int = 7",
				Confidence: 100.00,
			},
			expectedSuggestion: "```suggestion\nvar myInt = 7```\n",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "should drop = 0 from declaration of var myZeroInt; it is the zero value",
				Link:       "",
				Category:   "zero-value",
				LineText:   "var myZeroInt int = 0",
				Confidence: 100.00,
			},
			expectedSuggestion: "```suggestion\nvar myZeroInt int```\n",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "should drop = 0. from declaration of var myZeroFlt; it is the zero value",
				Link:       "",
				Category:   "zero-value",
				LineText:   "var myZeroFlt float32 = 0.",
				Confidence: 100.00,
			},
			expectedSuggestion: "```suggestion\nvar myZeroFlt float32```\n",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "should drop = 0.0 from declaration of var myZeroF64; it is the zero value",
				Link:       "",
				Category:   "zero-value",
				LineText:   "var myZeroF64 float64 = 0.0",
				Confidence: 100.00,
			},
			expectedSuggestion: "```suggestion\nvar myZeroF64 float64```\n",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "should drop = 0i from declaration of var myZeroImg; it is the zero value",
				Link:       "",
				Category:   "zero-value",
				LineText:   "var myZeroImg complex64 = 0i",
				Confidence: 100.00,
			},
			expectedSuggestion: "```suggestion\nvar myZeroImg complex64```\n",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "should drop = \"\" from declaration of var myZeroStr; it is the zero value",
				Link:       "",
				Category:   "zero-value",
				LineText:   "var myZeroStr string = \"\"",
				Confidence: 100.00,
			},
			expectedSuggestion: "```suggestion\nvar myZeroStr string```\n",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "should drop = `` from declaration of var myZeroStr; it is the zero value",
				Link:       "",
				Category:   "zero-value",
				LineText:   "var myZeroStr string = ``",
				Confidence: 100.00,
			},
			expectedSuggestion: "```suggestion\nvar myZeroStr string```\n",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "should drop = nil from declaration of var myZeroPtr; it is the zero value",
				Link:       "",
				Category:   "zero-value",
				LineText:   "var myZeroPtr *Q = nil",
				Confidence: 100.00,
			},
			expectedSuggestion: "```suggestion\nvar myZeroPtr *Q```\n",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "should drop = '\\x00' from declaration of var myZeroRune; it is the zero value",
				Link:       "",
				Category:   "zero-value",
				LineText:   "var myZeroRune rune = '\\x00'",
				Confidence: 100.00,
			},
			expectedSuggestion: "```suggestion\nvar myZeroRune rune```\n",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "should drop = '\\000' from declaration of var myZeroRune2; it is the zero value",
				Link:       "",
				Category:   "zero-value",
				LineText:   "var myZeroRune2 rune = '\\000'",
				Confidence: 100.00,
			},
			expectedSuggestion: "```suggestion\nvar myZeroRune2 rune```\n",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "should drop = `` from declaration of var myZeroStr; it is the zero value",
				Link:       "",
				Category:   "zero-value",
				LineText:   "var myZeroStr string",
				Confidence: 100.00,
			},
			expectedSuggestion: "",
		},
		{
			problem: lint.Problem{
				Position: token.Position{
					Filename: "qux.go",
				},
				Text:       "should omit type int from declaration of var myInt; it will be inferred from the right-hand side",
				Link:       "",
				Category:   "type-inference",
				LineText:   "var myInt = 7",
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
