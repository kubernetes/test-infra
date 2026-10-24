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

package main

import (
	"fmt"
	"io"
	"sort"
	"sync"
	"text/tabwriter"
)

// execStats counts how often a test was executed, and how many of those
// executions passed.
type execStats struct {
	total  int
	passed int
}

// successRate returns the percentage of executions that passed.
func (s execStats) successRate() float64 {
	if s.total == 0 {
		return 0
	}
	return 100 * float64(s.passed) / float64(s.total)
}

// averageTests returns the average number of tests executed per run, given
// the total number of test executions and the number of runs.
func averageTests(totalExecutions, runs int) float64 {
	if runs == 0 {
		return 0
	}
	return float64(totalExecutions) / float64(runs)
}

// coverage tracks, for every known test, how often it was executed by each
// job and how often those executions passed.
//
// All methods are safe to call concurrently from multiple goroutines,
// e.g. while runs of different jobs are being analyzed in parallel.
type coverage struct {
	// allTests is the full set of test names as reported by --list-tests.
	allTests []string

	mu sync.Mutex
	// counts[testName][jobName] = execution stats of jobName for testName.
	counts map[string]map[string]*execStats
	// runsPerJob is the number of runs that were analyzed for each job:
	// this includes runs that turned out to execute the wrong suite (in
	// which case no tests are counted for them), but not runs that were
	// skipped entirely because an earlier (newer) run of the same job
	// already showed it runs the wrong suite.
	runsPerJob map[string]int
	// jobSuite[job] is the short name ("e2e", "e2e_node") of the suite
	// that job was detected to actually execute, or the raw JUnit
	// "testsuite" name if it doesn't belong to any known suite. It is
	// unset for jobs whose runs never produced any JUnit data.
	jobSuite map[string]string
	// jobs is the sorted list of job names that were analyzed.
	jobs []string
}

func newCoverage(allTests, jobs []string) *coverage {
	return &coverage{
		allTests:   allTests,
		counts:     make(map[string]map[string]*execStats),
		runsPerJob: make(map[string]int),
		jobSuite:   make(map[string]string),
		jobs:       jobs,
	}
}

func (cov *coverage) recordRun(job string) {
	cov.mu.Lock()
	defer cov.mu.Unlock()
	cov.runsPerJob[job]++
}

// recordJobSuite records the short name of the suite that job was detected
// to execute, based on a run's top-level JUnit "testsuite" name.
func (cov *coverage) recordJobSuite(job, suiteName string) {
	cov.mu.Lock()
	defer cov.mu.Unlock()
	cov.jobSuite[job] = suiteName
}

func (cov *coverage) recordExecution(job, test string, passed bool) {
	cov.mu.Lock()
	defer cov.mu.Unlock()
	byJob, ok := cov.counts[test]
	if !ok {
		byJob = make(map[string]*execStats)
		cov.counts[test] = byJob
	}
	stats, ok := byJob[job]
	if !ok {
		stats = &execStats{}
		byJob[job] = stats
	}
	stats.total++
	if passed {
		stats.passed++
	}
}

// overallSuccessRate returns the percentage of all recorded test executions,
// across all tests and jobs, that passed.
func (cov *coverage) overallSuccessRate() float64 {
	cov.mu.Lock()
	defer cov.mu.Unlock()
	var total execStats
	for _, byJob := range cov.counts {
		for _, s := range byJob {
			total.total += s.total
			total.passed += s.passed
		}
	}
	return total.successRate()
}

// writeReport writes a human-readable text report to w, using text/tabwriter
// to align the columns of the various tables it contains.
func (cov *coverage) writeReport(w io.Writer) error {
	totalRuns := 0
	for _, job := range cov.jobs {
		totalRuns += cov.runsPerJob[job]
	}

	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	var err error
	printf := func(format string, args ...interface{}) {
		if err != nil {
			return
		}
		_, err = fmt.Fprintf(tw, format, args...)
	}

	printf("Test execution coverage report\n\n")
	printf("Jobs analyzed (%d), with number of runs found and average tests executed per run (see https://prow.k8s.io/job-history/gs/kubernetes-ci-logs/logs/<job name>):\n", len(cov.jobs))
	jobExecTotals := make(map[string]int)
	for _, byJob := range cov.counts {
		for job, s := range byJob {
			jobExecTotals[job] += s.total
		}
	}
	jobsByRuns := append([]string(nil), cov.jobs...)
	sort.SliceStable(jobsByRuns, func(i, j int) bool {
		return cov.runsPerJob[jobsByRuns[i]] > cov.runsPerJob[jobsByRuns[j]]
	})
	totalExecutions := 0
	for _, job := range jobsByRuns {
		suiteName := cov.jobSuite[job]
		if suiteName == "" {
			if cov.runsPerJob[job] == 0 {
				// No runs at all were found for this job within
				// -age, so nothing can be said about which suite it
				// runs.
				suiteName = "unknown"
			} else {
				// Runs were analyzed, but none of them contained any
				// JUnit data at all (e.g. a build job).
				suiteName = "none"
			}
		}
		totalExecutions += jobExecTotals[job]
		printf("  %s\t%d\t%s\t%.1f\n", job, cov.runsPerJob[job], suiteName, averageTests(jobExecTotals[job], cov.runsPerJob[job]))
	}
	printf("  %s\t%d\t\t%.1f\n\n", "TOTAL", totalRuns, averageTests(totalExecutions, totalRuns))

	tests := append([]string(nil), cov.allTests...)
	sort.Strings(tests)

	var executed, notExecuted []string
	for _, test := range tests {
		if len(cov.counts[test]) > 0 {
			executed = append(executed, test)
		} else {
			notExecuted = append(notExecuted, test)
		}
	}

	printf("Executed tests (%d of %d, %.0f%% overall success rate):\n\n", len(executed), len(tests), cov.overallSuccessRate())
	for _, test := range executed {
		byJob := cov.counts[test]
		total := execStats{}
		for _, s := range byJob {
			total.total += s.total
			total.passed += s.passed
		}
		printf("%s: total %d %.1f%%\n", test, total.total, total.successRate())
		jobNames := make([]string, 0, len(byJob))
		for job := range byJob {
			jobNames = append(jobNames, job)
		}
		sort.Strings(jobNames)
		sort.SliceStable(jobNames, func(i, j int) bool {
			return byJob[jobNames[i]].total > byJob[jobNames[j]].total
		})
		for _, job := range jobNames {
			s := byJob[job]
			printf("    %s\t%d\t%.1f%%\n", job, s.total, s.successRate())
		}
	}

	printf("\nTests never executed (%d of %d):\n\n", len(notExecuted), len(tests))
	for _, test := range notExecuted {
		printf("%s\n", test)
	}

	if err != nil {
		return err
	}
	return tw.Flush()
}
