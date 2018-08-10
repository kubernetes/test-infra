/*
This file stores the main structs and their methods used by the coverage app
*/
package calc

import (
	"fmt"
	"github.com/kubernetes/test-infra/coverage/git"
	"github.com/kubernetes/test-infra/coverage/githubUtil"
	"github.com/kubernetes/test-infra/coverage/str"
	"sort"
	"strconv"
	"strings"
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
func (blk *codeBlock) addToFileCov(coverage *Coverage) {

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
		coverage := newCoverage(blk.fileName)
		g.append(coverage)
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

// Coverage stores test coverage summary data for one file
type Coverage struct {
	name          string
	nCoveredStmts int
	nAllStmts     int
	lineCovLink   string
}

func newCoverage(name string) *Coverage {
	return &Coverage{name, 0, 0, ""}
}

// Name returns the file name
func (c *Coverage) Name() string {
	return c.name
}

// Percentage returns the percentage of statements covered
func (c *Coverage) Percentage() string {
	ratio, err := c.Ratio()
	if err == nil {
		return str.PercentStr(ratio)
	}

	return "N/A"
}

// PercentageForTestgrid returns the percentage of statements covered
func (c *Coverage) PercentageForTestgrid() string {
	ratio, err := c.Ratio()
	if err == nil {
		return str.PercentageForTestgrid(ratio)
	}

	return ""
}

func (c *Coverage) Ratio() (ratio float32, err error) {
	if c.nAllStmts == 0 {
		err = fmt.Errorf("[%s] has 0 statement", c.Name())
	} else {
		ratio = float32(c.nCoveredStmts) / float32(c.nAllStmts)
	}
	return
}

// String returns the summary of coverage in string
func (c *Coverage) String() string {
	ratio, err := c.Ratio()
	if err == nil {
		return fmt.Sprintf("[%s]\t%s (%d of %d stmts) covered", c.Name(),
			str.PercentStr(ratio), c.nCoveredStmts, c.nAllStmts)
	}
	return "ratio not exist"
}

func (c *Coverage) LineCovLink() string {
	return c.lineCovLink
}

func (c *Coverage) SetLineCovLink(link string) {
	c.lineCovLink = link
}

// IsCoverageLow checks if the coverage is less than the threshold.
func (c *Coverage) IsCoverageLow(covThresholdInt int) bool {
	covThreshold := float32(covThresholdInt) / 100
	ratio, err := c.Ratio()
	if err == nil {
		return ratio < covThreshold
	}
	return true
}

func SortCoverages(cs []Coverage) {
	sort.Slice(cs, func(i, j int) bool {
		return cs[i].Name() < cs[j].Name()
	})
}
