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

package summarize

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
)

// smear takes a slice of map deltas and returns a slice of maps.
func smear(deltas []map[string]string) []map[string]string {
	cur := make(map[string]string)
	out := make([]map[string]string, 0, len(deltas))

	for _, delta := range deltas {
		for key, val := range delta {
			// Create a key-value mapping, or replace the value with the new one if it exists
			cur[key] = val
		}

		// Copy cur to avoid messing with the original map
		curCopy := make(map[string]string, len(cur))
		for key, val := range cur {
			curCopy[key] = val
		}

		out = append(out, curCopy)
	}

	return out
}

// failOnDifferentLengths fails the provided test if wantedLen != gotLen. Returns true if the test
// failed, false otherwise.
func failOnDifferentLengths(t *testing.T, wantedLen int, gotLen int) bool {
	if wantedLen != gotLen {
		t.Errorf("Wanted result and actual result have different lengths (%d vs. %d)", wantedLen, gotLen)
		return true
	}
	return false
}

// failOnMismatchedSlices fails the provided test if the provided slices have different lengths or
// if their elements do not match (in order). It returns true if it failed and false otherwise.
//
// The slices must both be of the same type, and must be string slices or int slices. The function
// panics if either of these is not true.
func failOnMismatchedSlices(t *testing.T, want interface{}, got interface{}) bool {
	switch want := want.(type) {
	case []string:
		switch got := got.(type) {
		case []string:
			if failOnDifferentLengths(t, len(want), len(got)) {
				return true
			}
			for i := range want {
				if want[i] != got[i] {
					t.Errorf("Wanted value and actual value did not match.\nWanted: %#v\nActual: %#v", want, got)
					return true
				}
			}
		default:
			t.Logf("Type of want does not equal type of got")
			t.FailNow()
		}
	case []int:
		switch got := got.(type) {
		case []int:
			if failOnDifferentLengths(t, len(want), len(got)) {
				return true
			}
			for i := range want {
				if want[i] != got[i] {
					t.Errorf("Wanted value and actual value did not match.\nWanted: %#v\nActual: %#v", want, got)
					return true
				}
			}
		default:
			t.Logf("Type of want does not equal type of got")
			t.FailNow()
		}
	default:
		t.Logf("want and got must be of type []string or []int")
		t.FailNow()
	}

	return false
}

// failOnMismatchedTestSlices fails the provided test t if the provided test slices have different
// lengths or if their elements do not match (in order). It creates a series of subtests for teach
// pair of tests in the slices.
func failOnMismatchedTestSlices(t *testing.T, want []test, got []test) {
	if failOnDifferentLengths(t, len(want), len(want)) {
		return
	}
	for j := range want {
		wantTest := want[j]
		gotTest := got[j]

		t.Run(fmt.Sprintf("tests[%d]", j), func(t *testing.T) {
			if wantTest.Name != gotTest.Name {
				t.Errorf("name = %s, wanted %s", gotTest.Name, wantTest.Name)
				return
			}
			if failOnDifferentLengths(t, len(wantTest.Jobs), len(gotTest.Jobs)) {
				return
			}
			for k := range wantTest.Jobs {
				wantJobs := wantTest.Jobs[k]
				gotJobs := gotTest.Jobs[k]
				if wantJobs.Name != gotJobs.Name {
					t.Errorf("name = %s, wanted %s", gotJobs.Name, wantJobs.Name)
					return
				}
				failOnMismatchedSlices(t, wantJobs.BuildNumbers, gotJobs.BuildNumbers)
			}
		})
	}
}

