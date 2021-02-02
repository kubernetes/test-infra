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
Ported from Java com.google.gwt.dev.util.editdistance, which is:
Copyright 2010 Google Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not
use this file except in compliance with the License. You may obtain a copy of
the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
License for the specific language governing permissions and limitations under
the License.
*/

package berghelroach

import (
	"math/rand"
	"strings"
	"testing"

	"k8s.io/test-infra/triage/utils"
)

/*
Since Berghel-Roach is superior for longer strings with moderately
low edit distances, we try a few of those specifically.
This Modified form uses less space, and can handle yet larger ones.
*/

func TestHugeEdit(t *testing.T) {
	const size int = 10000
	const seed int64 = 1

	verifySomeEdits(t, generateRandomString(size, seed), (size / 50), (size / 50))
}

func TestHugeString(t *testing.T) {
	// An even larger size is feasible, but the test would no longer
	// qualify as "small".
	const size int = 20000
	const seed int64 = 1

	verifySomeEdits(t, generateRandomString(size, seed), 30, 25)
}

func TestLongString(t *testing.T) {
	// A very large string for testing.
	veryLargeString := "We have granted to God, and by this our present Charter have " +
		"confirmed, for Us and our Heirs for ever, that the Church of " +
		"England shall be free, and shall have all her whole Rights and " +
		"Liberties inviolable.  We have granted also, and given to all " +
		"the Freemen of our Realm, for Us and our Heirs for ever, these " +
		"Liberties under-written, to have and to hold to them and their " +
		"Heirs, of Us and our Heirs for ever."

	t.Run("Small number of edits", func(t *testing.T) {
		verifySomeEdits(t, veryLargeString, 8, 10)
	})

	t.Run("Larger number of edits", func(t *testing.T) {
		verifySomeEdits(t, veryLargeString, 40, 30)
	})

}

/*
Abstract Levenshtein engine test cases
*/

// TestLevenshteinOnWords tests a Levenshtein engine against the dynamic programming-based computation
// for a bunch of string pairs.
func TestLevenshteinOnWords(t *testing.T) {
	// First, some setup

	// A small set of words for testing, including at least some of
	// each of these: empty, very short, more than 32/64 character,
	// punctuation, non-ASCII characters
	words := []string{
		"", "a", "b", "c", "ab", "ace",
		"fortressing", "inadequately", "prank", "authored",
		"fortresing", "inadeqautely", "prang", "awthered",
		"cruller's", "fanatic", "Laplace", "recollections",
		"Kevlar", "underpays", "jalape\u00f1o", "ch\u00e2telaine",
		"kevlar", "overpaid", "jalapeno", "chatelaine",
		"A survey of algorithms for running text search by Navarro appeared",
		"in ACM Computing Surveys 33#1: http://portal.acm.org/citation.cfm?...",
		"Another algorithm (Four Russians) that Navarro",
		"long patterns and high limits was not evaluated for inclusion here.",
		"long patterns and low limits were evaluated for inclusion here.",
		"Filtering algorithms also improve running search",
		"for pure edit distance.",
	}

	// To be used as a key for the wordDistances map
	type wordPair struct {
		wordA string
		wordb string
	}
	// The cartesian product of words with itself.
	wordDistances := make(map[wordPair]int, len(words)*len(words))

	// Prepare the map of word distances
	for _, wordA := range words {
		for _, wordB := range words {
			wordDistances[wordPair{wordA, wordB}] = dynamicProgrammingLevenshtein(wordA, wordB)
		}
	}

	// Now for the actual test
	for _, wordA := range words {
		for _, wordB := range words {
			ed := berghelRoach{pattern: wordA}
			specificAlgorithmVerify(t, ed, wordA, wordB, wordDistances[wordPair{wordA, wordB}])
		}
	}
}

// TestLongerPattern tests Levenshtein edit distance on a longer pattern
func TestLongerPattern(t *testing.T) {
	specificAlgorithmVerify(t, berghelRoach{pattern: "abcdefghijklmnopqrstuvwxyz"}, "abcdefghijklmnopqrstuvwxyz",
		"abcefghijklMnopqrStuvwxyz..",
		5) // dMS..
}

// TestShortPattern tests Levenshtein edit distance on a very short pattern
func TestShortPattern(t *testing.T) {
	specificAlgorithmVerify(t, berghelRoach{pattern: "short"}, "short", "shirt", 1)
}

// TestZeroLengthPattern verifies zero-length behavior
func TestZeroLengthPattern(t *testing.T) {
	nonEmpty := "target"
	specificAlgorithmVerify(t, berghelRoach{pattern: ""}, "", nonEmpty, len(nonEmpty))
	specificAlgorithmVerify(t, berghelRoach{pattern: nonEmpty}, nonEmpty, "", len(nonEmpty))
}

// Utility functions

// dynamicProgrammingLevenshtein computes Levenshtein edit distance
// using the far-from-optimal dynamic programming technique.  This is
// here purely to verify the results of better algorithms.
func dynamicProgrammingLevenshtein(s1 string, s2 string) int {
	lastRowSize := len(s1) + 1
	lastRow := make([]int, lastRowSize)
	for i := 0; i < lastRowSize; i++ {
		lastRow[i] = i
	}

	for j := 0; j < len(s2); j++ {
		thisRow := make([]int, len(lastRow))
		thisRow[0] = j + 1
		for i := 1; i < len(thisRow); i++ {
			thisRow[i] = utils.Min(lastRow[i]+1,
				thisRow[i-1]+1,
				lastRow[i-1]+utils.BtoI(s2[j] != s1[i-1]))
		}
		lastRow = thisRow
	}
	return lastRow[len(lastRow)-1]
}

