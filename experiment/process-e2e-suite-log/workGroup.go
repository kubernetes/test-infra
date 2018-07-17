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
	"errors"
	"fmt"
	"sync"
)

type ProcessFileFunction func(fileName string)

// quantity of workers to use in processing
var workersQuantity int
var fileNameWaitingGroup sync.WaitGroup
var fileNameChannel chan string = make(chan string)

func SetupWorkGroup(workersQuantityArg int, processFile ProcessFileFunction) error {
	if workersQuantityArg <= 0 {
		return errors.New(fmt.Sprintf("only positive numbers are allowed for workersQuantity, passed:[%d]", workersQuantityArg))
	}
	workersQuantity = workersQuantityArg

	for i := 0; i < workersQuantity; i++ {
		go worker(processFile)
	}
	return nil
}

func AddFile(fileName string) {
	fileNameWaitingGroup.Add(1)
	fileNameChannel <- fileName
}

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
