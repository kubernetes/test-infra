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
	"io"
	"io/ioutil"
	"os"
	"sort"
	"strconv"
	"strings"

	"k8s.io/klog/v2"
)

// loadFailures loads a builds file and one or more test failure files. It maps build paths to builds
// and groups test failures by test name. memoize determines if memoized results should attempt to be
// retrieved, and if new results should be memoized to JSON.
func loadFailures(buildsFilepath string, testsFilepaths []string, memoize bool) (map[string]build, map[string][]failure, error) {
	builds, err := loadBuilds(buildsFilepath, memoize)
	if err != nil {
		return nil, nil, fmt.Errorf("Could not retrieve builds: %s", err)
	}

	tests, err := loadTests(testsFilepaths, memoize)
	if err != nil {
		return nil, nil, fmt.Errorf("Could not retrieve tests: %s", err)
	}

	return builds, tests, nil
}

// loadPrevious loads a previous output and returns the 'clustered' field.
func loadPrevious(filepath string) ([]jsonCluster, error) {
	var previous jsonOutput

	err := getJSON(filepath, &previous)
	if err != nil {
		return nil, fmt.Errorf("Could not get previous results JSON: %s", err)
	}

	return previous.Clustered, nil
}

// loadOwners loads an owners JSON file and returns it.
func loadOwners(filepath string) (map[string][]string, error) {
	var owners map[string][]string

	err := getJSON(filepath, &owners)
	if err != nil {
		return nil, fmt.Errorf("Could not get owners JSON: %s", err)
	}

	return owners, nil
}

// writeResults outputs the results of clustering to a file.
func writeResults(filepath string, data jsonOutput) error {
	err := writeJSON(filepath, data)
	if err != nil {
		return fmt.Errorf("Could not write results to disk: %s", err)
	}
	return nil
}

type renderedSliceOutput struct {
	Clustered []jsonCluster `json:"clustered"`
	Builds    columns       `json:"builds"`
}

// writeRenderedSlice outputs the results of a call to renderSlice() to a file.
func writeRenderedSlice(filepath string, clustered []jsonCluster, builds columns) error {
	output := renderedSliceOutput{
		clustered,
		builds,
	}

	err := writeJSON(filepath, output)
	if err != nil {
		return fmt.Errorf("Could not write subset to disk: %s", err)
	}
	return nil
}

/*
getMemoizedResults attempts to retrieve memoized function results from the given filepath. If it
succeeds, it places the results into v and returns true. Otherwise, it returns false. Internally,
it calls encoding/json's Unmarshal using v as the second argument. Therefore, v mut be a non-nil
pointer.

message is a message that gets printed on success, appended to "Done (cached) ". If it is the empty
string, no message is printed.
*/
func getMemoizedResults(filepath string, message string, v interface{}) (ok bool) {
	err := getJSON(filepath, v)
	if err == nil {
		if message != "" {
			klog.V(2).Infof("Retrieved memoized results from %#v, done (cached) %s", filepath, message)
		}
		return true
	}
	return false
}

/*
memoizeResults saves the results stored in v to a JSON file. v should be a value, not a pointer. It
prints a warning if the results could not be memoized.

message is a message that gets printed on success, appended to "Done ". If it is the empty
string, no message is printed.
*/
func memoizeResults(filepath, message string, v interface{}) {
	klog.V(2).Infof("Memoizing results to %s...", filepath)

	err := writeJSON(filepath, v)
	if err != nil {
		klog.Warningf("Could not memoize results to '%s': %s", filepath, err)
		return
	}

	if message != "" {
		klog.V(2).Infof("Successfully memoized, done " + message)
	}
}

// jsonBuild represents a build as reported by the JSON. All values are strings.
// This should not be instantiated directly, but rather via the encoding/json package's
// Unmarshal method. This is an intermediary state for the data until it can be put into
// a build object.
type jsonBuild struct {
	Path        string `json:"path"`
	Started     string `json:"started"`
	Elapsed     string `json:"elapsed"`
	TestsRun    string `json:"tests_run"`
	TestsFailed string `json:"tests_failed"`
	Result      string `json:"result"`
	Executor    string `json:"executor"`
	Job         string `json:"job"`
	Number      string `json:"number"`
	PR          string `json:"pr"`
	Key         string `json:"key"` // Often nonexistent
}

