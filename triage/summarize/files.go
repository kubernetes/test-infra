/*
Copyright 2020 The Kubernetes Authors.

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

/*
Contains functions that manage the reading and writing of files related to package summarize.
This includes reading and interpreting JSON files as actionable data, memoizing function
results to JSON, and outputting results once the summarization process is complete.
*/

package summarize

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strconv"
	"strings"
)

// jsonBuild represents a build as reported by the JSON. All values are strings.
// This should not be instantiated directly, but rather via the encoding/json package's
// Unmarshal method. This is an intermediary state for the data until it can be put into
// a build object.
type jsonBuild struct {
	path         string
	started      string
	elapsed      string
	tests_run    string
	tests_failed string
	result       string
	executor     string
	job          string
	number       string
}

// asBuild is a factory function that creates a build object from a jsonBuild object, appropriately
// handling all type conversions.
func (jb *jsonBuild) asBuild() (build, error) {
	// The build object that will be returned, initialized with the values that
	// don't need conversion.
	b := build{
		path:     jb.path,
		result:   jb.result,
		executor: jb.executor,
		job:      jb.job,
	}

	// To avoid assignment issues
	var err error

	// started
	b.started, err = strconv.Atoi(jb.started)
	if err != nil {
		return build{}, fmt.Errorf("Error converting JSON string '%s' to int: %s", jb.started, err)
	}

	// elapsed
	tempElapsed, err := strconv.ParseFloat(jb.elapsed, 32)
	if err != nil {
		return build{}, fmt.Errorf("Error converting JSON string '%s' to float32: %s", jb.elapsed, err)
	}
	b.elapsed = int(tempElapsed)

	// testsRun
	b.testsRun, err = strconv.Atoi(jb.tests_run)
	if err != nil {
		return build{}, fmt.Errorf("Error converting JSON string '%s' to int: %s", jb.tests_run, err)
	}

	// testsFailed
	b.testsFailed, err = strconv.Atoi(jb.tests_failed)
	if err != nil {
		return build{}, fmt.Errorf("Error converting JSON string '%s' to int: %s", jb.tests_failed, err)
	}

	// number
	b.number, err = strconv.Atoi(jb.number)
	if err != nil {
		return build{}, fmt.Errorf("Error converting JSON string '%s' to int: %s", jb.number, err)
	}

	return b, nil
}

// jsonFailure represents a test failure as reported by the JSON. All values are strings.
// This should not be instantiated directly, but rather via the encoding/json package's
// Unmarshal method. This is an intermediary state for the data until it can be put into
// a failure object.
type jsonFailure struct {
	started      string
	build        string
	name         string
	failure_text string
}

// asFailure is a factory function that creates a failure object from the jsonFailure object,
// appropriately handling all type conversions.
func (jf *jsonFailure) asFailure() (failure, error) {
	// The failure object that will be returned, initialized with the values that
	// don't need conversion.
	f := failure{
		build:       jf.build,
		name:        jf.name,
		failureText: jf.failure_text,
	}

	// To avoid assignment issues
	var err error

	// started
	f.started, err = strconv.Atoi(jf.started)
	if err != nil {
		return failure{}, fmt.Errorf("Error converting JSON string '%s' to int: %s", jf.started, err)
	}

	return f, nil
}

// Load builds and failed tests files. Group builds by path, group failed test by test name.
// @file_memoize("loading failed tests", "memo_load_failures.json") TODO
func loadFailures(buildsFilepath string, testsFilepaths []string) (map[string]build, map[string][]failure, error) {
	builds, err := loadBuilds(buildsFilepath)
	if err != nil {
		return nil, nil, fmt.Errorf("Could not retrieve builds: %s", err)
	}

	tests, err := loadTests(testsFilepaths)
	if err != nil {
		return nil, nil, fmt.Errorf("Could not retrieve tests: %s", err)
	}

	return builds, tests, nil
}

// loadBuilds parses a JSON file containing build information and returns a map from build paths
// to build objects.
func loadBuilds(filepath string) (map[string]build, error) {
	// The map
	var builds map[string]build

	// jsonBuilds temporarily stores the builds as they are retrieved from the JSON file
	// until they can be converted to build objects
	var jsonBuilds []jsonBuild

	err := getJSON(filepath, &jsonBuilds)
	if err != nil {
		return nil, fmt.Errorf("Could not get builds JSON: %s", err)
	}

	// Convert the build information to internal build objects and store them in the builds map
	for _, jBuild := range jsonBuilds {
		// Skip builds without a start time or build number
		if jBuild.started == "" || jBuild.number == "" {
			continue
		}

		bld, err := jBuild.asBuild()
		if err != nil {
			return nil, fmt.Errorf("Could not create build object from jsonBuild object: %s", err)
		}

		if strings.Contains(bld.path, "pr-logs") {
			parts := strings.Split(bld.path, "/")
			bld.pr = parts[len(parts)-3]
		}

		builds[bld.path] = bld
	}

	return builds, nil
}

// loadTests parses multiple JSON files containing test information for failed tests. It returns a
// map from test names to build objects.
func loadTests(testsFilepaths []string) (map[string][]failure, error) {
	// The map
	var tests map[string][]failure

	// jsonTests temporarily stores the tests as they are retrieved from the JSON file
	// until they can be converted to build objects
	var jsonFailures []jsonFailure
	for _, filepath := range testsFilepaths {
		err := getJSON(filepath, &jsonFailures)
		if err != nil {
			return nil, fmt.Errorf("Could not get tests JSON: %s", err)
		}

		// Convert the failure information to internal failure objects and store them in the failed_tests map
		for _, jf := range jsonFailures {
			// Check if tests of this type are already in the map
			if _, ok := tests[jf.name]; !ok {
				tests[jf.name] = make([]failure, 0)
			}

			test, err := jf.asFailure()
			if err != nil {
				return nil, fmt.Errorf("Could not create failure object from jsonFailure object: %s", err)
			}

			tests[jf.name] = append(tests[jf.name], test)
		}
	}

	// Sort the failures withing each test by build
	for _, testSlice := range tests {
		sort.Slice(testSlice, func(i, j int) bool { return testSlice[i].build < testSlice[j].build })
	}

	return tests, nil
}

// getJSON opens a JSON file, parses it according to the schema provided by v, and places the results
// into v. Internally, it calls encoding/json's Unmarshal using v as the second argument. Therefore,
// v mut be a non-nil pointer.
func getJSON(filepath string, v interface{}) error {
	// Open the tests file
	fd, err := os.Open(filepath)
	if err != nil {
		return fmt.Errorf("Could not open file '%s': %s", filepath, err)
	}
	defer fd.Close()

	// Read the file
	contents, err := ioutil.ReadAll(fd)
	if err != nil {
		return fmt.Errorf("Could not read file '%s': %s", filepath, err)
	}

	// Decode the JSON into failures
	err = json.Unmarshal(contents, v)
	if err != nil {
		return fmt.Errorf("Could not unmarshal JSON: %s", err)
	}

	return nil
}
