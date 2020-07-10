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

// TODO Fix function and variable case to use camelCase
// TODO Convert complex map types to type aliases
// TODO Revise comments to better match arguments

// Package summarize groups test failures together by finding edit distances between their failure messages,
// and emits JSON for rendering in a browser.
package summarize

import (
	"crypto/sha1"
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"regexp"
	"sort"
	"strings"
	"time"

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

var longOutputLen = 10000
var truncatedSep = "\n...[truncated]...\n"
var maxClusterTextLen = longOutputLen + len(truncatedSep)

// Will be used across make_ngram_counts() calls
var ngram_counts map[string][]int

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
}

// failure represents a specific instance of a test failure.
type failure struct {
	started     int
	build       string
	name        string
	failureText string
}

// sfsmSort (string failure-slice map sort) provides the keys and values of the
// provided map as a slice of key-value pairs, sorted in descending order based
// on number of failures.
func sfsmSort(m map[string][]failure) (result []struct {
	string
	failures []failure
}) {
	// Fill the slice
	for key, val := range m {
		result = append(result, struct {
			string
			failures []failure
		}{key, val})
	}

	// Sort the slice. Use > instead of < in less function so largest values
	// (i.e. clusters with the most failures) are first.
	sort.Slice(result, func(i, j int) bool { return len(result[i].failures) > len(result[j].failures) })

	return result
}

// sfsmKeys (string failure-slice map keys) provides the keys of a map as a slice.
func sfsmKeys(m map[string][]failure) []string {
	result := make([]string, len(m))

	iter := 0
	for key := range m {
		result[iter] = key
		iter++
	}

	return result
}

// ssfsmSort (string string failure-slice map sort) provides the keys and values of the
// provided map as a slice of key-value pairs, sorted in descending order based on the
// aggregate number of failures across all of a test's clusters.
func ssfsmSort(m map[string]map[string][]failure) (result []struct {
	string
	m map[string][]failure
}) {
	// Fill the slice
	for key, val := range m {
		result = append(result, struct {
			string
			m map[string][]failure
		}{key, val})
	}

	// Sort the slice. Use > instead of < in less function so largest values
	// (i.e. largest number of failures across all clusters) are first.
	sort.Slice(result, func(i, j int) bool {
		testIFailures := 0
		testJFailures := 0

		for _, failureSlice := range result[i].m {
			testIFailures += len(failureSlice)
		}

		for _, failureSlice := range result[j].m {
			testJFailures += len(failureSlice)
		}

		return testIFailures > testJFailures
	})

	return result
}

// ssfsmKeys (string string failure-slice map keys) provides the keys of a map as a slice.
func ssfsmKeys(m map[string]map[string][]failure) []string {
	result := make([]string, len(m))

	iter := 0
	for key := range m {
		result[iter] = key
		iter++
	}

	return result
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
	repl := func(match string) string {
		if _, ok := matches[match]; !ok {
			matches[match] = fmt.Sprintf("UNIQ%d", len(matches)+1)
		}
		return matches[match]
	}

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

	s = flakeReasonOrdinalRE.ReplaceAllStringFunc(s, repl)

	// for long strings, remove repeated lines!
	if len(s) > longOutputLen {
		s = utils.RemoveDuplicateLines(s)
	}

	if len(s) > longOutputLen { // ridiculously long test output
		s = s[:longOutputLen/2] + truncatedSep + s[len(s)-longOutputLen/2:]
	}

	return s
}

// Given a test name, remove [...] and {...}. Matches code in testgrid and kubernetes/hack/update_owners.py.
func normalize_name(name string) string {
	name = regexp.MustCompile(`\[.*?\]|{.*?\}`).ReplaceAllLiteralString(name, "")
	name = regexp.MustCompile(`\s+`).ReplaceAllLiteralString(name, " ")
	return strings.TrimSpace(name)
}

/*
Convert a string into a histogram of frequencies for different byte combinations.
This can be used as a heuristic to estimate edit distance between two strings in
constant time.

Instead of counting each ngram individually, they are hashed into buckets.
This makes the output count size constant.
*/
func make_ngram_counts(s string) []int {
	size := 64
	if _, ok := ngram_counts[s]; ok {
		counts := make([]int, size)
		for x := 0; x < len(s)-3; x++ {
			counts[int(crc32.Checksum([]byte(s[x:x+4]), crc32.MakeTable(0))&uint32(size-1))]++
		}
		ngram_counts[s] = counts // memoize
	}
	return ngram_counts[s]
}

