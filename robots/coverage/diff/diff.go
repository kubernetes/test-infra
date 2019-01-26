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

// Package diff calculates the difference of two coverage lists and produces a collection of
// individual coverage difference. The result is formatted optionally to a table in coverage bot post
package diff

import (
	"k8s.io/test-infra/gopherage/pkg/cov/junit/calculation"
)

// deltaSensitivity is checked against to tell whether the coverage delta is worth reporting. Coverage delta is displayed as a percentage with one decimal place. Any difference smaller than this value will not appear different.
const deltaSensitivity = 0.001

type coverageChange struct {
	name      string
	baseRatio float32
	newRatio  float32
}

func isChangeSignificant(baseRatio, newRatio float32) bool {
	diff := newRatio - baseRatio
	if diff < 0 {
		diff = -diff
	}
	return diff > deltaSensitivity
}

// toMap returns maps the file name to its coverage for faster retrieval
// & membership check
func toMap(g *calculation.CoverageList) map[string]calculation.Coverage {
	m := make(map[string]calculation.Coverage)
	for _, cov := range g.Group {
		m[cov.Name] = cov
	}
	return m
}

// findChanges compares the newList of coverage against the base list and returns the result
func findChanges(baseList *calculation.CoverageList, newList *calculation.CoverageList) []*coverageChange {
	var changes []*coverageChange
	baseFilesMap := toMap(baseList)
	for _, newCov := range newList.Group {
		baseCov, ok := baseFilesMap[newCov.Name]
		var baseRatio float32
		if !ok {
			baseRatio = -1
		} else {
			baseRatio = baseCov.Ratio()
		}
		newRatio := newCov.Ratio()
		if isChangeSignificant(baseRatio, newRatio) {
			changes = append(changes, &coverageChange{
				name:      newCov.Name,
				baseRatio: baseRatio,
				newRatio:  newRatio,
			})
		}
	}
	return changes
}
