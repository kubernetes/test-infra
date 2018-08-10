/*

 */

package calc

import (
	"bufio"
	"fmt"
	"github.com/kubernetes/test-infra/coverage/artifacts"
	"log"
	"os"
)

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

func writeLine(file *os.File, content string) {
	if file != nil {
		fmt.Fprintln(file, content)
	}
}