/*
Compute a heuristic lower-bound edit distance using ngram counts.

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
func ngram_editdist(a string, b string) int {
	counts_a := make_ngram_counts(a)
	counts_b := make_ngram_counts(b)

	shortestCounts := utils.Min(len(counts_a), len(counts_b))
	result := 0
	for i := 0; i < shortestCounts; i++ {
		result += utils.Abs(counts_a[i] - counts_b[i])
	}

	return result / 4
}

// Returns a hashed version of the ngram counts.
func make_ngram_counts_digest(s string) string {
	ngramResults := make_ngram_counts(s)
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
func find_match(fnorm string, candidates []string) (result string, found bool) {
	type distancePair struct {
		distResult int
		key        string
	}

	distancePairs := make([]distancePair, len(candidates))

	iter := 0
	for _, candidate := range candidates {
		distancePairs[iter] = distancePair{ngram_editdist(fnorm, candidate), candidate}
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
func cluster_test(failures []failure) (result map[string][]failure) {
	start := time.Now()

	for _, flr := range failures {
		fNorm := normalize(flr.failureText)

		// If this string is already in the result list, store it
		if _, ok := result[fNorm]; ok {
			result[fNorm] = append(result[fNorm], flr)
		} else {
			// Otherwise, check if a match can be found for the normalized string
			other, found := find_match(fNorm, sfsmKeys(result))
			if found {
				result[other] = append(result[other], flr)
			} else {
				result[fNorm] = []failure{flr}
			}
		}

		// Bail if the clustering takes too long
		if time.Since(start).Seconds() > 60 {
			log.Print("bailing early, taking too long!")
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
func cluster_local(failuresByTest map[string][]failure) (localClustering map[string]map[string][]failure) {
	numFailures := 0
	start := time.Now()
	log.Printf("Clustering failures for %d unique tests...", len(failuresByTest))

	// Look at tests with the most failures first.
	for n, pair := range sfsmSort(failuresByTest) {
		numFailures += len(pair.failures)
		log.Printf("%4d/%4d tests, %5d failures, %s", n+1, len(failuresByTest), len(pair.failures), pair.string)
		localClustering[pair.string] = cluster_test(pair.failures)
	}

	elapsed := time.Since(start)
	log.Printf("Finished locally clustering %d unique tests (%d failures) in %s", len(localClustering), numFailures, elapsed.String())

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
func cluster_global(newlyClustered map[string]map[string][]failure, previouslyClustered []map[string]string) (globalClustering map[string]map[string][]failure) {
	numFailures := 0

	log.Printf("Combining clustered failures for %d unique tests...", len(newlyClustered))
	start := time.Now()

	if previouslyClustered != nil {
		// Seed clusters using output from the previous run
		n := 0
		for _, cluster := range previouslyClustered {
			key := cluster["key"]
			if key != normalize(key) {
				log.Print(key)
				log.Print(normalize(key))
				n++
				continue
			}

			globalClustering[key] = make(map[string][]failure)
		}

		log.Printf("Seeding with %d previous clusters", len(globalClustering))

		if n != 0 {
			log.Printf("!!! %d clusters lost from different normalization! !!!", n)
		}
	}

	// Look at tests with the most failures over all clusters first
	for n, outerPair := range ssfsmSort(newlyClustered) {
		testName := outerPair.string
		testClusters := outerPair.m

		log.Printf("%4d/%4d tests, %4d clusters, %s", n+1, len(newlyClustered), len(testClusters), testName)
		testStart := time.Now()

		// Look at clusters with the most failures first
		for m, innerPair := range sfsmSort(testClusters) {
			key := innerPair.string // The cluster text
			tests := innerPair.failures

			clusterStart := time.Now()
			fTextLen := len(key)
			numClusters := len(testClusters)
			numTests := len(tests)
			var clusterCase string

			log.Printf("  %4d/%4d clusters, %5d chars failure text, %5d failures ...", m, numClusters, fTextLen, numTests)
			numFailures += numTests

			// If a cluster exists for the given
			if _, ok := globalClustering[key]; ok {
				clusterCase = "EXISTING"

				// If there isn't yet a slice of failures that test, make a new one
				if _, ok := globalClustering[key][testName]; !ok {
					globalClustering[key][testName] = make([]failure, len(tests))
				}

				// Copy the contents into the test's failure slice
				globalClustering[key][testName] = append(globalClustering[key][testName], tests...)
			} else if time.Since(testStart).Seconds() > 30 && fTextLen > maxClusterTextLen/2 && numTests == 1 {
				// if we've taken longer than 30 seconds for this test, bail on pathological / low value cases
				clusterCase = "BAILED"
			} else {
				other, found := find_match(key, ssfsmKeys(globalClustering))
				if found {
					clusterCase = "OTHER"

					// If there isn't yet a slice of failures that test, make a new one
					if _, ok := globalClustering[other][testName]; !ok {
						globalClustering[other][testName] = make([]failure, len(tests))
					}

					// Copy the contents into the test's failure slice
					globalClustering[other][testName] = append(globalClustering[other][testName], tests...)
				} else {
					clusterCase = "NEW"
					globalClustering[key] = map[string][]failure{testName: tests}
				}
			}

			clusterDuration := int(time.Since(clusterStart).Seconds())
			log.Printf("  %4d/%4d clusters, %5d chars failure text, %5d failures, cluster:%s in %d sec, test: %s",
				m, numClusters, fTextLen, numTests, clusterCase, clusterDuration, testName)
	}
	}

	// If we seeded clusters using the previous run's keys, some of those
	// clusters may have disappeared. Remove the resulting empty entries.
	for key, val := range globalClustering {
		if len(val) == 0 {
			delete(globalClustering, key)
		}
	}

	elapsed := time.Since(start)
	log.Printf("Finished clustering %d unique tests (%d failures) into %d clusters in %s",
		len(newlyClustered), numFailures, len(globalClustering), elapsed.String())

	return globalClustering
}
