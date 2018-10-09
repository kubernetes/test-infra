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

// Package calc calculates coverage through summarizing and also stores all
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
	isPresubmit bool, concernedFiles *map[string]bool, covThresInt int) (g *CoverageList) {

	defer f.Close()
	if keyProfileFile != nil {
		defer keyProfileFile.Close()
	}

	scanner := bufio.NewScanner(f)
	scanner.Scan() // discard first line
	writeLine(keyProfileFile, scanner.Text())

	logrus.Infof("isPresubmit=%v", isPresubmit)
	logrus.Infof("concerned files=%v", concernedFiles)

	g = newCoverageList("localSummary", concernedFiles, covThresInt)
	for scanner.Scan() {
		row := scanner.Text()
		blk := toBlock(row)
		isConcerned := updateConcernedFiles(concernedFiles,
			blk.filePathInGithub(), isPresubmit)
		if isConcerned {
			blk.addToGroupCov(g)
			writeLine(keyProfileFile, row)
		}
	}

	logrus.Infof("updated concerned files=%v", concernedFiles)

	return
}

// writeLine writes a line in the given file, if the file pointer is not nil
func writeLine(file io.Writer, content string) {
	if file != nil {
		fmt.Fprintln(file, content)
	}
}
