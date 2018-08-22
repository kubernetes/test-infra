// package calc calculates coverage through summarizing and also stores all
// the coverage structs used by or produced by the process
package calc

import (
	"bufio"
	"fmt"
	"io"

	"github.com/sirupsen/logrus"
)

// CovList read profiling information from reader and constructs CoverageList.
// If called in presubmit, it also creates a filtered version of profile,
// that only includes files in corresponding github commit,
// less those files that are excluded from coverage calculation
func CovList(f io.ReadCloser, keyProfileFile io.WriteCloser,
	concernedFiles *map[string]bool, covThresInt int) (g *CoverageList) {

	defer f.Close()
	if keyProfileFile != nil {
		defer keyProfileFile.Close()
	}

	scanner := bufio.NewScanner(f)
	scanner.Scan() // discard first line
	writeLine(keyProfileFile, scanner.Text())

	isPresubmit := concernedFiles != nil
	logrus.Infof("isPresubmit=%v", isPresubmit)
	logrus.Infof("concerned Files=%v", concernedFiles)

	if !isPresubmit {
		concernedFiles = &map[string]bool{}
	}

	g = NewCoverageList("localSummary", concernedFiles, covThresInt)
	for scanner.Scan() {
		row := scanner.Text()
		blk := toBlock(row)
		isConcerned := updateConcernedFiles(concernedFiles,
			blk.filePathInGithub(), isPresubmit)
		if isConcerned {
			blk.addToGroupCov(g)
			writeLine(keyProfileFile, row)
			logrus.Infof("concerned line: %s", row)
		}
	}

	return
}

// writeLine writes a line in the given file, if the file pointer is not nil
func writeLine(file io.Writer, content string) {
	if file != nil {
		fmt.Fprintln(file, content)
	}
}
