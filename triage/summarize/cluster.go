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
Contains functions that cluster failures.
*/

package summarize

import "time"

/*
clusterLocal clusters together the failures for each test. Failures come in grouped
by the test they belong to. These groups are subdivided into clusters.

Takes:
    {
		testName1: [failure1, failure2, failure3, failure4, ...],
		testName2: [failure5, failure6, failure7, failure8, failure9, ...],
		...
	}

Returns:
	{
		testName1: {
			clusterText1: [failure1, failure4, ...],
			clusterText2: [failure3, ...],
			clusterText3: [failure2, ...],
			...
		},
		testName2: {
			clusterText4: [failure5, failure7, failure8, ...],
			clusterText5: [failure6, failure9, ...],
			...
		},
		...
	}
*/
func clusterLocal(failuresByTest failuresGroup) nestedFailuresGroups {
	const memoPath string = "memo_cluster_local.json"
	const memoMessage string = "clustering inside each test"

	clustered := make(nestedFailuresGroups)

	// Try to retrieve memoized results first to avoid another computation
	if getMemoizedResults(memoPath, memoMessage, &clustered) {
		return clustered
	}

	numFailures := 0
	start := time.Now()
	logInfo("Clustering failures for %d unique tests...", len(failuresByTest))

	// Look at tests with the most failures first.
	for n, pair := range failuresByTest.sortByNumberOfFailures() {
		numFailures += len(pair.failures)
		logInfo("%4d/%4d tests, %5d failures, %s", n+1, len(failuresByTest), len(pair.failures), pair.key)
		clustered[pair.key] = clusterTest(pair.failures)
	}

	elapsed := time.Since(start)
	logInfo("Finished locally clustering %d unique tests (%d failures) in %s", len(clustered), numFailures, elapsed.String())

	// Memoize the results
	memoizeResults(memoPath, memoMessage, clustered)
	return clustered
}

// failure represents a specific instance of a test failure.
type failure struct {
	started     int
	build       string
	name        string
	failureText string `json:"failure_text"`
}

