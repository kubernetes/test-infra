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

package main

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/metadata/junit"
	"github.com/sirupsen/logrus"
)

var (
	// reSuiteStart identifies the start of a new TestSuite and captures the package path.
	// Matches lines like "pkg: k8s.io/test-infra/experiment/dummybenchmarks"
	reSuiteStart = regexp.MustCompile(`^pkg:\s+(\S+)\s*$`)
	// reSuiteEnd identifies the end of a TestSuite and captures the overall result, package path, and runtime.
	// Matches lines like:
	// "ok  	k8s.io/test-infra/experiment/dummybenchmarks/subpkg	1.490s"
	// "FAIL	k8s.io/test-infra/experiment/dummybenchmarks	17.829s"
	reSuiteEnd = regexp.MustCompile(`^(ok|FAIL)\s+(\S+)\s+(\S+)\s*$`)
	// reBenchMetrics identifies lines with metrics for successful Benchmarks and captures the name, op count, and metric values.
	// Matches lines like:
	// "Benchmark-4                 	20000000	       77.9 ns/op"
	// "BenchmarkAllocsAndBytes-4   	10000000	       131 ns/op	 152.50 MB/s	     112 B/op	       2 allocs/op"
	reBenchMetrics = regexp.MustCompile(`^(Benchmark\S*)\s+(\d+)\s+([\d\.]+) ns/op(?:\s+([\d\.]+) MB/s)?(?:\s+([\d\.]+) B/op)?(?:\s+([\d\.]+) allocs/op)?\s*$`)
	// reActionLine identifies lines that start with "--- " and denote the start of log output and/or a skipped or failed Benchmark.
	// Matches lines like:
	// "--- BENCH: BenchmarkLog-4"
	// "--- SKIP: BenchmarkSkip"
	// "--- FAIL: BenchmarkFatal"
	reActionLine = regexp.MustCompile(`^--- (BENCH|SKIP|FAIL):\s+(\S+)\s*$`)
)

func truncate(str string, n int) string {
	if len(str) <= n {
		return str
	}
	if n > 3 {
		return str[:n-3] + "..."
	}
	return str[:n]
}

func recordLogText(s *junit.Suite, text string) {
	if len(s.Results) == 0 {
		logrus.Error("Tried to record Benchmark log text before any Benchmarks were found for the package!")
		return
	}
	result := &s.Results[len(s.Results)-1]
	// TODO: make text truncation configurable.
	text = truncate(text, 1000)

	switch {
	case result.Failure != nil:
		*result.Failure = text
		// Also add failure text to "categorized_fail" property for TestGrid.
		result.SetProperty("categorized_fail", text)

	case result.Skipped != nil:
		*result.Skipped = text

	default:
		result.Output = &text
	}
}

// applyPropertiesFromMatch sets the properties and test duration for Result based on a Benchmark metric line match (reBenchMetrics).
func applyPropertiesFromMatch(result *junit.Result, match []string) error {
	opCount, err := strconv.ParseFloat(match[2], 64)
	if err != nil {
		return fmt.Errorf("error parsing opcount %q: %v", match[2], err)
	}
	opDuration, err := strconv.ParseFloat(match[3], 64)
	if err != nil {
		return fmt.Errorf("error parsing ns/op %q: %v", match[3], err)
	}
	result.Time = opCount * opDuration / 1000000000 // convert from ns to s.
	result.SetProperty("op count", match[2])
	result.SetProperty("avg op duration (ns/op)", match[3])
	if len(match[4]) > 0 {
		result.SetProperty("MB/s", match[4])
	}
	if len(match[5]) > 0 {
		result.SetProperty("alloced B/op", match[5])
	}
	if len(match[6]) > 0 {
		result.SetProperty("allocs/op", match[6])
	}

	return nil
}

func parse(raw []byte) (*junit.Suites, error) {
	lines := strings.Split(string(raw), "\n")

	var suites junit.Suites
	var suite junit.Suite
	var logText string
	for _, line := range lines {
		// First handle multi-line log text aggregation
		if strings.HasPrefix(line, "    ") {
			logText += strings.TrimPrefix(line, "    ") + "\n"
			continue
		} else if len(logText) > 0 {
			recordLogText(&suite, logText)
			logText = ""
		}

		switch {
		case reSuiteStart.MatchString(line):
			match := reSuiteStart.FindStringSubmatch(line)
			suite = junit.Suite{
				Name: match[1],
			}

		case reSuiteEnd.MatchString(line):
			match := reSuiteEnd.FindStringSubmatch(line)
			if match[2] != suite.Name {
				// "Mismatched package summary for match[2] with suite.Name testsuites.
				// This is normal in scenarios where the line matching pkg <packagename>
				// is missing (running as a bazel test).
				suite.Name = match[2]
			}
			duration, err := time.ParseDuration(match[3])
			if err != nil {
				return nil, fmt.Errorf("failed to parse package test time %q: %v", match[3], err)
			}
			suite.Time = duration.Seconds()
			suites.Suites = append(suites.Suites, suite)
			suite = junit.Suite{}

		case reBenchMetrics.MatchString(line):
			match := reBenchMetrics.FindStringSubmatch(line)
			result := junit.Result{
				ClassName: path.Base(suite.Name),
				Name:      match[1],
			}
			if err := applyPropertiesFromMatch(&result, match); err != nil {
				return nil, fmt.Errorf("error parsing benchmark metric values: %v", err)
			}
			suite.Results = append(suite.Results, result)
			suite.Tests += 1

		case reActionLine.MatchString(line):
			match := reActionLine.FindStringSubmatch(line)
			emptyText := "" // Will be replaced with real text once output is read.
			if match[1] == "SKIP" {
				suite.Results = append(suite.Results, junit.Result{
					ClassName: path.Base(suite.Name),
					Name:      match[2],
					Skipped:   &emptyText,
				})
			} else if match[1] == "FAIL" {
				suite.Results = append(suite.Results, junit.Result{
					ClassName: path.Base(suite.Name),
					Name:      match[2],
					Failure:   &emptyText,
				})
				suite.Failures += 1
				suite.Tests += 1
			}
		}
	}
	return &suites, nil
}
