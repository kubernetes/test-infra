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
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/testgrid/metadata/junit"
)

func createTempFile() (string, error) {
	outFile, err := ioutil.TempFile("", "dummybenchmarks")
	if err != nil {
		return "", fmt.Errorf("create error: %v", err)
	}
	if err := outFile.Close(); err != nil {
		return "", fmt.Errorf("close error: %v", err)
	}
	return outFile.Name(), nil
}

var goBinaryPath = flag.String("go", "go", "The location of the go binary. This flag is primarily intended for use with bazel.")

func TestDummybenchmarksIntegration(t *testing.T) {
	// Set HOME and GOROOT envvars for bazel if needed.
	if os.Getenv("HOME") == "" {
		wd, _ := os.Getwd() // Just use `/home` if we can't determine working dir.
		os.Setenv("HOME", wd+"/home")
	}
	if os.Getenv("GOROOT") == "" {
		goRoot, _ := filepath.Abs("../../external/go_sdk")
		os.Setenv("GOROOT", goRoot)
		t.Logf("Setting GOROOT to %q.\n", goRoot)
	}

	outFile, err := createTempFile()
	if err != nil {
		t.Fatalf("Error creating output file: %v.", err)
	}
	defer os.Remove(outFile)
	logFile, err := createTempFile()
	if err != nil {
		t.Fatalf("Error creating log file: %v.", err)
	}
	defer os.Remove(logFile)
	t.Logf("Logging benchmark output to %q.", logFile)
	opts := &options{
		outputFile:   outFile,
		logFile:      logFile,
		goBinaryPath: *goBinaryPath,
		passOnError:  true,
		// Omit benchmarks that aren't in the core group to keep test time reasonable.
		benchRegexp: "Core",
	}

	t.Logf("Starting benchmarkjunit outputting to %q...", opts.outputFile)
	run(opts, []string{"../../experiment/dummybenchmarks/..."})
	t.Log("Finished running benchmarkjunit. Validating JUnit XML...")

	// Print log file contents.
	rawLog, err := ioutil.ReadFile(opts.logFile)
	if err != nil {
		t.Fatalf("Error reading log file: %v.", err)
	}
	t.Logf("Log file output:\n%s\n\n", string(rawLog))

	// Read and parse JUnit output.
	raw, err := ioutil.ReadFile(opts.outputFile)
	if err != nil {
		t.Fatalf("Error reading output file: %v.", err)
	}
	suites, err := junit.Parse(raw)
	if err != nil {
		t.Fatalf("Error parsing JUnit XML testsuites: %v.", err)
	}

	if len(suites.Suites) != 2 {
		t.Fatalf("Expected 2 testsuites, but found %d.", len(suites.Suites))
	}
	// Validate the main 'dummybenchmarks' suite
	s := suites.Suites[0]
	if base := path.Base(s.Name); base != "dummybenchmarks" {
		t.Errorf("Expected testsuites[0] to have basename \"dummybenchmarks\", but got %q.", base)
	}
	expectedBenchmarks := []string{
		"BenchmarkCoreSimple",
		"BenchmarkCoreAllocsAndBytes",
		"BenchmarkCoreParallel",
		"BenchmarkCoreLog",
		"BenchmarkCoreSkip",
		"BenchmarkCoreSkipNow",
		"BenchmarkCoreError",
		"BenchmarkCoreFatal",
		"BenchmarkCoreFailNow",
		"BenchmarkCoreNestedShallow/simple",
		"BenchmarkCoreNestedShallow/parallel",
	}
	var allocsAndBytes junit.Result
	var foundBenchmarks []string
	for _, result := range s.Results {
		// Remove the trailing "-\d+"
		name := strings.Split(result.Name, "-")[0]
		foundBenchmarks = append(foundBenchmarks, name)

		if name == "BenchmarkCoreAllocsAndBytes" {
			allocsAndBytes = result
		}
	}
	sort.Strings(expectedBenchmarks)
	sort.Strings(foundBenchmarks)
	if !reflect.DeepEqual(expectedBenchmarks, foundBenchmarks) {
		t.Errorf("Expected benchmarks %q, but got %q.", expectedBenchmarks, foundBenchmarks)
	}
	// Check that all properties exist on the AllocsAndBytes benchmark and that
	// all parse to float64. (This is the only benchmark that has all properties.)
	foundProps := make(map[string]string)
	for _, prop := range allocsAndBytes.Properties.PropertyList {
		if val, ok := foundProps[prop.Name]; ok {
			t.Errorf("BenchmarkCoreAllocsAndBytes has duplicated property %q. Values: %q, %q.", prop.Name, val, prop.Value)
		}
		foundProps[prop.Name] = prop.Value
	}
	expectedProps := []string{
		"op count",
		"avg op duration (ns/op)",
		"MB/s",
		"alloced B/op",
		"allocs/op",
	}
	for _, expectedProp := range expectedProps {
		value, ok := foundProps[expectedProp]
		if !ok {
			t.Errorf("BenchmarkCoreAllocsAndBytes is missing property %q.", expectedProp)
			continue
		}
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			t.Errorf("Failed to parse the %q=%q property of BenchmarkCoreAllocsAndBytes: %v.", expectedProp, value, err)
		}
	}

	// Validate the 'subpkg' suite.
	s = suites.Suites[1]
	if base := path.Base(s.Name); base != "subpkg" {
		t.Errorf("Expected testsuites[1] to have basename \"subpkg\", but got %q.", base)
	}
	if len(s.Results) != 1 || !strings.HasPrefix(s.Results[0].Name, "BenchmarkCoreSubPkg") {
		t.Errorf("Expected one \"BenchmarkCoreSubPkg\" result, but found %+v.", s.Results)
	}
}
