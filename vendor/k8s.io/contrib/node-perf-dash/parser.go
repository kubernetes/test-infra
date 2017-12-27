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
	"log"
	"os"
	"regexp"
	"strings"
)

// State machine of the parser.
const (
	scanning   = iota
	inTest     = iota
	processing = iota
)

var (
	// Regex to extract perf data log. It is used to get the test end time (time of the first perf data log).
	// The end time is used to find events for each test from kubelet.log.
	// TODO(coufon): to be more reliable, explicitly log test starting and ending time in a fixed format in node e2e test.
	regexResult = regexp.MustCompile(`([A-Z][a-z]*\s{1,2}\d{1,2} \d{2}:\d{2}:\d{2}.\d{3}): INFO: .*`)

	// buildFIFOs stores a FIFO for each test/machine. A FIFO stores all build names. It is used to find and discard the oldest build.
	buildFIFOs = map[string][]string{}

	// formattedNodeNames stores formatted node names, looked up by host name (the machine which runs the test).
	formattedNodeNames = map[string]string{}
)

// parseTestOutput extract perf/time series data from test result file.
func parseTestOutput(scanner *bufio.Scanner, job string, buildNumber int, result TestToBuildData, testTime TestTime) {
	buff := &bytes.Buffer{}
	state := scanning
	TestDetail := ""
	endTime := ""
	build := fmt.Sprintf("%d", buildNumber)
	isTimeSeries := false

	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, TestNameSeparator) && strings.Contains(line, BenchmarkSeparator) {
			TestDetail = line
			state = inTest
		}
		if state == processing {
			if strings.Contains(line, perfResultEnd) || strings.Contains(line, timeSeriesEnd) ||
				strings.Contains(line, "INFO") || strings.Contains(line, "STEP") ||
				strings.Contains(line, "Failure") || strings.Contains(line, "[AfterEach]") {
				state = inTest

				// Parse JSON to perf/time series data item
				obj := TestData{}
				if err := json.Unmarshal(buff.Bytes(), &obj); err != nil {
					fmt.Fprintf(os.Stderr, "Error: parsing JSON in build %d: %v %s\n",
						buildNumber, err, buff.String())
					continue
				}

				// testName is the short name of Ginkgo tests.
				// node is the name of host machine. It has the format "machineType-image-uuid" (for benchmark tests) or "prefix-uuid-image" (for normal node e2e tests).
				// nodeName is the formatted name of node in the format "image_machineCapacity". Currently image and machineCapacity are labels with test data.
				// If the labels are not found, image and machine info are extracted from node name "machineType-image-uuid" (to be deprecated).
				testName, node, nodeName := obj.Labels["test"], obj.Labels["node"], formatNodeName(obj.Labels, job)
				testInfoMap.Info[testName] = TestDetail

				if endTime == "" {
					log.Fatal("Error: test end time not parsed")
				}

				// Record the test name, node and its end time.
				testTime.Add(testName, node, endTime)
				// Clear endtime.
				endTime = ""

				if _, found := result[testName]; !found {
					result[testName] = &DataPerTest{
						Job:     job,
						Version: obj.Version,
						Data:    map[string]DataPerNode{},
					}
				}
				if _, found := result[testName].Data[nodeName]; !found {
					result[testName].Data[nodeName] = DataPerNode{}
				}
				if _, found := result[testName].Data[nodeName][build]; !found {
					// Find a new build.
					result[testName].Data[nodeName][build] = &DataPerBuild{}

					key := testName + "/" + nodeName
					// Update build FIFO.
					buildFIFOs[key] = append(buildFIFOs[key], build)
					// Remove stale builds.
					if len(buildFIFOs[key]) > *builds {
						delete(result[testName].Data[nodeName], buildFIFOs[key][0])
						buildFIFOs[key] = buildFIFOs[key][1:]
					}
				}

				// Append new data item.
				if result[testName].Version == obj.Version {
					if isTimeSeries {
						(result[testName].Data[nodeName][build]).AppendSeriesData(obj)
						isTimeSeries = false
					} else {
						(result[testName].Data[nodeName][build]).AppendPerfData(obj)
					}
				}

				buff.Reset()
			}
		}
		if state == inTest && (strings.Contains(line, perfResultTag) || strings.Contains(line, timeSeriesTag)) {
			if strings.Contains(line, timeSeriesTag) {
				isTimeSeries = true
			}
			state = processing

			// Parse test end time
			matchResult := regexResult.FindSubmatch([]byte(line))
			if matchResult != nil {
				endTime = string(matchResult[1])
			} else {
				log.Fatalf("Error: can not parse test end time:\n%s\n", line)
			}

			// TODO(coufon): it requires the result tag must be on the same line with the starting of JSON data ('{'),
			// it should be more flexible.
			line = line[strings.Index(line, "{"):]
		}
		if state == processing {
			buff.WriteString(line + " ")
		}
	}
}

