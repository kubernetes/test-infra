/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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

package src

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"

	"k8s.io/kubernetes/test/e2e"

	"github.com/golang/glog"
)

const (
	defaultState     = iota
	readingLogs      = iota
	readingResources = iota
	readingMetrics   = iota
)

// ProcessSingleTest processes single Jenkins output file, reading and parsing JSON
// summaries embedded in it.
func ProcessSingleTest(scanner *bufio.Scanner, buildNumber int) (map[string]*e2e.LogsSizeDataSummary, map[string]*e2e.ResourceUsageSummary, map[string]*e2e.MetricsForE2E) {
	buff := &bytes.Buffer{}
	logSummary := make(map[string]*e2e.LogsSizeDataSummary)
	resourceSummary := make(map[string]*e2e.ResourceUsageSummary)
	metricsSummary := make(map[string]*e2e.MetricsForE2E)
	state := defaultState
	oldTestSeparator := "[It] [Performance] "
	testSeparator := "[It] [Feature:Performance] "
	testName := ""
	for scanner.Scan() {
		line := scanner.Text()
		if state == defaultState {
			separator := ""
			if strings.Contains(line, testSeparator) {
				separator = testSeparator
			} else if strings.Contains(line, oldTestSeparator) {
				separator = oldTestSeparator
			}
			if separator != "" {
				testName = strings.Trim(strings.Split(line, separator)[1], " ")
				buff.Reset()
			}
		}
		if strings.Contains(line, "Finished") {
			if state == readingLogs {
				logSummary[testName] = &e2e.LogsSizeDataSummary{}
				state = defaultState
				if err := json.Unmarshal(buff.Bytes(), logSummary[testName]); err != nil {
					glog.V(0).Infof("error parsing LogsSizeDataSummary JSON in build %d: %v %s\n", buildNumber, err, buff.String())
					continue
				}
			}
			if state == readingResources {
				resourceSummary[testName] = &e2e.ResourceUsageSummary{}
				state = defaultState
				if err := json.Unmarshal(buff.Bytes(), resourceSummary[testName]); err != nil {
					glog.V(0).Infof("error parsing ResourceUsageSummary JSON in build %d: %v %s\n", buildNumber, err, buff.String())
					continue
				}
			}
			if state == readingMetrics {
				metricsSummary[testName] = &e2e.MetricsForE2E{}
				state = defaultState
				if err := json.Unmarshal(buff.Bytes(), metricsSummary[testName]); err != nil {
					glog.V(0).Infof("error parsing MetricsForE2E JSON in build %d: %v %s\n", buildNumber, err, buff.String())
					continue
				}
			}
			buff.Reset()
		}
		if state != defaultState {
			buff.WriteString(line + " ")
		}
		if strings.Contains(line, "LogsSizeDataSummary JSON") {
			state = readingLogs
		}
		if strings.Contains(line, "ResourceUsageSummary JSON") {
			state = readingResources
		}
		if strings.Contains(line, "MetricsForE2E JSON") {
			state = readingMetrics
		}
	}
	return logSummary, resourceSummary, metricsSummary
}
