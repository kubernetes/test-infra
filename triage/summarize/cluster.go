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

import (
	"sync"
	"time"

	"k8s.io/klog/v2"
)

/*
clusterLocal clusters together the failures for each test. Failures come in grouped
by the test they belong to. These groups are subdivided into clusters.

numWorkers determines how many goroutines to spawn to simultaneously process the failure
groups. If numWorkers <= 0, the value is set to 1.

memoize determines if memoized results should attempt to be retrieved, and if new results should be
memoized to JSON.

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
func clusterLocal(failuresByTest failuresGroup, numWorkers int, memoize bool) nestedFailuresGroups {
	const memoPath string = "memo_cluster_local.json"
	const memoMessage string = "clustering inside each test"

	clustered := make(nestedFailuresGroups)

	// Try to retrieve memoized results first to avoid another computation
	if memoize && getMemoizedResults(memoPath, memoMessage, &clustered) {
		return clustered
	}

	numFailures := 0 // The number of failures processed so far
	start := time.Now()
	klog.V(2).Infof("Clustering failures for %d unique tests...", len(failuresByTest))

	/*
		Since local clustering is done within each test, there is no interdependency among the clusters,
		and the clustering process can be easily parallelized.

		One goroutine will push to a work queue, and one goroutine will read from a done queue.
		The specified number of goroutines will act as workers. Each worker will pull from the work
		queue and perform a clustering operation, and then push the result to the done queue.
	*/

	var workerWG sync.WaitGroup
	var doneQueueWG sync.WaitGroup // Prevents this function from returning results before they are all written to clustered

	if numWorkers <= 0 {
		numWorkers = 1
	}

	workQueue := make(chan *failuresGroupPair, numWorkers)

	type doneGroup struct {
		input  *failuresGroupPair // The failures to be clustered
		output failuresGroup      // The clustered failures
	}
	doneQueue := make(chan doneGroup, numWorkers)

	// Push to the work queue
	go func() {
		// Look at tests with the most failures first.
		sortedFailures := failuresByTest.sortByMostFailures()
		for i := range sortedFailures {
			workQueue <- &sortedFailures[i]
		}

		// Close the channel so the workers know to stop
		close(workQueue)
	}()

	// Read from the done queue
	doneQueueWG.Add(1)
	go func() {
		defer doneQueueWG.Done()

		for dg := range doneQueue {
			numFailures += len(dg.input.Failures)
			klog.V(3).Infof("%4d/%4d tests, %5d failures, %s", len(clustered)+1, len(failuresByTest), len(dg.input.Failures), dg.input.Key)
			clustered[dg.input.Key] = dg.output
		}
	}()

	// Create the workers
	for i := 0; i < numWorkers; i++ {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			for pair := range workQueue {
				doneQueue <- doneGroup{
					pair,
					clusterTest(pair.Failures),
				}
			}
		}()
	}

	// Wait for the workers to finish
	workerWG.Wait()
	// Close the done queue so the doneQueue goroutine knows to stop
	close(doneQueue)
	// Wait for the doneQueue goroutine
	doneQueueWG.Wait()

	elapsed := time.Since(start)
	klog.V(2).Infof("Finished locally clustering %d unique tests (%d failures) in %s", len(clustered), numFailures, elapsed.String())

	// Memoize the results
	if memoize {
		memoizeResults(memoPath, memoMessage, clustered)
	}
	return clustered
}

// failure represents a specific instance of a test failure.
type failure struct {
	Started     int    `json:"started"`
	Build       string `json:"build"`
	Name        string `json:"name"`
	FailureText string `json:"failure_text"`
}

/*
clusterGlobal combines together clustered failures from each test. Clusters come in
grouped by the test their failures belong to. Similar cluster texts are merged across
tests, with their failures remaining grouped by test.

This is done hierarchically for efficiency-- each test's failures are likely to be similar,
reducing the number of clusters that need to be paired up at this stage.

previouslyClustered can be nil when there aren't previous results to use.

memoize determines if memoized results should attempt to be retrieved, and if new results should be
memoized to JSON.

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
func clusterGlobal(newlyClustered nestedFailuresGroups, previouslyClustered []jsonCluster, memoize bool) nestedFailuresGroups {
	const memoPath string = "memo_cluster_global.json"
	const memoMessage string = "clustering across tests"

	// The eventual global clusters
	clusters := make(nestedFailuresGroups)

	// Try to retrieve memoized results first to avoid another computation
	if memoize && getMemoizedResults(memoPath, memoMessage, &clusters) {
		return clusters
	}

	numFailures := 0

	klog.V(2).Infof("Combining clustered failures for %d unique tests...", len(newlyClustered))
	start := time.Now()

	if previouslyClustered != nil {
		// Seed clusters using output from the previous run
		n := 0
		for _, cluster := range previouslyClustered {
			key := cluster.Key
			normalizedKey := normalize(key)
			if key != normalizedKey {
				klog.V(4).Infof(key)
				klog.V(4).Infof(normalizedKey)
				n++
				continue
			}

			clusters[key] = make(failuresGroup)
		}

		klog.V(2).Infof("Seeding with %d previous clusters", len(clusters))

		if n != 0 {
			klog.Warningf("!!! %d clusters lost from different normalization! !!!", n)
		}
	}

	// Look at tests with the most failures over all clusters first
	for n, outerPair := range newlyClustered.sortByMostAggregatedFailures() {
		testName := outerPair.Key
		testClusters := outerPair.Group

		klog.V(3).Infof("%4d/%4d tests, %4d clusters, %s", n+1, len(newlyClustered), len(testClusters), testName)
		testStart := time.Now()

		// Look at clusters with the most failures first
		for m, innerPair := range testClusters.sortByMostFailures() {
			key := innerPair.Key // The cluster text
			tests := innerPair.Failures

			clusterStart := time.Now()
			fTextLen := len(key)
			numClusters := len(testClusters)
			numTests := len(tests)
			var clusterCase string

			klog.V(3).Infof("  %4d/%4d clusters, %5d chars failure text, %5d failures ...", m+1, numClusters, fTextLen, numTests)
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
			klog.V(3).Infof("  %4d/%4d clusters, %5d chars failure text, %5d failures, cluster:%s in %d sec, test: %s",
				m+1, numClusters, fTextLen, numTests, clusterCase, clusterDuration, testName)
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
	klog.V(2).Infof("Finished clustering %d unique tests (%d failures) into %d clusters in %s",
		len(newlyClustered), numFailures, len(clusters), elapsed.String())

	// Memoize the results
	if memoize {
		memoizeResults(memoPath, memoMessage, clusters)
	}
	return clusters
}

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
		fNorm := normalize(flr.FailureText)

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
			klog.V(2).Infof("bailing early, taking too long!")
			break
		}
	}

	return result
}
