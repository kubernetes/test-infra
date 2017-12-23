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

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"k8s.io/kubernetes/test/e2e/perftype"
)

// states of parsing machine
const (
	scanning   = iota
	inTest     = iota
	processing = iota
)

func parseTestOutput(scanner *bufio.Scanner, job string, tests Tests, buildNumber int, result TestToBuildData) {
	buff := &bytes.Buffer{}
	state := scanning
	testName := ""
	build := fmt.Sprintf("%d", buildNumber)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, TestNameSeparator) {
			var ok bool
			testName, ok = tests[strings.Trim(strings.Split(line, TestNameSeparator)[1], " ")]
			if !ok {
				state = scanning
				continue
			}
			state = inTest
			if _, found := result[testName]; found && result[testName].Job != job {
				// If the job is different, we'll skip the test and keep the old result
				state = scanning
			}
		}
		if state == processing {
			// TODO(random-liu): Deprecate the old label when all latest 100 builds have the perftype.PerfResultEnd
			if strings.Contains(line, perftype.PerfResultEnd) || strings.Contains(line, "INFO") || strings.Contains(line, "STEP") || strings.Contains(line, "Failure") ||
				strings.Contains(line, "[AfterEach]") {
				state = inTest
				obj := perftype.PerfData{}
				if err := json.Unmarshal(buff.Bytes(), &obj); err != nil {
					fmt.Fprintf(os.Stderr, "error parsing JSON in build %d: %v %s\n", buildNumber, err, buff.String())
					continue
				}
				// Only support the latest version of performance data.
				// TODO(random-liu): Also show older version data if there is any.
				if _, found := result[testName]; !found {
					result[testName] = BuildData{Job: job, Version: obj.Version, Builds: map[string][]perftype.DataItem{}}
				}
				if result[testName].Version == obj.Version {
					result[testName].Builds[build] = append(result[testName].Builds[build], obj.DataItems...)
				}

				buff.Reset()
			}
		}
		if state == inTest && strings.Contains(line, perftype.PerfResultTag) {
			state = processing
			line = line[strings.Index(line, "{"):]
		}
		if state == processing {
			buff.WriteString(line + " ")
		}
	}
}
