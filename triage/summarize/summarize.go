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

// TODO Fix function and variable case to use camelCase and remove named parameters
// TODO Revise comments to better match arguments
// TODO Move structs to be before their first uses, including build and failure
// TODO Define build, test, job, failure, etc. in the README
// TODO Go through function chain in README
// TODO add JSON annotations to build and failure, and any other field names necessary

// Package summarize groups test failures together by finding edit distances between their failure messages,
// and emits JSON for rendering in a browser.
package summarize

import (
	"crypto/sha1"
	"flag"
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"regexp"
	"sort"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/triage/berghelroach"
	"k8s.io/test-infra/triage/utils"
)

var flakeReasonDateRE *regexp.Regexp = regexp.MustCompile(
	`[A-Z][a-z]{2}, \d+ \w+ 2\d{3} [\d.-: ]*([-+]\d+)?|` +
		`\w{3}\s+\d{1,2} \d+:\d+:\d+(\.\d+)?|(\d{4}-\d\d-\d\d.|.\d{4} )\d\d:\d\d:\d\d(.\d+)?`)

// Find random noisy strings that should be replaced with renumbered strings, for more similarity.
var flakeReasonOrdinalRE *regexp.Regexp = regexp.MustCompile(
	`0x[0-9a-fA-F]+` + // hex constants
		`|\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}(:\d+)?` + // IPs + optional port
		`|[0-9a-fA-F]{8}-\S{4}-\S{4}-\S{4}-\S{12}(-\d+)?` + // UUIDs + trailing digits
		`|[0-9a-f]{12,32}` + // hex garbage
		`|(minion-group-|default-pool-)[-0-9a-z]{4,}`) // node names

const longOutputLen = 10000
const truncatedSep = "\n...[truncated]...\n"
const maxClusterTextLen = longOutputLen + len(truncatedSep)

// log logs a message, prepending [I] to it.
func logInfo(format string, v ...interface{}) {
	log.Printf("[I]"+format, v)
}

// log logs a message, prepending [W] to it.
func logWarning(format string, v ...interface{}) {
	log.Printf("[W]"+format, v)
}

// build represents a specific instance of a build.
type build struct {
	path        string
	started     int
	elapsed     int
	testsRun    int
	testsFailed int
	result      string
	executor    string
	job         string
	number      int
	pr          string
	key         string // Often nonexistent
}

// failure represents a specific instance of a test failure.
type failure struct {
	started     int
	build       string
	name        string
	failureText string
}

/*
normalize reduces excess entropy to make clustering easier, given
a traceback or error message from a text.

This includes:

- blanking dates and timestamps

- renumbering unique information like

-- pointer addresses

-- UUIDs

-- IP addresses

- sorting randomly ordered map[] strings.
*/
func normalize(s string) string {
	// blank out dates
	s = flakeReasonDateRE.ReplaceAllLiteralString(s, "TIME")

	// do alpha conversion-- rename random garbage strings (hex pointer values, node names, etc)
	// into 'UNIQ1', 'UNIQ2', etc.
	var matches map[string]string

	// Go's maps are in a random order. Try to sort them to reduce diffs.
	if strings.Contains(s, "map[") {
		// Match anything of the form "map[x]", where x does not contain "]" or "["
		sortRE := regexp.MustCompile(`map\[([^][]*)\]`)
		s = sortRE.ReplaceAllStringFunc(
			s,
			func(match string) string {
				// Access the 1th submatch to grab the capture goup in sortRE.
				// Split the capture group by " " so it can be sorted.
				splitMapTypes := strings.Split(sortRE.FindStringSubmatch(match)[1], " ")
				sort.StringSlice.Sort(splitMapTypes)

				// Rejoin the sorted capture group with " ", and insert it back into "map[]"
				return fmt.Sprintf("map[%s]", strings.Join(splitMapTypes, " "))
			})
	}

	s = flakeReasonOrdinalRE.ReplaceAllStringFunc(s, func(match string) string {
		if _, ok := matches[match]; !ok {
			matches[match] = fmt.Sprintf("UNIQ%d", len(matches)+1)
		}
		return matches[match]
	})

	// for long strings, remove repeated lines!
	if len(s) > longOutputLen {
		s = utils.RemoveDuplicateLines(s)
	}

	// truncate ridiculously long test output
	if len(s) > longOutputLen {
		s = s[:longOutputLen/2] + truncatedSep + s[len(s)-longOutputLen/2:]
	}

	return s
}

