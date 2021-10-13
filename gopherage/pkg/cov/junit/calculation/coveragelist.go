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

package calculation

import (
	"path"
	"strings"
)

// CoverageList is a collection and summary over multiple file Coverage objects
type CoverageList struct {
	*Coverage
	Group []Coverage
}

// CovList constructs new (file) Group Coverage
func newCoverageList(name string) *CoverageList {
	return &CoverageList{
		Coverage: &Coverage{Name: name},
		Group:    []Coverage{},
	}
}

// Ratio summarizes the list of coverages and returns the summarized ratio
func (covList *CoverageList) Ratio() float32 {
	covList.summarize()
	return covList.Coverage.Ratio()
}

// summarize summarizes all items in the Group and stores the result
func (covList *CoverageList) summarize() {
	covList.NumCoveredStmts = 0
	covList.NumAllStmts = 0
	for _, item := range covList.Group {
		covList.NumCoveredStmts += item.NumCoveredStmts
		covList.NumAllStmts += item.NumAllStmts
	}
}

// Subset returns the subset obtained through applying filter
func (covList *CoverageList) Subset(prefix string) *CoverageList {
	s := newCoverageList("Filtered Summary: " + prefix)
	for _, c := range covList.Group {
		if strings.HasPrefix(c.Name, prefix) {
			s.Group = append(s.Group, c)
		}
	}
	return s
}

// ListDirectories gets a list of sub-directories that contains source code.
func (covList CoverageList) ListDirectories() []string {
	dirSet := map[string]bool{}
	for _, cov := range covList.Group {
		dirSet[path.Dir(cov.Name)] = true
	}
	var result []string
	for key := range dirSet {
		result = append(result, key)
	}
	return result
}
