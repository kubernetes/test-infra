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
