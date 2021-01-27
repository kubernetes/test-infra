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
Contains functions that handle, normalize, and otherwise modify text.
*/

package summarize

import (
	"crypto/sha1"
	"fmt"
	"hash/crc32"
	"io"
	"regexp"
	"sort"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
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
	matches := make(map[string]string)

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

	// In Python, given an array [x, y, z], calling str() on the array will output
	// "[x, y, z]". This will try to replicate that behavior.
	// Represent the ngramResults
	ngramResultsAsString := strings.Replace(fmt.Sprintf("%v", ngramResults), " ", ", ", -1)

	// Generate the hash
	hash := sha1.New()
	_, err := io.WriteString(hash, ngramResultsAsString)
	if err != nil {
		klog.Fatalf("Error writing ngram results string to sha1 hash: %s", err)
	}

	return fmt.Sprintf("%x", hash.Sum(nil))[:20]
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

var spanRE = regexp.MustCompile(`\w+|\W+`)

/*
commonSpans finds something similar to the longest common subsequence of xs, but much faster.

Returns a list of [matchlen_1, mismatchlen_2, matchlen_2, mismatchlen_2, ...], representing
sequences of the first element of the list that are present in all members.
*/
func commonSpans(xs []string) []int {
	common := make(sets.String)
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

	spans := make([]int, 0)
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

	if spanLen != 0 {
		spans = append(spans, spanLen)
	}

	return spans
}

var memoizedNgramCounts = make(map[string][]int) // Will be used across makeNgramCounts() calls
var memoizedNgramCountsMutex sync.RWMutex        // makeNgramCounts is eventually depended on by some parallelized functions

/*
makeNgramCounts converts a string into a histogram of frequencies for different byte combinations.
This can be used as a heuristic to estimate edit distance between two strings in
constant time.

Instead of counting each ngram individually, they are hashed into buckets.
This makes the output count size constant.
*/
func makeNgramCounts(s string) []int {
	size := 64

	memoizedNgramCountsMutex.RLock() // Lock the map for reading
	if _, ok := memoizedNgramCounts[s]; !ok {
		memoizedNgramCountsMutex.RUnlock() // Unlock while calculating

		counts := make([]int, size)
		for x := 0; x < len(s)-3; x++ {
			counts[int(crc32.Checksum([]byte(s[x:x+4]), crc32.IEEETable)&uint32(size-1))]++
		}

		memoizedNgramCountsMutex.Lock() // Lock the map for writing
		memoizedNgramCounts[s] = counts // memoize
		memoizedNgramCountsMutex.Unlock()

		return counts
	} else {
		result := memoizedNgramCounts[s]
		memoizedNgramCountsMutex.RUnlock()
		return result
	}
}
