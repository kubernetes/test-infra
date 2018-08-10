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
	"bufio"
	"fmt"
	"os"
)

// fileElement is an object that holds the writer and the corresponding file behind it.
// The idea is to have both references in order to write to the writer in a buffered fashion
// and then when processing has finished, be able to flush data into the writer and close
// the related file.
type fileElement struct {
	file   *os.File
	writer *bufio.Writer
}

// This returns a function that will return a fileElement. This fileElement will be referencing a file
// whose name will respond to an expression like <prefix>_<counter>.
// "counter" will increase on each invocation.
func newFileElementSequencer(prefixToUse string) func() (*fileElement, error) {
	counter := 0
	prefix := prefixToUse

	return func() (*fileElement, error) {
		counter++
		file, err := os.Create(fmt.Sprintf("%s%d.tmp", prefix, counter))
		if err != nil {
			return nil, err
		}
		return &fileElement{file, bufio.NewWriter(file)}, nil
	}
}

func (fileElement *fileElement) tearDown() {
	if fileElement.writer != nil {
		fileElement.writer.Flush()
	}
	if fileElement.file != nil {
		fileElement.file.Close()
	}
}

func (fileElement *fileElement) writeLine(line string) {
	fmt.Fprintln(fileElement.writer, line)
}

// Split function takes in a file and splits it based on the given separatorLine, into subFiles
// that will folllow <prefix>_<counter> grammar names.
func Split(inputFilePathPtr *string, prefixForSubFilePtr *string, separatorLine string) error {
	file, err := os.Open(*inputFilePathPtr)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	nextFileElement := newFileElementSequencer(*prefixForSubFilePtr)

	fileElement, err := nextFileElement()
	if err != nil {
		return err
	}

	for scanner.Scan() {
		line := scanner.Text()

		if line == separatorLine {
			fileElement.tearDown()
			fileElement, err = nextFileElement()
			if err != nil {
				return err
			}
		}
		fileElement.writeLine(scanner.Text())
	}

	fileElement.tearDown()

	return scanner.Err()
}
