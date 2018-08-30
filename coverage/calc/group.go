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

package calc

import (
	"fmt"
	"log"
	"os"
	"strings"

	"k8s.io/test-infra/coverage/str"
)

// CoverageList is a collection and summary over multiple file Coverage objects
type CoverageList struct {
	*coverage
	group           []coverage
	concernedFiles  *map[string]bool
	covThresholdInt int
}

// CovList constructs new (file) group Coverage
func newCoverageList(name string, concernedFiles *map[string]bool,
	covThresholdInt int) *CoverageList {
	return &CoverageList{
		coverage:        newCoverage(name),
		group:           []coverage{},
		concernedFiles:  concernedFiles,
		covThresholdInt: covThresholdInt,
	}
}

//CovThresInt gets coverage threshold (as a integer between 0 to 100)
func (g *CoverageList) CovThresInt() int {
	return g.covThresholdInt
}

// writeToFile writes file level coverage in a file
func (g *CoverageList) writeToFile(filePath string) {
	f, err := os.Create(filePath)
	if err != nil {
		log.Fatal("Cannot create file", err)
	}
	defer f.Close()
	for _, c := range *g.Group() {
		fmt.Fprintln(f, c.string())
	}
}

// Group returns the collection of file Coverage objects
func (g *CoverageList) Group() *[]coverage {
	return &g.group
}

// Item returns the Coverage item in group by index
func (g *CoverageList) Item(index int) *coverage {
	return &g.group[index]
}

func (g *CoverageList) ratio() (float32, error) {
	g.Summarize()
	ratio, err := g.coverage.ratio()
	if err != nil {
		log.Fatal(err)
	}
	return ratio, err
}

func (g *CoverageList) percentage() string {
	ratio, _ := g.ratio()
	return str.PercentStr(ratio)
}

// Summarize summarizes all items in the group and stores the result
func (g *CoverageList) Summarize() {
	for _, item := range g.group {
		g.nCoveredStmts += item.nCoveredStmts
		g.nAllStmts += item.nAllStmts
	}
}

// hasCoverageBelowThreshold checks whether any item in the list has a
// coverage below the threshold
func (g *CoverageList) hasCoverageBelowThreshold(threshold float32) bool {
	for _, item := range g.group {
		ratio, err := item.ratio()
		if err != nil && ratio < threshold {
			return true
		}
	}
	return false
}

func (g *CoverageList) append(c *coverage) {
	g.group = append(g.group, *c)
}

func (g *CoverageList) size() int {
	return len(g.group)
}

func (g *CoverageList) lastElement() *coverage {
	return &g.group[g.size()-1]
}

// Subset returns the subset obtained through applying filter
func (g *CoverageList) Subset(prefix string) *CoverageList {
	s := newCoverageList("Filtered Summary", g.concernedFiles, g.covThresholdInt)
	for _, c := range g.group {
		if strings.HasPrefix(c.Name(), prefix) {
			s.append(&c)
		}
	}
	s.Summarize()
	return s
}

// toMap returns maps the file name to its coverage for faster retrieval
// & membership check
func (g *CoverageList) toMap() map[string]coverage {
	m := make(map[string]coverage)
	for _, c := range g.group {
		m[c.Name()] = c
	}
	return m
}

// Report summarizes overall coverage and then prints report
func (g *CoverageList) report(itemized bool) {
	if itemized {
		for _, item := range g.group {
			fmt.Println(item.string())
		}
	}
	g.Summarize()
	fmt.Printf("summarized ratio: %v\n", g.string())
}
