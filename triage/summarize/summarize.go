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
	"runtime"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

const longOutputLen = 10000
const truncatedSep = "\n...[truncated]...\n"
const maxClusterTextLen = longOutputLen + len(truncatedSep)

// summarizeFlags represents the command-line arguments to the summarize and their values.
type summarizeFlags struct {
	builds       string
	tests        []string
	previous     string
	owners       string
	output       string
	outputSlices string
	numWorkers   int
	memoize      bool
}

// parseFlags parses command-line arguments and returns them as a summarizeFlags object.
func parseFlags() summarizeFlags {
	var flags summarizeFlags

	flag.StringVar(&flags.builds, "builds", "", "path to builds.json file from BigQuery")
	flag.StringVar(&flags.previous, "previous", "", "path to previous output")
	flag.StringVar(&flags.owners, "owners", "", "path to test owner SIGs file")
	flag.StringVar(&flags.output, "output", "failure_data.json", "output path")
	flag.StringVar(&flags.outputSlices, "output_slices", "", "path to slices output (must include PREFIX in template)")
	flag.IntVar(&flags.numWorkers, "num_workers", 2*runtime.NumCPU()-1, "number of worker goroutines to spawn for parallelized functions") // This has shown to be a sensible number of workers
	flag.BoolVar(&flags.memoize, "memoize", false, "whether to memoize certain function results to JSON (and use previously memoized results if they exist)")

	flag.Parse()
	// list of tests files comes from arguments
	flags.tests = flag.Args()

	// Do some checks on the flags
	if !(strings.Contains(flags.outputSlices, "PREFIX")) {
		klog.Fatalf("'PREFIX' not in output_slices flag")
	}

	return flags
}

// setUpLogging adds flags that determine logging behavior for klog. See klog's documentation for how
// these flags work.
func setUpLogging(logtostderr bool, v int) {
	klogFlags := flag.NewFlagSet("klog", flag.PanicOnError)
	klog.InitFlags(klogFlags) // Add the klog flags

	// Set the flags
	err := klogFlags.Set("logtostderr", fmt.Sprint(logtostderr))
	if err != nil {
		klog.Fatalf("Could not set klog flag 'logtostderr': %s", err)
	}

	err = klogFlags.Set("v", fmt.Sprint(v))
	if err != nil {
		klog.Fatalf("Could not set klog flag 'v': %s", err)
	}
}

func summarize(flags summarizeFlags) {
	setUpLogging(true, 3)

	// Log flag info
	klog.V(1).Infof("Running with %d workers (%d detected CPUs)", flags.numWorkers, runtime.NumCPU())

	builds, failedTests, err := loadFailures(flags.builds, flags.tests, flags.memoize)
	if err != nil {
		klog.Fatalf("Could not load failures: %s", err)
	}

	var previousClustered []jsonCluster
	if flags.previous != "" {
		klog.V(2).Infof("Loading previous")
		previousClustered, err = loadPrevious(flags.previous)
		if err != nil {
			klog.Warningf("Could not get previous results, they will not be used: %s", err)
		}
	}

	clusteredLocal := clusterLocal(failedTests, flags.numWorkers, flags.memoize)

	clustered := clusterGlobal(clusteredLocal, previousClustered, flags.memoize)

	klog.V(2).Infof("Rendering results...")
	start := time.Now()

	data := render(builds, clustered)

	// Load the owners from the file, if given
	var owners map[string][]string
	if flags.owners != "" {
		owners, err = loadOwners(flags.owners)
		if err != nil {
			klog.Warningf("Could not load owners file, clusters will only be labeled based on test names: %s", err)
		}
	}
	err = annotateOwners(&data, builds, owners)
	if err != nil {
		klog.Warningf("Could not annotate owners: %s", err)
	}

	err = writeResults(flags.output, data)
	if err != nil {
		klog.Warningf("Could not write results to file: %s", err)
	}

	if flags.outputSlices != "" {
		for subset := 0; subset < 256; subset++ {
			idPrefix := fmt.Sprintf("%02x", subset)
			subsetClusters, cols := renderSlice(data, builds, idPrefix, "")
			err = writeRenderedSlice(strings.Replace(flags.outputSlices, "PREFIX", idPrefix, -1), subsetClusters, cols)
			if err != nil {
				klog.Warningf("Could not write subset %d to file: %s", subset, err)
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
				klog.Warningf("Could not write result for owner '%s' to file: %s", owner, err)
			}
		}
	}

	klog.V(0).Infof("Finished rendering results in %s", time.Since(start).String())
}

func Main() {
	summarize(parseFlags())
}