// parseTracingData extracts and converts tracing data into time series data.
func parseTracingData(scanner *bufio.Scanner, job string, buildNumber int, result TestToBuildData) {
	buff := &bytes.Buffer{}
	state := scanning
	build := fmt.Sprintf("%d", buildNumber)

	for scanner.Scan() {
		line := scanner.Text()
		if state == processing {
			if strings.Contains(line, timeSeriesEnd) {
				state = scanning

				obj := TestData{}
				if err := json.Unmarshal(buff.Bytes(), &obj); err != nil {
					fmt.Fprintf(os.Stderr, "error parsing JSON in build %d: %v\n%s\n", buildNumber, err, buff.String())
					continue
				}

				testName, nodeName := obj.Labels["test"], formatNodeName(obj.Labels, job)

				if _, found := result[testName]; !found {
					fmt.Fprintf(os.Stderr, "Error: tracing data have no test result: %s\n", testName)
					continue
				}
				if _, found := result[testName].Data[nodeName]; !found {
					fmt.Fprintf(os.Stderr, "Error: tracing data have no test result: %s\n", nodeName)
					continue
				}
				if _, found := result[testName].Data[nodeName][build]; !found {
					fmt.Fprintf(os.Stderr, "Error: tracing data have not test result: %s\n", build)
					continue
				}

				if result[testName].Version == obj.Version {
					(result[testName].Data[nodeName][build]).AppendSeriesData(obj)
				}

				buff.Reset()
			}
		}
		if strings.Contains(line, timeSeriesTag) {
			state = processing
			line = line[strings.Index(line, "{"):]
		}
		if state == processing {
			buff.WriteString(line + " ")
		}
	}
}

// formatNodeName gets fromatted node name (image_machineCapacity) from labels of test data.
func formatNodeName(labels map[string]string, job string) string {
	// Get the host name of the test node.
	node := labels["node"]
	// Check if we already have the formatted name.
	if formatted, ok := formattedNodeNames[node]; ok {
		return formatted
	}

	// The labels contains image and machine capacity info.
	image, okImage := labels["image"]
	machine, okMachine := labels["machine"]

	if okImage && okMachine {
		str := image + "_" + machine
		formattedNodeNames[node] = str
		return str
	}

	// Can not find image/machine in the labels. Extract machine/image info
	// from host name "machine-image-uuid" (to be deprecated)
	parts := strings.Split(node, "-")
	lastPart := len(parts) - 1

	machine = parts[0] + "-" + parts[1] + "-" + parts[2]

	// GCI image name (gci-test-00-0000-0-0) is changed across build, drop the
	// suffix for daily build (000-0-0) and keep milestone (test-gci-00)
	// TODO(coufon): we should change test framework to use a consistent name.
	if job == "continuous-node-e2e-docker-benchmark" && parts[3] == "gci" {
		lastPart -= 3
	}

	result := ""
	for _, part := range parts[3:lastPart] {
		result += part + "-"
	}

	image = result[:len(result)-1]
	str := image + "_" + machine
	formattedNodeNames[node] = str
	return str
}