/*
clusterGlobal combines together clustered failures from each test. Clusters come in
grouped by the test their failures belong to. Similar cluster texts are merged across
tests, with their failures remaining grouped by test.

This is done hierarchically for efficiency-- each test's failures are likely to be similar,
reducing the number of clusters that need to be paired up at this stage.

previouslyClustered can be nil when there aren't previous results to use.

Takes:
	{
		testName1: {
			clusterText1: [failure1, failure4, ...],
			clusterText2: [failure3, ...],
			clusterText3: [failure2, ...],
			...
		},
		testName2: {
			clusterText4: [failure5, failure7, failure8, ...],
			clusterText5: [failure6, failure9, ...],
			...
		},
		...
	}

Returns:
	{
		clusterTextA: {
			testName1: [failure1, failure4, ...],
			testName 2: [failure5, failure7, failure8, ...],
			...
		},
		clusterTextB: {
			testName1: [failure2, ...],
			testName2: [failure6, failure9, ...],
			...
		},
		clusterTextC: {
			testName1: [failure3, ...],
			...
		}
		...
	}
*/
func clusterGlobal(newlyClustered nestedFailuresGroups, previouslyClustered []jsonCluster) nestedFailuresGroups {
	const memoPath string = "memo_cluster_global.json"
	const memoMessage string = "clustering across tests"

	// The eventual global clusters
	clusters := make(nestedFailuresGroups)

	// Try to retrieve memoized results first to avoid another computation
	if getMemoizedResults(memoPath, memoMessage, &clusters) {
		return clusters
	}

	numFailures := 0

	logInfo("Combining clustered failures for %d unique tests...", len(newlyClustered))
	start := time.Now()

	if previouslyClustered != nil {
		// Seed clusters using output from the previous run
		n := 0
		for _, cluster := range previouslyClustered {
			key := cluster.key
			normalizedKey := normalize(key)
			if key != normalizedKey {
				logInfo(key)
				logInfo(normalizedKey)
				n++
				continue
			}

			clusters[key] = make(failuresGroup)
		}

		logInfo("Seeding with %d previous clusters", len(clusters))

		if n != 0 {
			logWarning("!!! %d clusters lost from different normalization! !!!", n)
		}
	}

	// Look at tests with the most failures over all clusters first
	for n, outerPair := range newlyClustered.sortByAggregateNumberOfFailures() {
		testName := outerPair.key
		testClusters := outerPair.group

		logInfo("%4d/%4d tests, %4d clusters, %s", n+1, len(newlyClustered), len(testClusters), testName)
		testStart := time.Now()

		// Look at clusters with the most failures first
		for m, innerPair := range testClusters.sortByNumberOfFailures() {
			key := innerPair.key // The cluster text
			tests := innerPair.failures

			clusterStart := time.Now()
			fTextLen := len(key)
			numClusters := len(testClusters)
			numTests := len(tests)
			var clusterCase string

			logInfo("  %4d/%4d clusters, %5d chars failure text, %5d failures ...", m, numClusters, fTextLen, numTests)
			numFailures += numTests

			// If a cluster exists for the given cluster text
			if _, ok := clusters[key]; ok {
				clusterCase = "EXISTING"

				// If there isn't yet a slice of failures for that test, make a new one
				if _, ok := clusters[key][testName]; !ok {
					clusters[key][testName] = make([]failure, 0, len(tests))
				}

				// Copy the contents into the test's failure slice
				clusters[key][testName] = append(clusters[key][testName], tests...)
			} else if time.Since(testStart).Seconds() > 30 && fTextLen > maxClusterTextLen/2 && numTests == 1 {
				// if we've taken longer than 30 seconds for this test, bail on pathological / low value cases
				clusterCase = "BAILED"
			} else {
				other, found := findMatch(key, clusters.keys())
				if found {
					clusterCase = "OTHER"

					// If there isn't yet a slice of failures that test, make a new one
					if _, ok := clusters[other][testName]; !ok {
						clusters[other][testName] = make([]failure, 0, len(tests))
					}

					// Copy the contents into the test's failure slice
					clusters[other][testName] = append(clusters[other][testName], tests...)
				} else {
					clusterCase = "NEW"
					clusters[key] = failuresGroup{testName: tests}
				}
			}

			clusterDuration := int(time.Since(clusterStart).Seconds())
			logInfo("  %4d/%4d clusters, %5d chars failure text, %5d failures, cluster:%s in %d sec, test: %s",
				m, numClusters, fTextLen, numTests, clusterCase, clusterDuration, testName)
		}
	}

	// If we seeded clusters using the previous run's keys, some of those
	// clusters may have disappeared. Remove the resulting empty entries.
	for key, val := range clusters {
		if len(val) == 0 {
			delete(clusters, key)
		}
	}

	elapsed := time.Since(start)
	logInfo("Finished clustering %d unique tests (%d failures) into %d clusters in %s",
		len(newlyClustered), numFailures, len(clusters), elapsed.String())

	// Memoize the results
	memoizeResults(memoPath, memoMessage, clusters)
	return clusters
}

/* Functions below this comment are only used within this file as of this commit. */

/*
clusterTest clusters a given a list of failures for one test.
Failure texts are normalized prior to clustering to avoid needless entropy.

Takes:
	[]failure

Returns:
	{
		clusterText1: [failure1, failure2, ...],
		clusterText2: [failure3, failure4, failure5,...],
		...
	}
*/
func clusterTest(failures []failure) failuresGroup {
	result := make(failuresGroup, len(failures))
	start := time.Now()

	for _, flr := range failures {
		fNorm := normalize(flr.failureText)

		// If this string is already in the result list, store it
		if _, ok := result[fNorm]; ok {
			result[fNorm] = append(result[fNorm], flr)
		} else {
			// Otherwise, check if a match can be found for the normalized string
			other, found := findMatch(fNorm, result.keys())
			if found {
				result[other] = append(result[other], flr)
			} else {
				result[fNorm] = []failure{flr}
			}
		}

		// Bail if the clustering takes too long
		if time.Since(start).Seconds() > 60 {
			logInfo("bailing early, taking too long!")
			break
		}
	}

	return result
}