func TestSummarize(t *testing.T) {
	// Setup
	tmpdir, err := ioutil.TempDir("", "summarize_test_*")
	if err != nil {
		t.Errorf("Could not create temporary directory: %s", err)
		return
	}
	defer os.RemoveAll(tmpdir)

	// Save the old working directory
	olddir, err := os.Getwd()
	if err != nil {
		t.Errorf("Could not get the application working directory: %s", err)
		return
	}
	err = os.Chdir(tmpdir) // Set the working directory to the temp directory
	if err != nil {
		t.Errorf("Could not set the application working directory to the temp directory: %s", err)
		return
	}
	defer func() {
		err = os.Chdir(olddir) // Set the working directory back to the normal directory
		if err != nil {
			t.Errorf("Could not set the application working directory back to the normal directory: %s", err)
			return
		}
	}()

	// Create some test input files

	// builds
	buildsPath := "builds.json"
	builds := smear([]map[string]string{
		{"started": "1234", "number": "1", "tests_failed": "1", "tests_run": "2", "elapsed": "4",
			"path": "gs://logs/some-job/1", "job": "some-job", "result": "SUCCESS"},
		{"number": "2", "path": "gs://logs/some-job/2"},
		{"number": "3", "path": "gs://logs/some-job/3"},
		{"number": "4", "path": "gs://logs/some-job/4"},
		{"number": "5", "path": "gs://logs/other-job/5", "job": "other-job", "elapsed": "8"},
		{"number": "7", "path": "gs://logs/other-job/7", "result": "FAILURE"},
	})
	err = writeJSON(buildsPath, builds)
	if err != nil {
		t.Errorf("Could not write builds.json: %s", err)
		return
	}

	// tests
	testsPath := "tests.json"
	testsTemp := smear([]map[string]string{
		{"name": "example test", "build": "gs://logs/some-job/1",
			"failure_text": "some awful stack trace exit 1"},
		{"build": "gs://logs/some-job/2"},
		{"build": "gs://logs/some-job/3"},
		{"build": "gs://logs/some-job/4"},
		{"name": "another test", "failure_text": "some other error message"},
		{"name": "unrelated test", "build": "gs://logs/other-job/5"},
		{}, // Intentional dupe
		{"build": "gs://logs/other-job/7"},
	})
	tests := make([]byte, 0)
	for _, obj := range testsTemp {
		result, err := json.Marshal(obj)
		if err != nil {
			t.Errorf("Could not encode JSON.\nError: %s\nObject: %#v", err, obj)
			return
		}

		tests = append(tests, result...)
		tests = append(tests, []byte("\n")...)
	}
	err = ioutil.WriteFile(testsPath, tests, 0644)
	if err != nil {
		t.Errorf("Could not write JSON to file: %s", err)
		return
	}

	// owners
	ownersPath := "owners.json"
	err = writeJSON(ownersPath, map[string][]string{"node": {"example"}})
	if err != nil {
		t.Errorf("Could not write JSON to file: %s", err)
		return
	}

	// Call summarize()
	summarize(summarizeFlags{
		builds:       buildsPath,
		tests:        []string{testsPath},
		previous:     "",
		owners:       ownersPath,
		output:       "failure_data.json",
		outputSlices: "failure_data_PREFIX.json",
		numWorkers:   4, // Arbitrary number to keep tests more or less consistent across platforms
		memoize:      false,
	})

	// Test the output
	var output jsonOutput
	err = getJSON("failure_data.json", &output)
	if err != nil {
		t.Errorf("Could not retrieve summarize() results: %s", err)
	}

	// Grab two random hashes for use across some of the following tests
	randomHash1 := output.Clustered[0].ID
	randomHash2 := output.Clustered[1].ID

	t.Run("Main output", func(t *testing.T) {
		// Test each field as a subtest

		t.Run("builds", func(t *testing.T) {
			want := columns{
				Cols: columnarBuilds{
					Elapsed:  []int{8, 8, 4, 4, 4, 4},
					Executor: []string{"", "", "", "", "", ""},
					PR:       []string{"", "", "", "", "", ""},
					Result: []string{
						"SUCCESS",
						"FAILURE",
						"SUCCESS",
						"SUCCESS",
						"SUCCESS",
						"SUCCESS",
					},
					Started:     []int{1234, 1234, 1234, 1234, 1234, 1234},
					TestsFailed: []int{1, 1, 1, 1, 1, 1},
					TestsRun:    []int{2, 2, 2, 2, 2, 2},
				},
				JobPaths: map[string]string{
					"other-job": "gs://logs/other-job",
					"some-job":  "gs://logs/some-job",
				},
				Jobs: map[string]jobCollection{
					// JSON keys are always strings, so although this is created as a map[int]x,
					// we'll check it as a map[string]x
					"other-job": map[string]int{"5": 0, "7": 1},
					"some-job":  []int{1, 4, 2},
				},
			}

			got := output.Builds

			// Go through each of got's sections and determine if they match want
			t.Run("cols", func(t *testing.T) {
				wantCols := want.Cols
				gotCols := got.Cols

				intTestCases := []struct {
					name string
					got  []int
					want []int
				}{
					{"elapsed", gotCols.Elapsed, wantCols.Elapsed},
					{"started", gotCols.Started, wantCols.Started},
					{"testsFailed", gotCols.TestsFailed, wantCols.TestsFailed},
					{"testsRun", gotCols.TestsRun, wantCols.TestsRun},
				}

				for _, tc := range intTestCases {
					t.Run(tc.name, func(t *testing.T) {
						failOnMismatchedSlices(t, tc.want, tc.got)
					})
				}

				stringTestCases := []struct {
					name string
					got  []string
					want []string
				}{
					{"executor", gotCols.Executor, wantCols.Executor},
					{"pr", gotCols.PR, wantCols.PR},
					{"result", gotCols.Result, wantCols.Result},
				}

				for _, tc := range stringTestCases {
					t.Run(tc.name, func(t *testing.T) {
						failOnMismatchedSlices(t, tc.want, tc.got)
					})
				}
			})

			t.Run("jobPaths", func(t *testing.T) {
				wantJobPaths := want.JobPaths
				gotJobPaths := got.JobPaths

				if failOnDifferentLengths(t, len(wantJobPaths), len(gotJobPaths)) {
					return
				}

				failed := false
				for key, wantedResult := range wantJobPaths {
					// Check if each want key exists in got
					if gotResult, ok := gotJobPaths[key]; ok {
						// If so, do their values match?
						if wantedResult != gotResult {
							failed = true
							break
						}
					} else {
						failed = true
						break
					}
				}

				if failed {
					t.Errorf("Wanted result (%#v) and actual result (%#v) do not match", wantJobPaths, gotJobPaths)
				}
			})

			t.Run("jobs", func(t *testing.T) {
				wantJobs := want.Jobs
				gotJobs := got.Jobs

				if failOnDifferentLengths(t, len(wantJobs), len(gotJobs)) {
					return
				}

				for jobName := range wantJobs {
					t.Run(jobName, func(t *testing.T) {
						// The values before they are type-checked
						wantJobCollection := wantJobs[jobName]
						gotJobCollection := gotJobs[jobName]

						switch wantJobCollection := wantJobCollection.(type) {
						case map[string]int:
							wantMap := wantJobCollection
							// json.Unmarshal converts objects to map[string]interface{}, we'll have
							// to check the type of each value separately
							gotMap := gotJobCollection.(map[string]interface{})

							if failOnDifferentLengths(t, len(wantMap), len(gotMap)) {
								return
							}
							for key := range wantMap {
								wantVal := wantMap[key]
								if gotVal, ok := gotMap[key]; ok {
									// json.Unmarshal represents all numbers as float64, convert to int
									if wantVal != int(gotVal.(float64)) {
										t.Errorf("Wanted value of %d for key '%s', got %d", wantVal, key, int(gotVal.(float64)))
									}
								} else {
									t.Errorf("No value in gotMap for key '%s'", key)
								}
							}
						case []int:
							wantSlice := wantJobCollection
							// json.Unmarshal converts slices to []interface{}, we'll have
							// to check the type of each value separately
							gotSlice := gotJobCollection.([]interface{})
							if failOnDifferentLengths(t, len(wantSlice), len(gotSlice)) {
								return
							}
							for i := range wantSlice {
								// json.Unmarshal represents all numbers as float64, convert to int
								if wantSlice[i] != int(gotSlice[i].(float64)) {
									t.Errorf("Want slice (%#v) does not match got slice (%#v)", wantSlice, gotSlice)
									return
								}
							}
						}
					})
				}
			})
		})

		t.Run("clustered", func(t *testing.T) {
			want := []jsonCluster{
				{
					ID:  randomHash1,
					Key: "some awful stack trace exit 1",
					Tests: []test{
						{
							Jobs: []job{
								{
									BuildNumbers: []string{"4", "3", "2", "1"},
									Name:         "some-job",
								},
							},
							Name: "example test",
						},
					},
					Spans: []int{29},
					Owner: "node",
					Text:  "some awful stack trace exit 1",
				},
				{
					ID:  randomHash2,
					Key: "some other error message",
					Tests: []test{
						{
							Jobs: []job{
								{
									BuildNumbers: []string{"7", "5"},
									Name:         "other-job",
								},
							},
							Name: "unrelated test",
						},
						{
							Jobs: []job{
								{
									BuildNumbers: []string{"4"},
									Name:         "some-job",
								},
							},
							Name: "another test",
						},
					},
					Spans: []int{24},
					Owner: "testing",
					Text:  "some other error message",
				},
			}

			got := output.Clustered

			// Check lengths
			if failOnDifferentLengths(t, len(want), len(got)) {
				return
			}

			// Go through each of got's sections and determine if they match want
			for i := range want {
				currentWant := want[i]
				currentGot := got[i]
				t.Run(fmt.Sprintf("want[%d]", i), func(t *testing.T) {
					// Simple string equality checking
					stringTestCases := []struct {
						name string
						got  string
						want string
					}{
						{"id", currentWant.ID, currentWant.ID},
						{"key", currentWant.Key, currentWant.Key},
						{"owner", currentWant.Owner, currentWant.Owner},
						{"text", currentWant.Text, currentWant.Text},
					}
					for _, tc := range stringTestCases {
						t.Run(tc.name, func(t *testing.T) {
							if tc.want != tc.got {
								t.Errorf("Wanted result (%s) and actual result (%s) do not match", tc.want, tc.got)
							}
						})
					}

					// Simple int slice
					t.Run("spans", func(t *testing.T) {
						failOnMismatchedSlices(t, currentWant.Spans, currentWant.Spans)
					})

					// tests
					t.Run("tests", func(t *testing.T) {
						failOnMismatchedTestSlices(t, currentWant.Tests, currentGot.Tests)
					})
				})
			}
		})
	})

	t.Run("Slices", func(t *testing.T) {
		var renderedSlice renderedSliceOutput

		filepath := fmt.Sprintf("failure_data_%s.json", randomHash1[:2])

		err := getJSON(filepath, &renderedSlice)
		if err != nil {
			t.Error(err)
			return
		}

		t.Run("clustered", func(t *testing.T) {
			want := []jsonCluster{output.Clustered[0]}
			got := renderedSlice.Clustered

			if failOnDifferentLengths(t, len(want), len(got)) {
				return
			}
			for i := range want {
				t.Run(fmt.Sprintf("got[%d]", i), func(t *testing.T) {
					stringTestCases := []struct {
						name string
						want string
						got  string
					}{
						{"key", want[i].Key, got[i].Key},
						{"id", want[i].ID, got[i].ID},
						{"text", want[i].Text, got[i].Text},
						{"owner", want[i].Owner, got[i].Owner},
					}
					for _, tc := range stringTestCases {
						t.Run(tc.name, func(t *testing.T) {
							if tc.got != tc.want {
								t.Errorf("Wanted value (%#v) did not match actual value (%#v)", want, got)
							}
						})
					}

					t.Run("spans", func(t *testing.T) {
						failOnMismatchedSlices(t, want[i].Spans, got[i].Spans)
					})

					t.Run("tests", func(t *testing.T) {
						failOnMismatchedTestSlices(t, want[i].Tests, got[i].Tests)
					})
				})
			}
		})

		t.Run("builds.cols.started", func(t *testing.T) {
			want := []int{1234, 1234, 1234, 1234}
			got := renderedSlice.Builds.Cols.Started

			failOnMismatchedSlices(t, want, got)
		})
	})

	// Call summarize() with no owners file
	t.Run("No owners file", func(t *testing.T) {
		summarize(summarizeFlags{
			builds:       buildsPath,
			tests:        []string{testsPath},
			previous:     "",
			owners:       "",
			output:       "failure_data.json",
			outputSlices: "failure_data_PREFIX.json",
		})
	})
}