// normalizeName removes [...] and {...} from a given test name. It matches code in testgrid and
// kubernetes/hack/update_owners.py.
func normalizeName(name string) string {
	name = regexp.MustCompile(`\[.*?\]|{.*?\}`).ReplaceAllLiteralString(name, "")
	name = regexp.MustCompile(`\s+`).ReplaceAllLiteralString(name, " ")
	return strings.TrimSpace(name)
}

// Will be used across makeNgramCounts() calls
var memoizedNgramCounts map[string][]int

/*
makeNgramCounts converts a string into a histogram of frequencies for different byte combinations.
This can be used as a heuristic to estimate edit distance between two strings in
constant time.

Instead of counting each ngram individually, they are hashed into buckets.
This makes the output count size constant.
*/
func makeNgramCounts(s string) []int {
	size := 64
	if _, ok := memoizedNgramCounts[s]; ok {
		counts := make([]int, size)
		for x := 0; x < len(s)-3; x++ {
			counts[int(crc32.Checksum([]byte(s[x:x+4]), crc32.MakeTable(0))&uint32(size-1))]++
		}
		memoizedNgramCounts[s] = counts // memoize
	}
	return memoizedNgramCounts[s]
}

/*
ngramEditDist computes a heuristic lower-bound edit distance using ngram counts.

An insert/deletion/substitution can cause up to 4 ngrams to differ:

	abcdefg => abcefg
	(abcd, bcde, cdef, defg) => (abce, bcef, cefg)

This will underestimate the edit distance in many cases:

- ngrams hashing into the same bucket will get confused

- a large-scale transposition will barely disturb ngram frequencies,
	but will have a very large effect on edit distance.

It is useful to avoid more expensive precise computations when they are
guaranteed to exceed some limit (being a lower bound), or as a proxy when
the exact edit distance computation is too expensive (for long inputs).
*/
func ngramEditDist(a string, b string) int {
	countsA := makeNgramCounts(a)
	countsB := makeNgramCounts(b)

	shortestCounts := utils.Min(len(countsA), len(countsB))
	result := 0
	for i := 0; i < shortestCounts; i++ {
		result += utils.Abs(countsA[i] - countsB[i])
	}

	return result / 4
}

// makeNgramCountsDigest returns a hashed version of the ngram counts.
func makeNgramCountsDigest(s string) string {
	ngramResults := makeNgramCounts(s)
	// Build a string representation of the ngram results that can then be hashed
	var builder strings.Builder

	// In Python, given an array [x, y, z], calling str() on the array will output
	// "[x, y, z]". This will try to replicate that behavior.
	builder.WriteString("[")
	for i := 0; i < len(ngramResults)-1; i++ {
		builder.WriteString(fmt.Sprintf("%d, ", ngramResults[i]))
	}
	// Add the last element separately to avoid a trailing comma and space,
	// and add the closing bracket.
	builder.WriteString(fmt.Sprintf("%d]", ngramResults[len(ngramResults)-1]))

	// Generate the hash
	hash := sha1.New()
	_, err := io.WriteString(hash, builder.String())
	if err != nil {
		log.Fatal("Error writing ngram results string to sha1 hash.")
	}

	return fmt.Sprintf("%x", hash.Sum(nil)[:20])
}