// asBuild is a factory function that creates a build object from a jsonBuild object, appropriately
// handling all type conversions.
func (jb *jsonBuild) asBuild() (build, error) {
	// The build object that will be returned, initialized with the values that
	// don't need conversion.
	b := build{
		Path:     jb.Path,
		Result:   jb.Result,
		Executor: jb.Executor,
		Job:      jb.Job,
		PR:       jb.PR,
		Key:      jb.Key,
	}

	// To avoid assignment issues
	var err error

	// started
	if jb.Started != "" {
		b.Started, err = strconv.Atoi(jb.Started)
		if err != nil {
			return build{}, fmt.Errorf("Error converting JSON string '%s' to int for build field 'started': %s", jb.Started, err)
		}
	}

	// elapsed
	if jb.Elapsed != "" {
		tempElapsed, err := strconv.ParseFloat(jb.Elapsed, 32)
		if err != nil {
			return build{}, fmt.Errorf("Error converting JSON string '%s' to float32 for build field 'elapsed': %s", jb.Elapsed, err)
		}
		b.Elapsed = int(tempElapsed)
	}

	// testsRun
	if jb.TestsRun != "" {
		b.TestsRun, err = strconv.Atoi(jb.TestsRun)
		if err != nil {
			return build{}, fmt.Errorf("Error converting JSON string '%s' to int for build field 'testsRun': %s", jb.TestsRun, err)
		}
	}

	// testsFailed
	if jb.TestsFailed != "" {
		b.TestsFailed, err = strconv.Atoi(jb.TestsFailed)
		if err != nil {
			return build{}, fmt.Errorf("Error converting JSON string '%s' to int for build field 'testsFailed': %s", jb.TestsFailed, err)
		}
	}

	// number
	if jb.Number != "" {
		b.Number, err = strconv.Atoi(jb.Number)
		if err != nil {
			return build{}, fmt.Errorf("Error converting JSON string '%s' to int for build field 'number': %s", jb.Number, err)
		}
	}

	return b, nil
}

// loadBuilds parses a JSON file containing build information and returns a map from build paths
// to build objects. memoize determines if memoized results should attempt to be retrieved, and if
// new results should be memoized to JSON.
func loadBuilds(filepath string, memoize bool) (map[string]build, error) {
	const memoPath string = "memo_load_builds.json"
	const memoMessage string = "loading builds"

	builds := make(map[string]build)

	// Try to retrieve memoized results first to avoid another computation
	if memoize && getMemoizedResults(memoPath, memoMessage, &builds) {
		return builds, nil
	}

	// jsonBuilds temporarily stores the builds as they are retrieved from the JSON file
	// until they can be converted to build objects
	jsonBuilds := make([]jsonBuild, 0)

	err := getJSON(filepath, &jsonBuilds)
	if err != nil {
		return nil, fmt.Errorf("Could not get builds JSON: %s", err)
	}

	// Convert the build information to internal build objects and store them in the builds map
	for _, jBuild := range jsonBuilds {
		// Skip builds without a start time or build number
		if jBuild.Started == "" || jBuild.Number == "" {
			continue
		}

		bld, err := jBuild.asBuild()
		if err != nil {
			return nil, fmt.Errorf("Could not create build object from jsonBuild object: %s", err)
		}

		if strings.Contains(bld.Path, "pr-logs") {
			parts := strings.Split(bld.Path, "/")
			bld.PR = parts[len(parts)-3]
		}

		builds[bld.Path] = bld
	}

	if memoize {
		memoizeResults(memoPath, memoMessage, builds)
	}

	return builds, nil
}

// jsonFailure represents a test failure as reported by the JSON. All values are strings.
// This should not be instantiated directly, but rather via the encoding/json package's
// Unmarshal method. This is an intermediary state for the data until it can be put into
// a failure object.
type jsonFailure struct {
	Started     string `json:"started"`
	Build       string `json:"build"`
	Name        string `json:"name"`
	FailureText string `json:"failure_text"`
}

