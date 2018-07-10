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
	"sync"
)

var rowWaitingGroup sync.WaitGroup
var rowChannel chan string = make(chan string)

func AddRow(row string) {
	rowWaitingGroup.Add(1)
	rowChannel <- row
}

func WaitUntilResultWriterFinished() {
	close(rowChannel)
	rowWaitingGroup.Wait()
}

func SetupFileWriter(resultFileName string) error {
	resultFile, err := os.Create(resultFileName)
	if err != nil {
		return err
	}
	go write(resultFile)
	return nil
}

func write(resultFile *os.File) {
	writer := bufio.NewWriter(resultFile)
	for row := range rowChannel {
		fmt.Fprintln(writer, row)
		writer.Flush()
		rowWaitingGroup.Done()
	}
	resultFile.Close()
}
