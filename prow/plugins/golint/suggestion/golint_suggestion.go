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
	"fmt"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"
	"golang.org/x/lint"
)

var (
	lintErrorfRegex          = regexp.MustCompile(`should replace errors\.New\(fmt\.Sprintf\(\.\.\.\)\) with fmt\.Errorf\(\.\.\.\)`)
	lintNamesUnderscoreRegex = regexp.MustCompile("don't use underscores in Go names; (.*) should be (.*)")
	lintNamesAllCapsRegex    = regexp.MustCompile("don't use ALL_CAPS in Go names; use CamelCase")
	lintStutterRegex         = regexp.MustCompile("name will be used as [^.]+\\.(.*) by other packages, and that stutters; consider calling this (.*)")
	lintRangesRegex          = regexp.MustCompile("should omit (?:2nd )?values? from range; this loop is equivalent to \\x60(for .*) ...\\x60")
	lintVarDeclRegex         = regexp.MustCompile("should (?:omit type|drop) (.*) from declaration of (?:.*); (?:it will be inferred from the right-hand side|it is the zero value)")
)

var lintHandlersMap = map[*regexp.Regexp]func(lint.Problem, []string) string{
	lintErrorfRegex:          fixErrorf,
	lintNamesUnderscoreRegex: fixNameUnderscore,
	lintNamesAllCapsRegex:    fixNameAllCaps,
	lintStutterRegex:         fixStutter,
	lintRangesRegex:          fixRanges,
	lintVarDeclRegex:         fixVarDecl,
}

// SuggestCodeChange returns code suggestions for a given lint.Problem
// Returns empty string if no suggestion can be given
func SuggestCodeChange(p lint.Problem) string {
	var suggestion string
	for regex, handler := range lintHandlersMap {
		matches := regex.FindStringSubmatch(p.Text)
		suggestion = handler(p, matches)
		if suggestion != "" && suggestion != p.LineText {
			return formatSuggestion(suggestion)
		}
	}
	return ""
}

func fixNameUnderscore(p lint.Problem, matches []string) string {
	if len(matches) < 3 {
		return ""
	}
	underscoreRe := regexp.MustCompile(`[A-Za-z]+(_[A-Za-z0-9]+)+`)
	namesWithUnderscore := underscoreRe.FindStringSubmatch(matches[1])
	suggestion := strings.Replace(p.LineText, namesWithUnderscore[0], matches[2], -1)
	return suggestion
}

func fixNameAllCaps(p lint.Problem, matches []string) string {
	result := ""
	if len(matches) == 0 {
		return result
	}
	// Identify all caps names
	reAllCaps := regexp.MustCompile(`[A-Z]+(_[A-Z0-9]+)*`)
	result = reAllCaps.ReplaceAllStringFunc(p.LineText, func(oldName string) string {
		return strings.Replace(strings.Title(strings.Replace(strings.ToLower(oldName), "_", " ", -1)), " ", "", -1)
	})
	return result
}

func fixStutter(p lint.Problem, matches []string) string {
	if len(matches) < 3 {
		return ""
	}
	suggestion := strings.Replace(p.LineText, matches[1], matches[2], -1)
	return suggestion
}

func fixErrorf(p lint.Problem, matches []string) string {
	if len(matches) != 1 {
		return ""
	}
	parameterText := ""
	count := 0
	parameterTextBeginning := "errors.New(fmt.Sprintf("
	parameterTextBeginningInd := strings.Index(p.LineText, parameterTextBeginning)
	if parameterTextBeginningInd < 0 {
		logrus.Infof("Cannot find \"errors.New(fmt.Sprintf(\" in problem line text %s", p.LineText)
		return ""
	}
	for _, char := range p.LineText[parameterTextBeginningInd+len(parameterTextBeginning):] {
		if char == '(' {
			count++
		}
		if char == ')' {
			count--
			if count < 0 {
				break
			}
		}
		parameterText += string(char)
	}
	if count > 0 {
		return ""
	}
	toReplace := fmt.Sprintf("errors.New(fmt.Sprintf(%s))", parameterText)
	replacement := fmt.Sprintf("fmt.Errorf(%s)", parameterText)
	suggestion := strings.Replace(p.LineText, toReplace, replacement, -1)
	return suggestion
}

func fixRanges(p lint.Problem, matches []string) string {
	if len(matches) != 2 {
		return ""
	}
	reValuesToOmit := regexp.MustCompile(`for (([ [A-Za-z0-9]+[,]?]?(, _ :?= ))|(_ = )|(_, _ = ))range`)
	valuesToOmit := reValuesToOmit.FindStringSubmatch(p.LineText)
	if len(valuesToOmit) == 0 {
		return ""
	}
	suggestion := strings.Replace(p.LineText, valuesToOmit[0], matches[1], -1)
	return suggestion
}

func fixVarDecl(p lint.Problem, matches []string) string {
	if len(matches) != 2 {
		return ""
	}
	suggestion := strings.Replace(p.LineText, " "+matches[1], "", -1)
	return suggestion
}

func formatSuggestion(s string) string {
	return "```suggestion\n" + s + "```\n"
}