// asFailure is a factory function that creates a failure object from the jsonFailure object,
// appropriately handling all type conversions.
func (jf *jsonFailure) asFailure() (failure, error) {
	// The failure object that will be returned, initialized with the values that
	// don't need conversion.
	f := failure{
		Build:       jf.Build,
		Name:        jf.Name,
		FailureText: jf.FailureText,
	}

	// To avoid assignment issues
	var err error

	// started
	if jf.Started != "" {
		f.Started, err = strconv.Atoi(jf.Started)
		if err != nil {
			return failure{}, fmt.Errorf("Error converting JSON string '%s' to int for failure field 'started': %s", jf.Started, err)
		}
	}

	return f, nil
}

// loadTests parses multiple JSON files containing test information for failed tests. It returns a
// map from test names to failure objects. memoize determines if memoized results should attempt to
// be retrieved, and if new results should be memoized to JSON.
func loadTests(testsFilepaths []string, memoize bool) (map[string][]failure, error) {
	const memoPath string = "memo_load_tests.json"
	const memoMessage string = "loading tests"

	tests := make(map[string][]failure)

	// Try to retrieve memoized results first to avoid another computation
	if memoize && getMemoizedResults(memoPath, memoMessage, &tests) {
		return tests, nil
	}

	// jsonTests temporarily stores the tests as they are retrieved from the JSON file
	// until they can be converted to failure objects
	jsonFailures := make([]jsonFailure, 0)
	for _, filepath := range testsFilepaths {
		file, err := os.Open(filepath)
		if err != nil {
			return nil, fmt.Errorf("Could not open tests file '%s': %s", filepath, err)
		}
		defer file.Close()

		// Read each line in the file as its own JSON object
		decoder := json.NewDecoder(file)
		for {
			// TODO: this probably gives fairly useless errors
			var jf jsonFailure
			if err := decoder.Decode(&jf); err == io.EOF {
				break // reached EOF
			} else if err != nil {
				return nil, err
			}
			jsonFailures = append(jsonFailures, jf)
		}

		// Convert the failure information to internal failure objects and store them in tests
		for _, jf := range jsonFailures {
			// Check if tests of this type are already in the map
			if _, ok := tests[jf.Name]; !ok {
				tests[jf.Name] = make([]failure, 0)
			}

			test, err := jf.asFailure()
			if err != nil {
				return nil, fmt.Errorf("Could not create failure object from jsonFailure object: %s", err)
			}

			tests[jf.Name] = append(tests[jf.Name], test)
		}
	}

	// Sort the failures within each test by build
	for _, testSlice := range tests {
		sort.Slice(testSlice, func(i, j int) bool { return testSlice[i].Build < testSlice[j].Build })
	}

	if memoize {
		memoizeResults(memoPath, memoMessage, tests)
	}

	return tests, nil
}

// getJSON opens a JSON file, parses it according to the schema provided by v, and places the results
// into v. Internally, it calls encoding/json's Unmarshal using v as the second argument. Therefore,
// v mut be a non-nil pointer.
func getJSON(filepath string, v interface{}) error {
	contents, err := ioutil.ReadFile(filepath)
	if err != nil {
		return fmt.Errorf("Could not open file '%s': %s", filepath, err)
	}

	// Decode the JSON into the provided interface
	err = json.Unmarshal(contents, v)
	if err != nil {
		return fmt.Errorf("Could not unmarshal JSON: %s\nTried to unmarshal the following: %#v", err, v)
	}

	return nil
}

// writeJSON generates JSON according to v and writes the results to filepath.
func writeJSON(filepath string, v interface{}) error {
	output, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("Could not encode JSON: %s\nTried to encode the following: %#v", err, v)
	}

	err = ioutil.WriteFile(filepath, output, 0644)
	if err != nil {
		return fmt.Errorf("Could not write JSON to file: %s", err)
	}

	return nil
}
