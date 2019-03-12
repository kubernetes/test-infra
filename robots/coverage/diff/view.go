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

package diff

import (
	"fmt"
	"strings"

	"golang.org/x/tools/cover"
	"k8s.io/test-infra/gopherage/pkg/cov/junit/calculation"
)

// formatPercentage converts a coverage ratio into a string value to be displayed by coverage robot
func formatPercentage(ratio float32) string {
	if ratio < 0 {
		return "Does not exist"
	}
	return fmt.Sprintf("%.1f%%", ratio*100)
}

// deltaDisplayed converts a coverage ratio delta into a string value to be displayed by coverage robot
func deltaDisplayed(change *coverageChange) string {
	if change.baseRatio < 0 {
		return ""
	}
	return fmt.Sprintf("%.1f", (change.newRatio-change.baseRatio)*100)
}

// makeTable checks each coverage change and produce the table content for coverage bot post
// It also report on whether any coverage fells below the given threshold
func makeTable(baseCovList, newCovList *calculation.CoverageList, coverageThreshold float32) (string, bool) {
	var rows []string
	isCoverageLow := false
	for _, change := range findChanges(baseCovList, newCovList) {
		filePath := change.name
		rows = append(rows, fmt.Sprintf("%s | %s | %s | %s",
			filePath,
			formatPercentage(change.baseRatio),
			formatPercentage(change.newRatio),
			deltaDisplayed(change)))

		if change.newRatio < coverageThreshold {
			isCoverageLow = true
		}
	}
	return strings.Join(rows, "\n"), isCoverageLow
}

// ContentForGithubPost constructs the message covbot posts
func ContentForGithubPost(baseProfiles, newProfiles []*cover.Profile, jobName string, coverageThreshold float32) (
	string, bool) {

	rows := []string{
		"The following is the code coverage report",
		fmt.Sprintf("Say `/test %s` to re-run this coverage report", jobName),
		"",
		"File | Old Coverage | New Coverage | Delta",
		"---- |:------------:|:------------:|:-----:",
	}

	table, isCoverageLow := makeTable(calculation.ProduceCovList(baseProfiles), calculation.ProduceCovList(newProfiles), coverageThreshold)

	if table == "" {
		return "", false
	}

	rows = append(rows, table)
	rows = append(rows, "")

	return strings.Join(rows, "\n"), isCoverageLow
}
