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
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"k8s.io/test-infra/test-coverage/third_party/forked/gotestsum/junitxml"
)

// componentSemVerConstraintSuffixRE matches the trailing
// " [component: [constraint ...], ...]" suffix that Ginkgo's JUnit reporter
// appends to a test's name for tests using ComponentSemVerConstraint (e.g.
// framework.WithKubeletMinVersion), for example:
//
//	... with multiple drivers using only drapbv1 [KubeletMinVersion:1.34] work [kubelet: [>=1.34]]
//
// This suffix is not part of the test name as reported by --list-tests, so
// it must be stripped for the JUnit test name to match.
var componentSemVerConstraintSuffixRE = regexp.MustCompile(`\s\[\w+: \[[^][]*](?:, \w+: \[[^][]*])*]$`)

// jobRun identifies one execution ("build") of a Prow job.
type jobRun struct {
	job     string
	buildID string
	started time.Time
}

// findRecentRuns returns the runs of job that started at or after cutoff,
// newest first.
func findRecentRuns(client *gcsClient, job string, cutoff time.Time) ([]jobRun, error) {
	prefix := "logs/" + job + "/"
	prefixes, err := client.listPrefixes(prefix)
	if err != nil {
		return nil, err
	}

	buildIDs := make([]string, 0, len(prefixes))
	for _, p := range prefixes {
		id := strings.TrimSuffix(strings.TrimPrefix(p, prefix), "/")
		if id == "" {
			continue
		}
		buildIDs = append(buildIDs, id)
	}
	// Build IDs are monotonically increasing snowflake-like numbers, so
	// sorting numerically, descending, gives us newest-first order.
	sort.Slice(buildIDs, func(i, j int) bool {
		ni, erri := strconv.ParseUint(buildIDs[i], 10, 64)
		nj, errj := strconv.ParseUint(buildIDs[j], 10, 64)
		if erri != nil || errj != nil {
			return buildIDs[i] > buildIDs[j]
		}
		return ni > nj
	})

	var runs []jobRun
	for _, id := range buildIDs {
		started, ok, err := getStartedTime(client, job, id)
		if err != nil {
			log.Printf("warning: %s/%s: %v", job, id, err)
			continue
		}
		if !ok {
			// Build result not (yet) uploaded, e.g. still running.
			continue
		}
		if started.Before(cutoff) {
			// Runs are listed newest-first, so once we see one that
			// is too old, all further ones will be too.
			break
		}
		runs = append(runs, jobRun{job: job, buildID: id, started: started})
	}
	return runs, nil
}

// startedJSON is the relevant subset of the "started.json" metadata file
// that Prow writes for each job run.
type startedJSON struct {
	Timestamp int64 `json:"timestamp"`
}

// getStartedTime returns the start time recorded in started.json for the
// given run. ok is false if the file does not exist (e.g. the run has not
// finished uploading yet).
func getStartedTime(client *gcsClient, job, buildID string) (_ time.Time, ok bool, _ error) {
	data, err := client.getObject(fmt.Sprintf("logs/%s/%s/started.json", job, buildID))
	if err != nil {
		return time.Time{}, false, nil
	}
	var started startedJSON
	if err := json.Unmarshal(data, &started); err != nil {
		return time.Time{}, false, fmt.Errorf("parsing started.json: %w", err)
	}
	return time.Unix(started.Timestamp, 0), true, nil
}

// testResult is the outcome of one executed (non-skipped) test case.
type testResult struct {
	name   string
	passed bool
}

