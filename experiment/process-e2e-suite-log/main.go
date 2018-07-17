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

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// input file path.
var inputFilePathPtr *string

// separator line to detect each section of the file.
var separatorLine string = "----------------"

// prefix used for the sub files generated.
var prefixForSubFilePtr *string

// tells if splitting step has to be processed or not.
var skipSplittingPtr *bool

// the quantity of workers to use
var workersQuantityPtr *int

// to access the endpoints map in an repeteable manner we need an ordered array.
var keys []string

// result file name.
var resultFileNamePtr *string

// separator character to use in result file.
var separatorCharacterPtr *string

// this replacer is to escape the text picked in the test description, in order to replace the ocurrences of
// the character separator by space and not break the columns
var replacer *strings.Replacer

func createHeader() string {
	var headerBuffer bytes.Buffer
	headerBuffer.WriteString("filename")
	headerBuffer.WriteString(*separatorCharacterPtr)
	headerBuffer.WriteString("testDescription")

	for _, key := range keys {
		headerBuffer.WriteString(*separatorCharacterPtr)
		headerBuffer.WriteString(key)
	}
	return headerBuffer.String()
}

func generateResultFile() {
	AddRow(createHeader())

	root, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
		return
	}

	filepath.Walk(root, func(path string, f os.FileInfo, _ error) error {
		if !f.IsDir() {
			match, err := regexp.MatchString(*prefixForSubFilePtr, f.Name())
			if err == nil && match {
				AddFile(f.Name())
			}
		}
		return nil
	})

	WaitUntilWorkGroupFinished()
	WaitUntilResultWriterFinished()
}

// regex to pick individual lines from the complete text of the section.
// This will be later used to pick specific lines to extract test description.
var lineRegEx *regexp.Regexp = regexp.MustCompile(`(?m)^.*$`)

// process individual file and generate a row.
func processFile(fileName string) {
	fileContent, err := ioutil.ReadFile(fileName)

	if err != nil {
		log.Fatal(err)
		return
	}

	var rowBuffer bytes.Buffer
	rowBuffer.WriteString(fileName)
	rowBuffer.WriteString(*separatorCharacterPtr)

	result := lineRegEx.FindAll(fileContent, 4)

	if len(result) < 4 {
		log.Printf("skipped file:[%s]", fileName)

	} else {
		line3 := replacer.Replace(string(result[2]))
		line4 := replacer.Replace(string(result[3]))

		rowBuffer.WriteString(fmt.Sprintf("%s%s", line3, line4))

		for _, key := range keys {
			matched, err := regexp.Match(endpoints[key], fileContent)
			if err != nil {
				log.Fatal(err)
			}
			rowBuffer.WriteString(*separatorCharacterPtr)
			if matched {
				rowBuffer.WriteString("Y")
			}
		}
		AddRow(rowBuffer.String())
	}
}

// sets up required elements for processing.
// 1) "keys":
// * fills "keys" visiting endpoints map in a sorted way (since maps is not sorted).
// * This will later be used to generate each line of the result file.
// 2) generates the selected amount of workers.
// 3) sets up the result writer.
func setup() {
	for k := range endpoints {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	err := SetupWorkGroup(*workersQuantityPtr, processFile)
	if err != nil {
		log.Fatal(err)
		return
	}
	err = SetupFileWriter(*resultFileNamePtr)
	if err != nil {
		log.Fatal(err)
		return
	}
}

// processes the mandatory and optional flags from the command line.
func processFlags() {
	inputFilePathPtr = flag.String("input-file-path", "", "file path for the input file")

	prefixForSubFilePtr = flag.String("prefix-sub-file", "res", "prefix for sub files")
	skipSplittingPtr = flag.Bool("skip-splitting", false, "skip splitting step of the input file")

	workersQuantityPtr = flag.Int("workers", 2, "workers quantity")

	resultFileNamePtr = flag.String("result-file-name", "", "file name for the output")
	separatorCharacterPtr = flag.String("separator", ",", "separator character for output file")

	flag.Parse()

	if *inputFilePathPtr == "" {
		log.Fatal("[mandatory flag missing] input-file-path is a mandatory flag, please provide and try again.")
	}
	if *resultFileNamePtr == "" {
		log.Fatal("[mandatory flag missing] result-file-name is a mandatory flag, please provide and try again.")
	}

	replacer = strings.NewReplacer(*separatorCharacterPtr, "")

	fmt.Println("> input-file-path:", *inputFilePathPtr)
	fmt.Println("> prefix-sub-file:", *prefixForSubFilePtr)
	fmt.Println("> workers:", *workersQuantityPtr)
	fmt.Println("> result-file-name:", *resultFileNamePtr)
	fmt.Println("> separator:", *separatorCharacterPtr)
}

func main() {
	processFlags()
	setup()
	if !*skipSplittingPtr {
		err := Split(inputFilePathPtr, prefixForSubFilePtr, separatorLine)
		if err != nil {
			log.Fatal(err)
			return
		}
	}
	generateResultFile()
}
