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

type FileElement struct {
	file   *os.File
	writer *bufio.Writer
}

// This returns a function that will return a fileElement. This fileElement will be referencing a file
// whose name will respond to an expression like <prefix>_<counter>.
// "counter" will increase on each invocation.
func newFileElementSequencer(prefixToUse string) func() (*FileElement, error) {
	counter := 0
	prefix := prefixToUse

	return func() (*FileElement, error) {
		counter += 1
		file, err := os.Create(fmt.Sprintf("%s_%d.tmp", prefix, counter))
		if err != nil {
			return nil, err
		}
		return &FileElement{file, bufio.NewWriter(file)}, nil
	}
}

func (fileElement *FileElement) tearDown() {
	if fileElement.writer != nil {
		fileElement.writer.Flush()
	}
	if fileElement.file != nil {
		fileElement.file.Close()
	}
}

func (fileElement *FileElement) writeLine(line string) {
	fmt.Fprintln(fileElement.writer, line)
}

// takes in a file and splits it based on the given separatorLine, into subFiles
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
	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}
