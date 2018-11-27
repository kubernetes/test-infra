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
	"github.com/golang/lint"
	"regexp"
	"strings"
)

var (
	lintNamesUnderscoreRegex = regexp.MustCompile("don't use underscores in Go names; (.*) should be (.*)")
	lintNamesAllCapsRegex    = regexp.MustCompile("don't use ALL_CAPS in Go names; use CamelCase")
	lintStutter              = regexp.MustCompile("name will be used as [^.]+\\.(.*) by other packages, and that stutters; consider calling this (.*)")
)

var lintHandlers = [...]func(lint.Problem) string{
	fixNameUnderscore,
	fixNameAllCaps,
	fixStutter,
}

// SuggestCodeChange returns code suggestions for a given lint.Problem
// Returns empty string if no suggestion can be given
func SuggestCodeChange(p lint.Problem) string {
	var suggestion = ""
	for _, handler := range lintHandlers {
		suggestion = handler(p)
		if suggestion != "" {
			return formatSuggestion(suggestion)
		}
	}
	return ""
}

func fixNameUnderscore(p lint.Problem) string {
	matches := lintNamesUnderscoreRegex.FindStringSubmatch(p.Text)
	if len(matches) < 3 {
		return ""
	}
	underscoreRe := regexp.MustCompile(`[A-Za-z]+(_[A-Za-z0-9]+)+`)
	namesWithUnderscore := underscoreRe.FindStringSubmatch(matches[1])
	suggestion := strings.Replace(p.LineText, namesWithUnderscore[0], matches[2], -1)
	if suggestion == p.LineText {
		return ""
	}
	return suggestion
}

func fixNameAllCaps(p lint.Problem) string {
	result := ""
	matches := lintNamesAllCapsRegex.FindStringSubmatch(p.Text)
	if len(matches) == 0 {
		return result
	}
	// Identify all caps names
	reAllCaps := regexp.MustCompile(`[A-Z]+(_[A-Z0-9]+)*`)
	result = reAllCaps.ReplaceAllStringFunc(p.LineText, func(oldName string) string {
		return strings.Replace(strings.Title(strings.Replace(strings.ToLower(oldName), "_", " ", -1)), " ", "", -1)
	})
	if result == p.LineText {
		return ""
	}
	return result
}

func fixStutter(p lint.Problem) string {
	matches := lintStutter.FindStringSubmatch(p.Text)
	if len(matches) < 3 {
		return ""
	}
	suggestion := strings.Replace(p.LineText, matches[1], matches[2], -1)
	if suggestion == p.LineText {
		return ""
	}
	return suggestion
}

func formatSuggestion(s string) string {
	return "```suggestion\n" + s + "```\n"
}
