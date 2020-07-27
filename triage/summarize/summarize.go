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

// Package summarize groups test failures together by finding edit distances between their failure messages,
// and emits JSON for rendering in a browser.
package summarize

import (
	"flag"
	"fmt"
	"log"
	"strings"
	"time"
)

const longOutputLen = 10000
const truncatedSep = "\n...[truncated]...\n"
const maxClusterTextLen = longOutputLen + len(truncatedSep)

// logInfo logs a message, prepending [I] to it.
func logInfo(format string, v ...interface{}) {
	log.Printf("[I]"+format, v...)
}

// logWarning logs a message, prepending [W] to it.
func logWarning(format string, v ...interface{}) {
	log.Printf("[W]"+format, v...)
}

// summarizeFlags represents the command-line arguments to the summarize and their values.
type summarizeFlags struct {
	builds       *string
	tests        []string
	previous     *string
	owners       *string
	output       *string
	outputSlices *string
}

// parseFlags parses command-line arguments and returns them as a summarizeFlags object.
func parseFlags() summarizeFlags {
	var flags summarizeFlags

	flags.builds = flag.String("builds", "", "path to builds.json file from BigQuery")
	tempTests := flag.String("tests", "", "path to tests.json files from BigQuery")
	flags.previous = flag.String("previous", "", "path to previous output")
	flags.owners = flag.String("owners", "", "path to test owner SIGs file")
	flags.output = flag.String("output", "failure_data.json", "output path")
	flags.outputSlices = flag.String("output_slices", "", "path to slices output (must include PREFIX in template)")

	// The tests flag can contain multiple arguments, so we'll split it by space
	flags.tests = strings.Split(*tempTests, " ")

	flag.Parse()

	return flags
}

func summarize(flags summarizeFlags) {
	builds, failedTests, err := loadFailures(*flags.builds, flags.tests)
	if err != nil {
		log.Fatalf("Could not load failures: %s", err)
	}

	var previousClustered []jsonCluster
	if *flags.previous != "" {
		logInfo("Loading previous")
		previousClustered, err = loadPrevious(*flags.previous)
		if err != nil {
			log.Fatalf("Could not get previous results: %s", err)
		}
	}

	clusteredLocal := clusterLocal(failedTests)

	clustered := clusterGlobal(clusteredLocal, previousClustered)

	logInfo("Rendering results...")
	start := time.Now()

	data := render(builds, clustered)

	var owners map[string][]string
	if *flags.owners != "" {
		owners, err = loadOwners(*flags.owners)
		if err != nil {
			logWarning("Could not get owners, clusters will not be labeled with owners: %s", err)

			// Set the flag to the empty string so the program doesn't try to write owners files later
			empty := ""
			flags.owners = &empty
		} else {
			err = annotateOwners(data, builds, owners)
			if err != nil {
				logWarning("Could not annotate owners, clusters will not be labeled with owners")

				empty := ""
				flags.owners = &empty
			}
		}
	}

	err = writeResults(*flags.output, data)
	if err != nil {
		logWarning("Could not write results to file: %s", err)
	}

	if *flags.outputSlices != "" {
		if !(strings.Contains(*flags.outputSlices, "PREFIX")) {
			log.Panic("'PREFIX' not in flags.output_slices")
		}

		for subset := 0; subset < 256; subset++ {
			idPrefix := fmt.Sprintf("%02x", subset)
			subset, cols := renderSlice(data, builds, idPrefix, "")
			err = writeRenderedSlice(strings.Replace(*flags.outputSlices, "PREFIX", idPrefix, -1), subset, cols)
			if err != nil {
				logWarning("Could not write subset %d to file: %s", subset, err)
			}
		}

		if *flags.owners != "" {
			// for output
			if _, ok := owners["testing"]; !ok {
				owners["testing"] = make([]string, 0)
			}

			for owner := range owners {
				ownerResults, cols := renderSlice(data, builds, "", owner)
				err = writeRenderedSlice(strings.Replace(*flags.outputSlices, "PREFIX", "sig-"+owner, -1), ownerResults, cols)
				if err != nil {
					logWarning("Could not write result for owner '%s' to file: %s", owner, err)
				}
			}
		}
	}

	logInfo("Finished rendering results in %s", time.Since(start).String())
}

func main() {
	summarize(parseFlags())
}
