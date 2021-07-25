/*
Copyright 2021 The Kubernetes Authors.

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
	"fmt"
	"regexp"
	"strings"
)

var (
	TestHelpRe          = regexp.MustCompile(`(?m)^/test[ \t]*\?\s*$`)
	EmptyTestRe         = regexp.MustCompile(`(?m)^/test\s*$`)
	RetestWithTargetRe  = regexp.MustCompile(`(?m)^/retest[ \t]+\S+`)
	TestWithAnyTargetRe = regexp.MustCompile(`(?m)^/test[ \t]+\S+`)

	TestWithoutTargetNote     = "The `/test` command needs one or more targets.\n"
	RetestWithTargetNote      = "The `/retest` command does not accept any targets.\n"
	TargetNotFoundNote        = "The specified target(s) for `/test` were not found.\n"
	ThereAreNoTestAllJobsNote = "No jobs can be run with `/test all`.\n"
)

func MayNeedHelpComment(body string) bool {
	return EmptyTestRe.MatchString(body) ||
		RetestWithTargetRe.MatchString(body) ||
		TestWithAnyTargetRe.MatchString(body) ||
		TestHelpRe.MatchString(body)
}

func ShouldRespondWithHelp(body string, toRunOrSkip int) (bool, string) {
	switch {
	case TestHelpRe.MatchString(body):
		return true, ""
	case EmptyTestRe.MatchString(body):
		return true, TestWithoutTargetNote
	case RetestWithTargetRe.MatchString(body):
		return true, RetestWithTargetNote
	case toRunOrSkip == 0 && TestAllRe.MatchString(body):
		return true, ThereAreNoTestAllJobsNote
	case toRunOrSkip == 0 && TestWithAnyTargetRe.MatchString(body):
		return true, TargetNotFoundNote
	default:
		return false, ""
	}
}

func HelpMessage(org, repo, branch, note string, testAllNames, optionalTestCommands, requiredTestCommands []string) string {
	var resp string
	if len(testAllNames)+len(optionalTestCommands)+len(requiredTestCommands) == 0 {
		return fmt.Sprintf("No presubmit jobs available for %s/%s@%s", org, repo, branch)
	}

	listBuilder := func(names []string) string {
		var list strings.Builder
		for _, name := range names {
			list.WriteString(fmt.Sprintf("\n* `%s`", name))
		}
		return list.String()
	}

	resp = note
	if len(requiredTestCommands) > 0 {
		resp += fmt.Sprintf("The following commands are available to trigger required jobs:%s\n\n", listBuilder(requiredTestCommands))
	}
	if len(optionalTestCommands) > 0 {
		resp += fmt.Sprintf("The following commands are available to trigger optional jobs:%s\n\n", listBuilder(optionalTestCommands))
	}

	var testAllNote string
	if len(testAllNames) == len(optionalTestCommands)+len(requiredTestCommands) {
		testAllNote = "Use `/test all` to run all jobs.\n"
	} else if len(testAllNames) > 0 {
		testAllNote = fmt.Sprintf("Use `/test all` to run the following jobs that were automatically triggered:%s\n\n", listBuilder(testAllNames))
	}
	resp += testAllNote

	return resp
}