/*
performEdits performs some edits on a string.

original: string to be modified

alphabet: some characters guaranteed not to be in the original

replacements: how many single-character replacements to try

insertions: how many characters to insert
*/
func performEdits(original string, alphabet string, replacements int, insertions int) (int, string) {
	rand.Seed(768614336404564651)
	edits := 0

	// Convert the original string to a slice for easier manipulation.
	originalSlice := []byte(original)

	for i := 0; i < insertions; i++ {
		utils.ByteSliceInsert(&originalSlice, alphabet[rand.Intn(len(alphabet))], rand.Intn(len(originalSlice)))
		edits++
	}

	for i := 0; i < replacements; i++ {
		where := rand.Intn(len(originalSlice))
		letterInAlphabet := false
		for i := 0; i < len(alphabet); i++ {
			if originalSlice[where] == alphabet[i] {
				letterInAlphabet = true
				break
			}
		}

		if !letterInAlphabet {
			originalSlice[where] = alphabet[rand.Intn(len(alphabet))]
			edits++
		}
	}

	return edits, string(originalSlice)
}

/*
generateRandomString Generates a long random alphabetic string,
suitable for use with verifySomeEdits (using digits for the alphabet).

size: desired string length

seed: random number generator seed
*/
func generateRandomString(size int, seed int64) string {
	alphabet := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

	// Create a (repeatable) random string from the alphabet
	rand.Seed(seed)
	var builder strings.Builder

	for i := 0; i < size; i++ {
		builder.WriteByte(alphabet[rand.Intn(len(alphabet))])
	}

	return builder.String()
}

/*
verifyResult verifies a single edit distance result.
If the expected distance is within limit, result must b
be correct; otherwise, result must be over limit.

s1: one string compared

s2: other string compared

expectedResult: correct distance from s1 to s2

k: limit applied to computation

d: distance computed
*/
func verifyResult(t *testing.T, s1 string, s2 string, expectedResult int, k int, d int) {
	// Check if the strings are long. If so, truncate them.
	maxLength := 50
	if len(s1) > maxLength {
		s1 = s1[:maxLength] + "[truncated]"
	}
	if len(s2) > maxLength {
		s2 = s2[:maxLength] + "[truncated]"
	}

	// Use %#v to add quotation marks around the text
	if k >= expectedResult {
		if expectedResult != d {
			t.Errorf("Distance from %#v to %#v should be %d (within limit=%d) but was %d", s1, s2, expectedResult, k, d)
		}
	} else {
		if !(d > k) {
			t.Errorf("Distance from %#v to %#v should be %d (exceeding limit=%d) but was %d", s1, s2, expectedResult, k, d)
		}
	}
}

// genericVerification exercises an edit distance engine across a wide range of limit values
func genericVerification(t *testing.T, ed berghelRoach, s1 string, s2 string, expectedResult int) {
	if len(s1) < 500 {
		// For small strings, try every limit
		maxDiff := utils.Max(len(s1), len(s2)) + 2
		for k := 0; k < maxDiff; k++ {
			verifyResult(t, s1, s2, expectedResult, k, ed.getDistance(s2, k))
		}
	} else {
		// For big strings, try a sampling of limits:
		//   0 to 3,
		//   another 4 on either side of the expected result
		//   s2 length
		for k := 0; k < 4; k++ {
			verifyResult(t, s1, s2, expectedResult, k, ed.getDistance(s2, k))
		}
		for k := utils.Max(4, expectedResult-4); k < expectedResult+4; k++ {
			verifyResult(t, s1, s2, expectedResult, k, ed.getDistance(s2, k))
		}
		verifyResult(t, s1, s2, expectedResult, len(s2),
			ed.getDistance(s2, len(s2)))
	}

	// Always try near MAX_VALUE
	if ed.getDistance(s2, int(^uint(0)>>1)-1) != expectedResult { // int(^uint(0) >> 1) is the largest possible int
		t.Errorf("getDistance failed with maximum uint value minus 1, got %d", expectedResult)
	}
	if ed.getDistance(s2, int(^uint(0)>>1)) != expectedResult { // int(^uint(0) >> 1) is the largest possible int
		t.Errorf("getDistance failed with maximum uint value, got %d", expectedResult)
	}
}

// specificAlgorithmVerify tests a specific engine on a pair of strings
func specificAlgorithmVerify(t *testing.T, ed berghelRoach, s1 string, s2 string, expectedResult int) {
	genericVerification(t, ed, s1, s2, expectedResult)

	// Try again with the same instance
	genericVerification(t, ed, s1, s2, expectedResult)
}

// verifySomeEdits verifies the distance between an original string and some
// number of simple edits on it.  The distance is assumed to
// be unit-cost Levenshtein distance.
func verifySomeEdits(t *testing.T, original string, replacements int, insertions int) {
	edits, modified := performEdits(original, "0123456789", replacements, insertions)

	specificAlgorithmVerify(t, berghelRoach{pattern: original}, original, modified, edits)

	specificAlgorithmVerify(t, berghelRoach{pattern: modified}, modified, original, edits)
}
