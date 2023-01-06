/*
Copyright 2019 The Kubernetes Authors.

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

package git

import (
	"fmt"
	"strings"
)

// fakeResolver allows for simple injections in tests
type fakeResolver struct {
	out string
	err error
}

func (r *fakeResolver) Resolve() (string, error) {
	return r.out, r.err
}

type execResponse struct {
	out []byte
	err error
}

// fakeExecutor is useful in testing for mocking an Executor
type fakeExecutor struct {
	records   [][]string
	responses map[string]execResponse
}

func (e *fakeExecutor) Run(args ...string) ([]byte, error) {
	e.records = append(e.records, args)
	key := strings.Join(args, " ")
	if response, ok := e.responses[key]; ok {
		return response.out, response.err
	}
	return []byte{}, fmt.Errorf("no response configured for %s", key)
}
