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
	"fmt"
	"sync"
)

// ProcessFileFunction is an alias for a function that will process the file
// given as an argument.
type ProcessFileFunction func(fileName string)

// quantity of workers to use in processing
var workersQuantity int
var fileNameWaitingGroup sync.WaitGroup
var fileNameChannel = make(chan string)

// SetupWorkGroup will set up the workers creating as many workers asthe quantity given as an argument.
// Each worker will use the given ProcessFileFunction to apply it against
func SetupWorkGroup(workersQuantityArg int, processFile ProcessFileFunction) error {
	if workersQuantityArg <= 0 {
		return fmt.Errorf("only positive numbers are allowed for workersQuantity, passed:[%d]", workersQuantityArg)
	}
	workersQuantity = workersQuantityArg

	for i := 0; i < workersQuantity; i++ {
		go worker(processFile)
	}
	return nil
}

// AddFile will add a file to be added in the fileChannel and manages the logic
// to increment the waiting group that controls that there were items sent
// into the fileChannel and need to be processed.
func AddFile(fileName string) {
	fileNameWaitingGroup.Add(1)
	fileNameChannel <- fileName
}

// WaitUntilWorkGroupFinished will close the channel (since no more items will
// be sent) and wait until all pending items are finished.
func WaitUntilWorkGroupFinished() {
	close(fileNameChannel)
	fileNameWaitingGroup.Wait()
}

// A worker will:
// 1) consume from a fileNameChannel a fileName
// 2) process an individual file (this is a step that can be done independently)
// 3) Notifies the item was finished by decrementing a waitingGroup.
func worker(processFile ProcessFileFunction) {
	for fileName := range fileNameChannel {
		processFile(fileName)
		fileNameWaitingGroup.Done()
	}
}
