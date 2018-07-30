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

// This package is in charge of processing the master file and generating a result file.
package main

import (
	"errors"
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

type options struct {
	// input file path.
	inputFilePathPtr *string

	// separator line to detect each section of the file.
	separatorLine string

	// prefix used for the sub files generated.
	prefixForSubFilePtr *string

	// tells if splitting step has to be processed or not.
	skipSplittingPtr *bool

	// the quantity of workers to use
	workersQuantityPtr *int

	// to access the endpoints map in an repeteable manner we need an ordered array.
	keys []string

	// result file name.
	resultFileNamePtr *string

	// separator character to use in result file.
	separatorCharacterPtr *string

	// this replacer is to escape the text picked in the test description, in order to replace the ocurrences of
	// the character separator by space and not break the columns
	replacer *strings.Replacer

	// regex to pick individual lines from the complete text of the section.
	// This will be later used to pick specific lines to extract test description.
	lineRegEx *regexp.Regexp

	// rowMapper
	rowMapper rowMapper
}

func createHeader(keys []string) *rowData {
	rowDataPtr := newRowData("filename", "testDescription")
	for _, key := range keys {
		rowDataPtr.addColumn(key, key)
	}
	return rowDataPtr
}

func generateResultFile(optionsPtr *options) {
	AddRow(optionsPtr.rowMapper.toRow(createHeader(optionsPtr.keys)))

	root, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
		return
	}

	filepath.Walk(root, func(path string, f os.FileInfo, _ error) error {
		if !f.IsDir() {
			match, err := regexp.MatchString(*(optionsPtr.prefixForSubFilePtr), f.Name())
			if err == nil && match {
				AddFile(f.Name())
			}
		}
		return nil
	})

	WaitUntilWorkGroupFinished()
	WaitUntilResultWriterFinished()
}

func generateProcessFileFunction(optionsPtr *options) ProcessFileFunction {
	options := optionsPtr
	// process individual file and generate a row.
	return func(fileName string) {
		fileContent, err := ioutil.ReadFile(fileName)

		if err != nil {
			log.Fatal(err)
			return
		}

		result := options.lineRegEx.FindAll(fileContent, 4)

		if len(result) < 4 {
			log.Printf("skipped file:[%s]", fileName)

		} else {
			line3 := options.replacer.Replace(string(result[2]))
			line4 := options.replacer.Replace(string(result[3]))

			rowDataPtr := newRowData(fileName, fmt.Sprintf("%s%s", line3, line4))

			for _, key := range options.keys {
				matched, err := regexp.Match(endpoints[key], fileContent)
				if err != nil {
					log.Fatal(err)
				}
				if matched {
					rowDataPtr.addColumn(key, "Y")
				}
			}
			AddRow(options.rowMapper.toRow(rowDataPtr))
		}
	}
}

// sets up required elements for processing.
// 1) invoking flag parsing
// 2) "keys":
// * fills "keys" visiting endpoints map in a sorted way (since maps is not sorted).
// * This will later be used to generate each line of the result file.
// 3) generates the selected amount of workers.
// 4) sets up the result writer.
// 5) sets up the rowMapper
func setup() (*options, error) {
	options := options{}
	err := processFlags(&options)
	if err != nil {
		return nil, err
	}

	options.replacer = strings.NewReplacer(*(options.separatorCharacterPtr), "")
	options.separatorLine = "----------------"
	options.lineRegEx = regexp.MustCompile(`(?m)^.*$`)

	var keys []string
	for k := range endpoints {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	options.keys = keys

	processFileFunction := generateProcessFileFunction(&options)
	err = SetupWorkGroup(*(options.workersQuantityPtr), processFileFunction)
	if err != nil {
		return nil, err
	}
	err = SetupFileWriter(*(options.resultFileNamePtr))
	if err != nil {
		return nil, err
	}

	options.rowMapper = newCSVRowMapper(options.separatorCharacterPtr, options.keys)

	fmt.Println("> input-file-path:", *(options.inputFilePathPtr))
	fmt.Println("> prefix-sub-file:", *(options.prefixForSubFilePtr))
	fmt.Println("> workers:", *(options.workersQuantityPtr))
	fmt.Println("> result-file-name:", *(options.resultFileNamePtr))
	fmt.Println("> separator:", *(options.separatorCharacterPtr))

	return &options, nil
}

// processes the mandatory and optional flags from the command line.
func processFlags(options *options) error {
	options.inputFilePathPtr = flag.String("input-file-path", "", "file path for the input file")
	options.resultFileNamePtr = flag.String("result-file-name", "", "file name for the output")

	options.prefixForSubFilePtr = flag.String("prefix-sub-file", "res_", "prefix for sub files")
	options.skipSplittingPtr = flag.Bool("skip-splitting", false, "skip splitting step of the input file")
	options.workersQuantityPtr = flag.Int("workers", 2, "workers quantity")
	options.separatorCharacterPtr = flag.String("separator", ",", "separator character for output file")

	flag.Parse()

	if *(options.inputFilePathPtr) == "" {
		return errors.New("[mandatory flag missing] input-file-path is a mandatory flag, please provide and try again")
	}
	if *(options.resultFileNamePtr) == "" {
		return errors.New("[mandatory flag missing] result-file-name is a mandatory flag, please provide and try again")
	}
	return nil
}

func main() {
	optionsPtr, err := setup()
	if err != nil {
		log.Fatal(err)
		return
	}

	if !*(optionsPtr.skipSplittingPtr) {
		err = Split(optionsPtr.inputFilePathPtr, optionsPtr.prefixForSubFilePtr, optionsPtr.separatorLine)
		if err != nil {
			log.Fatal(err)
			return
		}
	}
	generateResultFile(optionsPtr)
}
