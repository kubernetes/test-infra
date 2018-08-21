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
	*Coverage
	group           []Coverage
	concernedFiles  *map[string]bool
	covThresholdInt int
}

// CovList constructs new (file) group Coverage
func NewCoverageList(name string, concernedFiles *map[string]bool,
	covThresholdInt int) *CoverageList {
	return &CoverageList{
		Coverage:        newCoverage(name),
		group:           []Coverage{},
		concernedFiles:  concernedFiles,
		covThresholdInt: covThresholdInt,
	}
}

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
		fmt.Fprintln(f, c.String())
	}
}

// Group returns the collection of file Coverage objects
func (g *CoverageList) Group() *[]Coverage {
	return &g.group
}

// Item returns the Coverage item in group by index
func (g *CoverageList) Item(index int) *Coverage {
	return &g.group[index]
}

func (g *CoverageList) ratio() (float32, error) {
	g.Summarize()
	ratio, err := g.Coverage.Ratio()
	if err != nil {
		log.Fatal(err)
	}
	return ratio, err
}

func (g *CoverageList) Percentage() string {
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
		ratio, err := item.Ratio()
		if err != nil && ratio < threshold {
			return true
		}
	}
	return false
}

func (g *CoverageList) append(c *Coverage) {
	g.group = append(g.group, *c)
}

func (g *CoverageList) size() int {
	return len(g.group)
}

func (g *CoverageList) lastElement() *Coverage {
	return &g.group[g.size()-1]
}

// Subset returns the subset obtained through applying filter
func (g *CoverageList) Subset(prefix string) *CoverageList {
	s := NewCoverageList("Filtered Summary", g.concernedFiles, g.covThresholdInt)
	for _, c := range g.group {
		if strings.HasPrefix(c.Name(), prefix) {
			s.append(&c)
		}
	}
	s.Summarize()
	return s
}

// Map returns maps the file name to its coverage for faster retrieval
// & membership check
func (g *CoverageList) Map() map[string]Coverage {
	m := make(map[string]Coverage)
	for _, c := range g.group {
		m[c.Name()] = c
	}
	return m
}

// Report summarizes overall coverage and then prints report
func (g *CoverageList) Report(itemized bool) {
	if itemized {
		for _, item := range g.group {
			fmt.Println(item.String())
		}
	}
	g.Summarize()
	fmt.Printf("summarized ratio: %v\n", g.String())
}