// findMatch finds a match for a normalized failure string from a selection of candidates.
func findMatch(fnorm string, candidates []string) (result string, found bool) {
	type distancePair struct {
		distResult int
		key        string
	}

	distancePairs := make([]distancePair, len(candidates))

	iter := 0
	for _, candidate := range candidates {
		distancePairs[iter] = distancePair{ngramEditDist(fnorm, candidate), candidate}
		iter++
	}
	// Sort distancePairs by each pair's distResult
	sort.Slice(distancePairs, func(i, j int) bool { return distancePairs[i].distResult < distancePairs[j].distResult })

	for _, pair := range distancePairs {
		// allow up to 10% differences
		limit := int(float32(len(fnorm)+len(pair.key)) / 2.0 * 0.10)

		if pair.distResult > limit {
			continue
		}

		if limit <= 1 && pair.key != fnorm { // no chance
			continue
		}

		dist := berghelroach.Dist(fnorm, pair.key, limit)

		if dist < limit {
			return pair.key, true
		}
	}
	return "", false
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
func clusterTest(failures []failure) (result failuresGroup) {
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
// @file_memoize("clustering inside each test", "memo_cluster_local.json") TODO
func clusterLocal(failuresByTest failuresGroup) (localClustering nestedFailuresGroups) {
	numFailures := 0
	start := time.Now()
	logInfo("Clustering failures for %d unique tests...", len(failuresByTest))

	// Look at tests with the most failures first.
	for n, pair := range failuresByTest.sortByNumberOfFailures() {
		numFailures += len(pair.failures)
		logInfo("%4d/%4d tests, %5d failures, %s", n+1, len(failuresByTest), len(pair.failures), pair.key)
		localClustering[pair.key] = clusterTest(pair.failures)
	}

	elapsed := time.Since(start)
	logInfo("Finished locally clustering %d unique tests (%d failures) in %s", len(localClustering), numFailures, elapsed.String())

	return localClustering
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
// TODO make sure type of previouslyClustered is correct and update documentation to match
// @file_memoize("clustering across tests", "memo_cluster_global.json") TODO
func clusterGlobal(newlyClustered nestedFailuresGroups, previouslyClustered []jsonCluster) nestedFailuresGroups {
	// The eventual global clusters
	var clusters nestedFailuresGroups

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

			// If a cluster exists for the given
			if _, ok := clusters[key]; ok {
				clusterCase = "EXISTING"

				// If there isn't yet a slice of failures that test, make a new one
				if _, ok := clusters[key][testName]; !ok {
					clusters[key][testName] = make([]failure, len(tests))
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
						clusters[other][testName] = make([]failure, len(tests))
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

	return clusters
}

// job represents a job name and a collection of associated build numbers.
type job struct {
	name         string
	buildNumbers []int
}

/*
testsGroupByJob takes a group of failures and a map of builds and returns the list of build numbers
that belong to each failure's job.

builds is a mapping from build paths to build objects.
*/
func testsGroupByJob(failures []failure, builds map[string]build) []job {
	// groups maps job names to sets of failures' build numbers.
	var groups map[string]sets.Int

	// For each failure, grab its build's job name. Map the job name to the failure's build number.
	for _, flr := range failures {
		// Try to grab the build from builds if it exists
		if bld, ok := builds[flr.build]; ok {
			// If the JSON build's "number" field was not null
			if bld.number != 0 {
				// Create the set if one doesn't exist for the given job
				if _, ok := groups[bld.job]; !ok {
					groups[bld.job] = make(sets.Int, 1)
				}
				groups[bld.job].Insert(bld.number)
			}
		}
	}

	// Sort groups in two stages.
	// First, sort each build number set in descending order.
	// Then, sort the jobs by the number of build numbers in each job's build number slice, descending.

	// First stage
	// sortedBuildNumbers is essentially groups, but with the build numbers sorted.
	sortedBuildNumbers := make(map[string][]int, len(groups))
	// Create the slice to hold the set elements, fill it, and sort it
	for jobName, buildNumberSet := range groups {
		// Initialize the int slice
		sortedBuildNumbers[jobName] = make([]int, len(buildNumberSet))

		// Fill it
		iter := 0
		for buildNumber := range buildNumberSet {
			sortedBuildNumbers[jobName][iter] = buildNumber
			iter++
		}

		// Sort it. Use > instead of < in less function to sort descending.
		sort.Slice(sortedBuildNumbers[jobName], func(i, j int) bool { return sortedBuildNumbers[jobName][i] > sortedBuildNumbers[jobName][j] })
	}

	// Second stage
	sortedGroups := make([]job, len(groups))

	// Fill sortedGroups
	for newJobName, newBuildNumbers := range sortedBuildNumbers {
		sortedGroups = append(sortedGroups, job{newJobName, newBuildNumbers})
	}
	// Sort it
	sort.Slice(sortedGroups, func(i, j int) bool {
		iGroupLen := len(sortedGroups[i].buildNumbers)
		jGroupLen := len(sortedGroups[j].buildNumbers)

		// If they're the same length, sort by job name alphabetically
		if iGroupLen == jGroupLen {
			return sortedGroups[i].name < sortedGroups[j].name
		}

		// Use > instead of < to sort descending.
		return iGroupLen > jGroupLen
	})

	return sortedGroups
}

var spanRE = regexp.MustCompile(`\w+|\W+`)

/*
commonSpans finds something similar to the longest common subsequence of xs, but much faster.

Returns a list of [matchlen_1, mismatchlen_2, matchlen_2, mismatchlen_2, ...], representing
sequences of the first element of the list that are present in all members.
*/
func commonSpans(xs []string) []int {
	var common sets.String
	commonModified := false // Flag to keep track of whether common has been modified at least once

	for _, x := range xs {
		xSplit := spanRE.FindAllString(x, -1)
		if !commonModified { // first iteration
			common.Insert(xSplit...)
			commonModified = true
		} else {
			common = common.Intersection(sets.NewString(xSplit...))
		}
	}

	var spans []int
	match := true
	spanLen := 0
	for _, x := range spanRE.FindAllString(xs[0], -1) {
		if common.Has(x) {
			if !match {
				match = true
				spans = append(spans, spanLen)
				spanLen = 0
			}
			spanLen += len(x)
		} else {
			if match {
				match = false
				spans = append(spans, spanLen)
				spanLen = 0
			}
			spanLen += len(x)
		}
	}

	if spanLen == 0 {
		spans = append(spans, spanLen)
	}

	return spans
}

// flattenedGlobalCluster is the key and value of a specific global cluster (as clusterText and
// sortedTests, respectively), plus the result of calling makeNgramCountsDigest on the key.
type flattenedGlobalCluster struct {
	clusterText       string
	ngramCountsDigest string
	sortedTests       []failuresGroupPair
}

// test represents a test name and a collection of associated jobs.
type test struct {
	name string
	jobs []job
}

/*
jsonCluster represents a global cluster as it will be written to the JSON.

	key:   the cluster text
	id:    the result of calling makeNgramCountsDigest() on key
	text:  a failure text from one of the cluster's failures
	spans: common spans between all of the cluster's failure texts
	tests: the build numbers that belong to the cluster's failures as per testGroupByJob()
	owner: the SIG that owns the cluster, determined by annotateOwners()
*/
type jsonCluster struct {
	key   string
	id    string
	text  string
	spans []int
	tests []test
	owner string
}

// clustersToDisplay transposes and sorts the flattened output of clusterGlobal.
// builds maps a build path to a build object.
func clustersToDisplay(clustered []flattenedGlobalCluster, builds map[string]build) []jsonCluster {
	jsonClusters := make([]jsonCluster, 0, len(clustered))

	for _, flattened := range clustered {
		key := flattened.clusterText
		keyId := flattened.ngramCountsDigest
		clusters := flattened.sortedTests

		// Determine the number of failures across all clusters
		numClusterFailures := 0
		for _, cluster := range clusters {
			numClusterFailures += len(cluster.failures)
		}

		if numClusterFailures > 1 {
			jCluster := jsonCluster{
				key:   key,
				id:    keyId,
				text:  clusters[0].failures[0].failureText,
				tests: make([]test, len(clusters)),
			}

			// Get all of the failure texts from all clusters
			clusterFailureTexts := make([]string, numClusterFailures)
			for _, cluster := range clusters {
				for _, flr := range cluster.failures {
					clusterFailureTexts = append(clusterFailureTexts, flr.failureText)
				}
			}
			jCluster.spans = commonSpans(clusterFailureTexts)

			// Fill out jCluster.tests
			for i, cluster := range clusters {
				jCluster.tests[i] = test{
					name: cluster.key,
					jobs: testsGroupByJob(cluster.failures, builds),
				}
			}

			jsonClusters = append(jsonClusters, jCluster)
		}
	}

	return jsonClusters
}

/*
columnarBuilds represents a collection of build objects where the i-th build's property p can be
found at p[i].

For example, the 4th (0-indexed) build's start time can be found in started[4], while its elapsed
time can be found in elapsed[4].
*/
type columnarBuilds struct {
	started      []int
	tests_failed []int
	elapsed      []int
	tests_run    []int
	result       []string
	executor     []string
	pr           []string
}

// currentIndex returns the index of the next build to be stored (and, by extension, the number of
// builds currently stored).
func (cb *columnarBuilds) currentIndex() int {
	return len(cb.started)
}

// insert adds a build into the columnarBuilds object.
func (cb *columnarBuilds) insert(b build) {
	cb.started = append(cb.started, b.started)
	cb.tests_failed = append(cb.tests_failed, b.testsFailed)
	cb.elapsed = append(cb.elapsed, b.elapsed)
	cb.tests_run = append(cb.tests_run, b.testsRun)
	cb.result = append(cb.result, b.result)
	cb.executor = append(cb.executor, b.executor)
	cb.pr = append(cb.pr, b.pr)
}

// newColumnarBuilds creates a columnarBuilds object with the correct number of columns. The number
// of columns is the same as the number of builds being converted to columnar form.
func newColumnarBuilds(columns int) columnarBuilds {
	// Start the length at 0 because columnarBuilds.currentIndex() relies on the length.
	return columnarBuilds{
		started:      make([]int, 0, columns),
		tests_failed: make([]int, 0, columns),
		elapsed:      make([]int, 0, columns),
		tests_run:    make([]int, 0, columns),
		result:       make([]string, 0, columns),
		executor:     make([]string, 0, columns),
		pr:           make([]string, 0, columns),
	}
}

/*
jobCollection represents a collection of jobs. It can either be a map[int]int (a mapping from
build numbers to indexes of builds in the columnar representation) or a []int (a condensed form
of the mapping for dense sequential mappings from builds to indexes; see buildsToColumns() comment).
This is necessary because the outputted JSON is unstructured, and has some fields that can be
either a map or a slice.
*/
type jobCollection interface{}

/*
columns representas a collection of builds in columnar form, plus the necessary maps to decode it.

jobs maps job names to their location in the columnar form.

cols is the collection of builds in columnar form.

jobPaths maps a job name to a build path, minus the last path segment.
*/
type columns struct {
	jobs      map[string]jobCollection
	cols      columnarBuilds
	job_paths map[string]string // TODO this probably needs to remain in snake case for JSON compatibility, revisit later
}

// buildsToColumns converts a map (from build paths to builds) into a columnar form. This compresses
// much better with gzip. See columnarBuilds for more information on the columnar form.
func buildsToColumns(builds map[string]build) columns {
	// jobs maps job names to either map[int]int or []int. See jobCollection.
	var jobs map[string]jobCollection
	// The builds in columnar form
	columnarBuilds := newColumnarBuilds(len(builds))
	// The function result
	result := columns{jobs, columnarBuilds, make(map[string]string, 0)}

	// Sort the builds before making them columnar
	sortedBuilds := make([]build, len(builds))
	// Fill the slice
	for _, bld := range builds {
		sortedBuilds = append(sortedBuilds, bld)
	}
	// Sort the slice
	sort.Slice(sortedBuilds, func(i, j int) bool {
		// Sort by job name, then by build number
		if sortedBuilds[i].job == sortedBuilds[j].job {
			return sortedBuilds[i].number < sortedBuilds[j].number
		}
		return sortedBuilds[i].job < sortedBuilds[j].job
	})

	// Add the builds to columnarBuilds
	for _, bld := range sortedBuilds {
		// If there was no build number when the build was retrieved from the JSON
		if bld.number == 0 {
			continue
		}

		// Get the index within cols's slices of the next inserted build
		index := columnarBuilds.currentIndex()

		// Add the build
		columnarBuilds.insert(bld)

		// job maps build numbers to their indexes in the columnar representation
		var job map[int]int
		if _, ok := jobs[bld.job]; !ok {
			jobs[bld.job] = make(map[int]int)
		}
		// We can safely assert map[int]int here because replacement of maps with slices only
		// happens later
		job = jobs[bld.job].(map[int]int)

		// Store the job path
		if len(job) == 0 {
			result.job_paths[bld.job] = bld.path[:strings.LastIndex(bld.path, "/")]
		}

		// Store the column number (index) so we know in which column to find which build
		job[bld.number] = index
	}

	// Sort build numbers and compress some data
	for jobName, indexes := range jobs {
		// Sort the build numbers
		sortedBuildNumbers := make([]int, 0, len(indexes.(map[int]int)))
		for key := range indexes.(map[int]int) {
			sortedBuildNumbers = append(sortedBuildNumbers, key)
		}
		sort.Ints(sortedBuildNumbers)

		base := indexes.(map[int]int)[sortedBuildNumbers[0]]
		count := len(sortedBuildNumbers)

		// Optimization: if we have a dense sequential mapping of builds=>indexes,
		// store only the first build number, the run length, and the first index number.
		allTrue := true
		for i, buildNumber := range sortedBuildNumbers {
			if indexes.(map[int]int)[buildNumber] != i+base {
				allTrue = false
				break
			}
		}
		if (sortedBuildNumbers[len(sortedBuildNumbers)-1] == sortedBuildNumbers[0]+count-1) && allTrue {
			jobs[jobName] = []int{sortedBuildNumbers[0], count, base}
			for _, n := range sortedBuildNumbers {
				if !(n <= sortedBuildNumbers[0]+len(sortedBuildNumbers)) {
					log.Panicf(jobName, n, jobs[jobName], len(sortedBuildNumbers), sortedBuildNumbers)
				}
			}
		}
	}
	return result
}

// jsonOutput represents the output as it will be written to the JSON.
type jsonOutput struct {
	clustered []jsonCluster
	builds    columns
}

// render accepts a map from build paths to builds, and the global clusters, and renders them in a
// format consumable by the web page.
func render(builds map[string]build, clustered nestedFailuresGroups) jsonOutput {
	clusteredSorted := clustered.sortByAggregateNumberOfFailures()

	flattenedClusters := make([]flattenedGlobalCluster, len(clusteredSorted))

	for i, pair := range clusteredSorted {
		k := pair.key
		clusters := pair.group

		flattenedClusters[i] = flattenedGlobalCluster{
			k,
			makeNgramCountsDigest(k),
			clusters.sortByNumberOfFailures(),
		}
	}

	return jsonOutput{
		clustersToDisplay(flattenedClusters, builds),
		buildsToColumns(builds),
	}
}

// sigLabelRE matches '[sig-x]', so long as x does not contain a closing bracket.
var sigLabelRE = regexp.MustCompile(`\[sig-([^]]*)\]`)

// annotateOwners assigns ownership to a cluster based on the share of hits in the last day.
//
// owners maps SIG names to collections of SIG-specific prefixes.
func annotateOwners(data jsonOutput, builds map[string]build, owners map[string][]string) error {
	// Dynamically create a regular expression based on the value of owners.
	/*
		namedOwnerREs is a collection of regular expressions of the form
		    (?P<signame>prefixA|prefixB|prefixC)
		where signame is the name of a SIG (such as 'sig-testing') with '-' replaced with '_' for
		compatibility with regex capture group name rules. There can be any number of prefixes
		following the capture group name.
	*/
	namedOwnerREs := make([]string, len(owners))
	for sig, prefixes := range owners {
		// prefixREs is a collection of non-empty prefixes with any special regex characters quoted
		prefixREs := make([]string, len(prefixes))
		for _, prefix := range prefixes {
			if prefix != "" {
				prefixREs = append(prefixREs, regexp.QuoteMeta(prefix))
			}
		}

		namedOwnerREs = append(namedOwnerREs,
			fmt.Sprintf("(?P<%s>%s)",
				strings.Replace(sig, "-", "_", -1), // Regex group names can't have '-', we'll substitute back later
				strings.Join(prefixREs, "|")))
	}

	// ownerRE is the final regex created from the values of namedOwnerREs, placed into a
	// non-capturing group
	ownerRE, err := regexp.Compile(fmt.Sprintf(`(?:%s)`, strings.Join(namedOwnerREs, "|")))
	if err != nil {
		return fmt.Errorf("Could not compile ownerRE from provided SIG names and prefixes: %s", err)
	}

	jobPaths := data.builds.job_paths
	yesterday := utils.Max(data.builds.cols.started...) - (60 * 60 * 24)

	// Determine the owner for each cluster
	for _, cluster := range data.clustered {
		// Maps owner names to hits (I think hits yesterday and hits today, respectively)
		ownerCounts := make(map[string][2]int)

		// For each test, determine the owner with the most hits
		for _, test := range cluster.tests {
			var owner string
			if submatches := sigLabelRE.FindStringSubmatch(test.name); submatches != nil {
				owner = submatches[1] // Get the first (and only) submatch of sigLabelRE
			} else {
				normalizedTestName := normalizeName(test.name)

				// Determine whether there were any named groups with matches for normalizedTestName,
				// and if so what the first named group with a match is
				namedGroupMatchExists := false
				firstMatchingGroupName := ""
				// Names of the named capturing groups, which are really the names of the owners
				groupNames := ownerRE.SubexpNames()
			outer:
				for _, submatches := range ownerRE.FindAllStringSubmatch(normalizedTestName, -1) {
					for i, submatch := range submatches {
						// If the group is named and there was a match
						if groupNames[i] != "" && submatch != "" {
							namedGroupMatchExists = true
							firstMatchingGroupName = groupNames[i]
							break outer
						}
					}
				}

				ownerIndex := ownerRE.FindStringIndex(normalizedTestName)

				if ownerIndex == nil || // If no match was found for the owner, or
					ownerIndex[0] != 0 || // the test name did not begin with the owner name, or
					!namedGroupMatchExists { // there were no named groups that matched
					continue
				}

				// Get the name of the first named group with a non-empty match, and assign it to owner
				owner = firstMatchingGroupName
			}

			owner = strings.Replace(owner, "_", "-", -1) // Substitute '_' back to '-'

			if _, ok := ownerCounts[owner]; !ok {
				ownerCounts[owner] = [2]int{0, 0}
			}
			counts := ownerCounts[owner]

			for _, job := range test.jobs {
				if strings.Contains(job.name, ":") { // non-standard CI
					continue
				}

				jobPath := jobPaths[job.name]
				for _, build := range job.buildNumbers {
					bucketKey := fmt.Sprintf("%s/%s", jobPath, build)
					if _, ok := builds[bucketKey]; !ok {
						continue
					} else if builds[bucketKey].started > yesterday {
						counts[0]++
					} else {
						counts[1]++
					}
				}
			}
		}

		if len(ownerCounts) != 0 {
			// Get the topOwner with the most hits yesterday, then most hits today, then name
			currentHasMoreHits := func(topOwner string, topOwnerCounts [2]int, currentOwner string, currentCounts [2]int) bool {
				if currentCounts[0] == topOwnerCounts[0] {
					if currentCounts[1] == topOwnerCounts[1] {
						// Which has the earlier name alphabetically
						return currentOwner < topOwner
					}
					return currentCounts[1] > topOwnerCounts[1]
				}
				return currentCounts[0] > topOwnerCounts[0]
			}

			var topOwner string
			topCounts := [2]int{}
			for currentOwner, currentCounts := range ownerCounts {
				if currentHasMoreHits(topOwner, topCounts, currentOwner, currentCounts) {
					topOwner = currentOwner
					topCounts = currentCounts
				}
			}
			cluster.owner = topOwner
		} else {
			cluster.owner = "testing"
		}
	}
	return nil
}

// renderSlice returns clusters whose owner field is the owner parameter or whose id field has a
// prefix of the prefix parameter, and the columnar form of the jobs belonging to those clusters.
// If parameters prefix and owner are both the empty string, the function will return empty objects.
func renderSlice(data jsonOutput, builds map[string]build, prefix string, owner string) ([]jsonCluster, columns) {
	clustered := make([]jsonCluster, 0)
	// Maps build paths to builds
	buildsOut := make(map[string]build, 0)
	var jobs sets.String

	// For each cluster whose owner field is the owner parameter, or whose id field has a prefix of
	// the prefix parameter, add its tests' jobs to the jobs set.
	for _, cluster := range data.clustered {
		if owner != "" && cluster.owner == owner {
			clustered = append(clustered, cluster)
		} else if prefix != "" && strings.HasPrefix(cluster.id, prefix) {
			clustered = append(clustered, cluster)
		} else {
			continue
		}

		for _, tst := range cluster.tests {
			for _, jb := range tst.jobs {
				jobs.Insert(jb.name)
			}
		}
	}

	// Add builds whose job is in jobs to buildsOut
	for _, bld := range builds {
		if jobs.Has(bld.job) {
			buildsOut[bld.path] = bld
		}
	}

	return clustered, buildsToColumns(buildsOut)
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

func main() {
	flags := parseFlags()

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
