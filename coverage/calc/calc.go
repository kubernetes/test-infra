// package calc calculates coverage through summarizing and also stores all
// the coverage structs used by or produced by the process
package calc

import (
	"github.com/kubernetes/test-infra/coverage/artifacts"
	"os"
	"bufio"
	"log"
	"fmt"
)

// CovList read profiling information from reader and constructs CoverageList.
// If called in presubmit, it also creates a filtered version of profile,
// that only includes files in corresponding github commit,
// less those files that are excluded from coverage calculation
func CovList(f *artifacts.ProfileReader, keyProfileFile *os.File,
	concernedFiles *map[string]bool, covThresInt int) (g *CoverageList) {

	defer f.Close()
	defer keyProfileFile.Close()

	scanner := bufio.NewScanner(f)
	scanner.Scan() // discard first line
	writeLine(keyProfileFile, scanner.Text())

	isPresubmit := concernedFiles != nil
	log.Printf("isPresubmit=%v", isPresubmit)
	log.Printf("concerned Files=%v", concernedFiles)

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
			log.Printf("concerned line: %s", row)
		}
	}

	return
}

// writeLine writes a line in the given file, if the file pointer is not nil
func writeLine(file *os.File, content string) {
	if file != nil {
		fmt.Fprintln(file, content)
	}
}
