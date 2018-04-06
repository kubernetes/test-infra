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

package clonerefs

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"sync"

	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pod-utils/clone"
)

// Run clones the configured refs
func (o Options) Run() error {
	var numWorkers int
	if o.MaxParallelWorkers != 0 {
		numWorkers = o.MaxParallelWorkers
	} else {
		numWorkers = len(o.GitRefs)
	}

	wg := &sync.WaitGroup{}
	wg.Add(numWorkers)

	input := make(chan *kube.Refs)
	output := make(chan clone.Record, len(o.GitRefs))
	for i := 0; i < numWorkers; i++ {
		go func() {
			defer wg.Done()
			for ref := range input {
				output <- clone.Run(ref, o.SrcRoot, o.GitUserName, o.GitUserEmail)
			}
		}()
	}

	for _, ref := range o.GitRefs {
		input <- ref
	}

	close(input)
	wg.Wait()
	close(output)

	var results []clone.Record
	for record := range output {
		results = append(results, record)
	}

	logData, err := json.Marshal(results)
	if err != nil {
		return fmt.Errorf("failed to marshal clone records: %v", err)
	}

	if err := ioutil.WriteFile(o.Log, logData, 0755); err != nil {
		return fmt.Errorf("failed to write clone records: %v", err)
	}

	return nil
}
