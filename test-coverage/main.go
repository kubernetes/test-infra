/*
Copyright The Kubernetes Authors.

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

// Command coverage reports how often each test of a Kubernetes Ginkgo e2e
// suite ("e2e" or "e2e_node") has actually been executed by the periodic
// Prow jobs defined in a given job directory, over a recent window of time.
//
// It must be run with the current working directory set to the root of the
// test-infra repository, since it needs to load the main Prow config from
// there.
//
// Example:
//
//	go run ./coverage -kubernetes-repo ../kubernetes -e2e-suite e2e_node -job-filter '^ci-node-crio-'
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"regexp"
	"sort"
	"sync"
	"syscall"
	"time"
)

func main() {
	// sigs.k8s.io/prow/pkg/config (used by listPeriodicJobs) transitively
	// imports sigs.k8s.io/prow/pkg/interrupts, whose init() function
	// installs its own signal.Notify handler for SIGINT/SIGTERM. Since
	// this tool never calls interrupts.WaitForGracefulShutdown, that
	// handler just swallows the signal without doing anything, which
	// disables Go's default behavior of terminating the process on
	// SIGINT (i.e. Ctrl-C stops working). Go delivers a signal to every
	// channel registered for it via signal.Notify, so registering our
	// own handler here restores the ability to interrupt the tool.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		s := <-sigCh
		log.Printf("Received %s, exiting.", s)
		os.Exit(1)
	}()

	kubernetesRepo := flag.String("kubernetes-repo", "", "Path to a kubernetes/kubernetes repository checkout (required).")
	e2eSuiteName := flag.String("e2e-suite", "e2e_node", `Which Ginkgo test suite in the "test" directory to analyze: "e2e" or "e2e_node".`)
	jobDir := flag.String("job-dir", "", "Path to a directory of periodic Prow job definitions, relative to the test-infra repository root. Defaults to the standard job directory for -e2e-suite.")
	age := flag.Duration("age", 24*time.Hour, "How far back to look for job runs.")
	jobFilter := flag.String("job-filter", ".*", "Regular expression used to filter periodic job names.")
	workers := flag.Int("workers", 10, "Number of concurrent workers used to download and analyze job runs from GCS.")
	flag.Parse()

	if *kubernetesRepo == "" {
		fmt.Fprintln(os.Stderr, "Usage: coverage -kubernetes-repo <path> [-e2e-suite e2e|e2e_node] [-job-dir <path>] [-age <duration>] [-job-filter <regexp>] [-workers <n>]")
		flag.PrintDefaults()
		os.Exit(2)
	}

	suite, err := lookupE2ESuite(*e2eSuiteName)
	if err != nil {
		log.Fatal(err)
	}

	if *jobDir == "" {
		*jobDir = suite.defaultJobDir
	}

	jobFilterRE, err := regexp.Compile(*jobFilter)
	if err != nil {
		log.Fatalf("invalid -job-filter %q: %v", *jobFilter, err)
	}

	if *workers < 1 {
		log.Fatalf("invalid -workers %d: must be at least 1", *workers)
	}

	if err := run(*kubernetesRepo, suite, *jobDir, *age, jobFilterRE, *workers, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func run(kubernetesRepo string, suite e2eSuite, jobDir string, age time.Duration, jobFilter *regexp.Regexp, workers int, out *os.File) error {
	log.Printf("Listing %s tests in %s ...", suite.testPackage, kubernetesRepo)
	tests, err := listGinkgoTests(kubernetesRepo, suite.testPackage)
	if err != nil {
		return fmt.Errorf("listing %s tests: %w", suite.testPackage, err)
	}
	log.Printf("Found %d tests.", len(tests))

	log.Printf("Listing periodic jobs in %s matching %q ...", jobDir, jobFilter)
	jobs, err := listPeriodicJobs(jobDir, jobFilter)
	if err != nil {
		return fmt.Errorf("listing periodic jobs: %w", err)
	}
	log.Printf("Found %d periodic jobs: %v", len(jobs), jobs)

	cutoff := time.Now().Add(-age)
	client := newGCSClient()
	cov := newCoverage(tests, jobs)

	// Finding the recent runs of each job is independent per job, so it
	// is done by up to workers goroutines in parallel.
	var runsMu sync.Mutex
	runsByJob := make(map[string][]jobRun)
	forEachParallel(jobs, workers, func(job string) {
		log.Printf("Finding runs of %s since %s ...", job, cutoff.Format(time.RFC3339))
		runs, err := findRecentRuns(client, job, cutoff)
		if err != nil {
			log.Printf("warning: %s: %v", job, err)
			return
		}
		log.Printf("Found %d runs of %s.", len(runs), job)
		runsMu.Lock()
		defer runsMu.Unlock()
		runsByJob[job] = runs
	})

	// Downloading and parsing the JUnit results of a job's runs is done
	// job by job (newest run first), so that a job is only ever handled
	// by a single worker: once a run is found that executed the wrong
	// suite, the remaining (older) runs of that job are skipped, since
	// it is very unlikely that they ran a different suite than more
	// recent runs of the same job. Distributing work by job like this,
	// rather than by individual run, guarantees that a non-matching job
	// is analyzed exactly once, without needing any shared state (e.g. a
	// sync.Map) to coordinate between workers. cov is safe for
	// concurrent use.
	//
	// The suite reported for a job is fixed to whatever was determined
	// by the first (newest) run for which a suite could be detected at
	// all, and is not overwritten by older runs: a job whose recent runs
	// correctly executed the expected suite, but which then hits an
	// older run that (e.g. due to an infrastructure failure) ran a
	// different or no recognizable suite, should still be reported as
	// running the suite its recent runs actually ran, not the
	// unrepresentative older one that merely caused the scan to stop.
	//
	// Jobs are processed with the ones that have the most runs first,
	// since those take the longest; starting them last, after all
	// smaller jobs have already finished and their workers are idle,
	// would needlessly delay completion.
	sort.SliceStable(jobs, func(i, j int) bool {
		return len(runsByJob[jobs[i]]) > len(runsByJob[jobs[j]])
	})
	forEachParallel(jobs, workers, func(job string) {
		analyzed := 0
		suiteKnown := false
		for _, r := range runsByJob[job] {
			tests, detectedSuite, err := executedTests(client, r, suite)
			if err != nil {
				log.Printf("warning: %s/%s: %v", r.job, r.buildID, err)
				continue
			}
			analyzed++
			if detectedSuite != "" && detectedSuite != suite.junitClassname {
				suiteName := suiteNameForClassname(detectedSuite)
				expectedSuiteName := suiteNameForClassname(suite.junitClassname)
				log.Printf("Ignoring %s: ran suite %q instead of %q.", job, suiteName, expectedSuiteName)
				// Only record this as the job's suite if no earlier
				// (newer) run already determined it: older runs that
				// ran a different (or no) suite, e.g. because of an
				// infrastructure failure, must not override the
				// correct suite already established by more recent
				// runs.
				if !suiteKnown {
					cov.recordJobSuite(job, suiteName)
				}
				cov.recordRun(job)
				break
			}
			if detectedSuite != "" {
				if !suiteKnown {
					cov.recordJobSuite(job, suiteNameForClassname(detectedSuite))
					suiteKnown = true
				}
			}
			cov.recordRun(job)
			for _, test := range tests {
				cov.recordExecution(job, test.name, test.passed)
			}
		}
		log.Printf("Analyzed %d runs of %s.", analyzed, job)
	})

	return cov.writeReport(out)
}
