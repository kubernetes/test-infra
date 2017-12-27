/*
Copyright 2016 The Kubernetes Authors.

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

package helpers

import (
	"fmt"
	"strings"
	"sync"
)

const errorTopNum = 5

// ErrorStats analyzes the errors occurred during the benchmark
type ErrorStats struct {
	sync.RWMutex
	errs  map[string][]string
	enum  map[string]int
	total map[string]int
}

func newErrorStats() *ErrorStats {
	return &ErrorStats{
		errs:  make(map[string][]string),
		enum:  make(map[string]int),
		total: make(map[string]int),
	}
}

func (e *ErrorStats) hasError() bool {
	e.RLock()
	defer e.RUnlock()
	return len(e.errs) == 0
}

func (e *ErrorStats) add(l string, err error) {
	e.Lock()
	defer e.Unlock()
	e.total[l] = e.total[l] + 1
	if err != nil {
		e.enum[l] = e.enum[l] + 1
		// TODO(random-liu): Make the error top num configurable
		if e.enum[l] <= errorTopNum {
			e.errs[l] = append(e.errs[l], err.Error())
		}
	}
}

func (e *ErrorStats) stats() string {
	e.RLock()
	defer e.RUnlock()
	var str string
	if len(e.errs) == 0 {
		return str
	}
	for l, errList := range e.errs {
		str += fmt.Sprintf("%s error: \n", l)
		str += strings.Join(errList, "\n") + "\n"
		if e.enum[l] > errorTopNum {
			str += fmt.Sprintf("... %d more errors\n", e.enum[l]-errorTopNum)
		}
		str += fmt.Sprintf("error rate %02f%%\n\n", e.rate(l)*100)
	}
	return str
}

// rate should be used with lock protection
func (e *ErrorStats) rate(l string) float64 {
	if e.total[l] == 0 {
		return 0.0
	}
	return float64(e.enum[l]) / float64(e.total[l])
}
