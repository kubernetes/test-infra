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

// Package calc calculates & summarized code coverage from coverage profile
package calc

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"k8s.io/test-infra/coverage/git"
	"k8s.io/test-infra/coverage/githubUtil"
	"k8s.io/test-infra/coverage/str"
)

type codeBlock struct {
	fileName      string // the file the code block is in
	numStatements int    // number of statements in the code block
	coverageCount int    // number of times the block is covered
}

func (blk *codeBlock) filePathInGithub() string {
	return githubUtil.FilePathProfileToGithub(blk.fileName)
}

// add blk Coverage to file Coverage
func (blk *codeBlock) addToFileCov(coverage *coverage) {

	coverage.nAllStmts += blk.numStatements
	if blk.coverageCount > 0 {
		coverage.nCoveredStmts += blk.numStatements
	}
}

// add blk Coverage to file group Coverage; return true if the row is concerned
func updateConcernedFiles(concernedFiles *map[string]bool, filePath string,
	isPresubmit bool) (isConcerned bool) {
	// get linguist generated attribute value for the file.
	// If true => needs to be skipped for coverage.
	isConcerned, exists := (*concernedFiles)[filePath]

	if !exists {
		if isPresubmit {
			// presubmit already have concerned files defined,
			// we don't need to check git attributes here
			isConcerned = false
			return
		}
		isConcerned = !git.IsCoverageSkipped(filePath)
		(*concernedFiles)[filePath] = isConcerned
	}
	return
}

// add blk Coverage to file group Coverage; return true if the row is concerned
func (blk *codeBlock) addToGroupCov(g *CoverageList) (isConcerned bool) {
	if g.size() == 0 || g.lastElement().Name() != blk.fileName {
		// when a new file name is processed
		cov := newCoverage(blk.fileName)
		g.append(cov)
	}
	blk.addToFileCov(g.lastElement())
	return true
}

// convert a line in profile file to a codeBlock struct
func toBlock(line string) (res *codeBlock) {
	slice := strings.Split(line, " ")
	blockName := slice[0]
	nStmts, _ := strconv.Atoi(slice[1])
	coverageCount, _ := strconv.Atoi(slice[2])
	return &codeBlock{
		fileName:      blockName[:strings.Index(blockName, ":")],
		numStatements: nStmts,
		coverageCount: coverageCount,
	}
}

// coverage stores test coverage summary data for one file
type coverage struct {
	name          string
	nCoveredStmts int
	nAllStmts     int
	lineCovLink   string
}

func newCoverage(name string) *coverage {
	return &coverage{name, 0, 0, ""}
}

// Name returns the file name
func (c *coverage) Name() string {
	return c.name
}

// percentage returns the percentage of statements covered
func (c *coverage) percentage() string {
	ratio, err := c.ratio()
	if err == nil {
		return str.PercentStr(ratio)
	}

	return "N/A"
}

// PercentageForTestgrid returns the percentage of statements covered
func (c *coverage) PercentageForTestgrid() string {
	ratio, err := c.ratio()
	if err == nil {
		return str.PercentageForTestgrid(ratio)
	}

	return ""
}

func (c *coverage) ratio() (float32, error) {
	if c.nAllStmts == 0 {
		return -1, fmt.Errorf("[%s] has 0 statement", c.Name())
	}
	return float32(c.nCoveredStmts) / float32(c.nAllStmts), nil
}

// String returns the summary of coverage in string
func (c *coverage) string() string {
	ratio, err := c.ratio()
	if err == nil {
		return fmt.Sprintf("[%s]\t%s (%d of %d stmts) covered", c.Name(),
			str.PercentStr(ratio), c.nCoveredStmts, c.nAllStmts)
	}
	return "ratio not exist"
}

//LineCovLink returns the link to line coverage html
func (c *coverage) LineCovLink() string {
	return c.lineCovLink
}

//SetLineCovLink sets the link to line coverage html
func (c *coverage) SetLineCovLink(link string) {
	c.lineCovLink = link
}

// IsCoverageLow checks if the coverage is less than the threshold.
func (c *coverage) IsCoverageLow(covThresholdInt int) bool {
	covThreshold := float32(covThresholdInt) / 100
	ratio, err := c.ratio()
	if err == nil {
		return ratio < covThreshold
	}
	return false
}

//SortCoverages sorts coverage based on name alphabetically
func SortCoverages(cs []coverage) {
	sort.Slice(cs, func(i, j int) bool {
		return cs[i].Name() < cs[j].Name()
	})
}
