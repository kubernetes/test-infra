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
	builds       string
	tests        []string
	previous     string
	owners       string
	output       string
	outputSlices string
}

// parseFlags parses command-line arguments and returns them as a summarizeFlags object.
func parseFlags() summarizeFlags {
	var flags summarizeFlags

	flag.StringVar(&flags.builds, "builds", "", "path to builds.json file from BigQuery")
	flag.StringVar(&flags.previous, "previous", "", "path to previous output")
	flag.StringVar(&flags.owners, "owners", "", "path to test owner SIGs file")
	flag.StringVar(&flags.output, "output", "failure_data.json", "output path")
	flag.StringVar(&flags.outputSlices, "output_slices", "", "path to slices output (must include PREFIX in template)")

	// The tests flag can contain multiple arguments, so we'll split it by space
	tempTests := flag.String("tests", "", "path to tests.json files from BigQuery")

	flag.Parse()
	flags.tests = strings.Split(*tempTests, " ")

	return flags
}

func summarize(flags summarizeFlags) {
	builds, failedTests, err := loadFailures(flags.builds, flags.tests)
	if err != nil {
		log.Fatalf("Could not load failures: %s", err)
	}

	var previousClustered []jsonCluster
	if flags.previous != "" {
		logInfo("Loading previous")
		previousClustered, err = loadPrevious(flags.previous)
		if err != nil {
			log.Fatalf("Could not get previous results: %s", err)
		}
	}

	clusteredLocal := clusterLocal(failedTests)

	clustered := clusterGlobal(clusteredLocal, previousClustered)

	logInfo("Rendering results...")
	start := time.Now()

	data := render(builds, clustered)

	// Load the owners from the file, if given
	var owners map[string][]string
	if flags.owners != "" {
		owners, err = loadOwners(flags.owners)
		if err != nil {
			logWarning("Could not load owners file, clusters will only be labeled based on test names: %s", err)
		}
	}
	err = annotateOwners(&data, builds, owners)
	if err != nil {
		logWarning("Could not annotate owners: %s", err)
	}

	err = writeResults(flags.output, data)
	if err != nil {
		logWarning("Could not write results to file: %s", err)
	}

	if flags.outputSlices != "" {
		if !(strings.Contains(flags.outputSlices, "PREFIX")) {
			log.Panic("'PREFIX' not in flags.output_slices")
		}

		for subset := 0; subset < 256; subset++ {
			idPrefix := fmt.Sprintf("%02x", subset)
			subsetClusters, cols := renderSlice(data, builds, idPrefix, "")
			err = writeRenderedSlice(strings.Replace(flags.outputSlices, "PREFIX", idPrefix, -1), subsetClusters, cols)
			if err != nil {
				logWarning("Could not write subset %d to file: %s", subset, err)
			}
		}

		// If owners is nil, initialize it
		if owners == nil {
			owners = make(map[string][]string)
		}
		if _, ok := owners["testing"]; !ok {
			owners["testing"] = make([]string, 0)
		}
		for owner := range owners {
			ownerResults, cols := renderSlice(data, builds, "", owner)
			err = writeRenderedSlice(strings.Replace(flags.outputSlices, "PREFIX", "sig-"+owner, -1), ownerResults, cols)
			if err != nil {
				logWarning("Could not write result for owner '%s' to file: %s", owner, err)
			}
		}
	}

	logInfo("Finished rendering results in %s", time.Since(start).String())
}

func Main() {
	summarize(parseFlags())
}