// executedTests downloads and parses all junit_*.xml files directly under
// the "artifacts" directory of run, and returns the results of all test
// cases (matching the "[It] " Ginkgo test names, with that prefix stripped)
// that belong to suite and were executed (i.e. not skipped).
//
// detectedSuite is the name of the Ginkgo suite that run actually executed,
// as taken from the top-level "testsuite" element's "name" attribute (e.g.
// "E2eNode Suite" or "Kubernetes e2e suite"). Some JUnit files (e.g.
// "junit_coverage.xml", which lists code coverage numbers instead of test
// results) have a "testsuite" root element without a "name" attribute at
// all; those are reported as "unknown" so that they are still recognized
// as JUnit data (just not belonging to suite), instead of being confused
// with the run having produced no JUnit data at all. detectedSuite is only
// empty if no junit files (or no "testsuite" elements) were found at all,
// e.g. because the job failed before running any tests; the caller should
// not treat that as having run the wrong suite. A "testsuite" named
// "kubetest" or "kubetest2" (as found in "junit_runner.xml", written by
// the kubetest/kubetest2 wrapper itself) is ignored entirely: it merely
// lists the
// wrapper's own high-level steps (e.g. "Prepare", "GetDeployer"), not
// which Ginkgo suite was actually run, and it is always present
// regardless of whether the run's real test suite produced any results
// at all (e.g. because the run failed before getting that far). Treating
// it as a real suite would incorrectly flag runs as "ran the wrong
// suite" whenever the real suite's JUnit file happens to be missing,
// which can happen even for jobs whose other, successful runs clearly
// do execute the expected suite. Some jobs (e.g.
// "ci-node-e2e-containerd-2-0-ubuntu-dra-alpha-beta-features") produce
// junit files with several different suite names in the same run, for
// example because they run more than one test binary. In that case, any
// well-known suite (i.e. "Kubernetes e2e suite" or "E2eNode Suite", not
// just the one currently being analyzed) found anywhere in the run is
// considered the "main" one and reported as detectedSuite. Otherwise, if
// more than five distinct "testsuite" elements are found, detectedSuite is
// reported as "many": this heuristic is for gotestsum-produced JUnit
// files, as found in e.g. "ci-kubernetes-integration", which contain one
// "testsuite" element per Go test package and thus have too many of them
// to meaningfully pick one as "the" suite that was executed. Failing both
// of those, the first suite name encountered is used, which is somewhat
// arbitrary but not wrong. Regardless of which suite is picked as
// detectedSuite, test cases are collected from all "testsuite" elements
// whose name matches suite.junitClassname.
func executedTests(client *gcsClient, run jobRun, suite e2eSuite) (tests []testResult, detectedSuite string, err error) {
	artifactsPrefix := fmt.Sprintf("logs/%s/%s/artifacts/", run.job, run.buildID)
	files, err := client.listFiles(artifactsPrefix)
	if err != nil {
		return nil, "", err
	}

	var firstSuiteName, mainSuiteName string
	suiteCount := 0
	for _, f := range files {
		base := path.Base(f)
		if !strings.HasPrefix(base, "junit_") || !strings.HasSuffix(base, ".xml") {
			continue
		}
		data, err := client.getObject(f)
		if err != nil {
			log.Printf("warning: %s: %v", f, err)
			continue
		}
		suites, err := parseJUnitTestSuites(data)
		if err != nil {
			log.Printf("warning: parsing %s: %v", f, err)
			continue
		}
		for _, s := range suites {
			name := s.Name
			if name == "" {
				name = "unknown"
			}
			if name == "kubetest" || name == "kubetest2" {
				// The kubetest/kubetest2 wrapper's own suite, not a
				// real test suite; see the function doc comment.
				continue
			}
			suiteCount++
			if firstSuiteName == "" {
				firstSuiteName = name
			}
			if mainSuiteName == "" && isKnownSuiteClassname(name) {
				mainSuiteName = name
			}
			if name != suite.junitClassname {
				// This testsuite element belongs to a different suite,
				// e.g. because the job under consideration ran both
				// "e2e" and "e2e_node" tests, or ran the wrong suite
				// altogether.
				continue
			}
			for _, c := range s.TestCases {
				if c.SkipMessage != nil {
					continue
				}
				name := strings.TrimPrefix(c.Name, "[It] ")
				if name == c.Name {
					// Not an "[It]" test case, e.g. "[SynchronizedBeforeSuite]".
					continue
				}
				name = componentSemVerConstraintSuffixRE.ReplaceAllString(name, "")
				tests = append(tests, testResult{name: name, passed: c.Failure == nil})
			}
		}
	}
	switch {
	case mainSuiteName != "":
		detectedSuite = mainSuiteName
	case suiteCount > 5:
		detectedSuite = "many"
	default:
		detectedSuite = firstSuiteName
	}
	return tests, detectedSuite, nil
}

// parseJUnitTestSuites parses a JUnit XML report whose root element is
// either <testsuites> or a single <testsuite>, and returns all the
// top-level test suites found in it.
func parseJUnitTestSuites(data []byte) ([]junitxml.JUnitTestSuite, error) {
	var suites junitxml.JUnitTestSuites
	if err := xml.Unmarshal(data, &suites); err == nil {
		return suites.Suites, nil
	}

	var suite junitxml.JUnitTestSuite
	if err := xml.Unmarshal(data, &suite); err != nil {
		return nil, err
	}
	return []junitxml.JUnitTestSuite{suite}, nil
}
