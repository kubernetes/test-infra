#!/usr/bin/env python3

# Copyright 2017 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Ported from Java com.google.gwt.dev.util.editdistance, which is:
# Copyright 2010 Google Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License"); you may not
# use this file except in compliance with the License. You may obtain a copy of
# the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
# WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
# License for the specific language governing permissions and limitations under
# the License.

# pylint: disable=missing-docstring,invalid-name

import random
import unittest

import berghelroach


# A very large string for testing.
MAGNA = (
    "We have granted to God, and by this our present Charter have "
    "confirmed, for Us and our Heirs for ever, that the Church of "
    "England shall be free, and shall have all her whole Rights and "
    "Liberties inviolable.  We have granted also, and given to all "
    "the Freemen of our Realm, for Us and our Heirs for ever, these "
    "Liberties under-written, to have and to hold to them and their "
    "Heirs, of Us and our Heirs for ever."
)

# A small set of words for testing, including at least some of
# each of these: empty, very short, more than 32/64 character,
# punctuation, non-ASCII characters
words = [
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
    "for pure edit distance."
]

wordDistances = {}

# Computes Levenshtein edit distance using the far-from-optimal
# dynamic programming technique.  This is here purely to verify
# the results of better algorithms.
def dynamicProgrammingLevenshtein(s1, s2):
    lastRow = list(range(len(s1) + 1))
    for j, s2_item in enumerate(s2):
        thisRow = [0] * len(lastRow)
        thisRow[0] = j + 1
        for i in range(1, len(thisRow)):
            thisRow[i] = min(lastRow[i] + 1,
                             thisRow[i - 1] + 1,
                             lastRow[i - 1] + int(s2_item != s1[i-1]))
        lastRow = thisRow
    return lastRow[-1]

for wordA in words:
    for wordB in words:
        wordDistances[wordA, wordB] = dynamicProgrammingLevenshtein(wordA, wordB)


class AbstractLevenshteinTestCase:
    # pylint: disable=no-member

    # Tests a Levenshtein engine against the DP-based computation
    # for a bunch of string pairs.
    def testLevenshteinOnWords(self):
        for a in words:
            for b in words:
                ed = self.getInstance(a)
                self.specificAlgorithmVerify(ed, a, b, wordDistances[a, b])

    # Tests Levenshtein edit distance on a longer pattern
    def testLongerPattern(self):
        self.genericLevenshteinVerify("abcdefghijklmnopqrstuvwxyz",
                                      "abcefghijklMnopqrStuvwxyz..",
                                      5)  # dMS..

    # Tests Levenshtein edit distance on a very short pattern
    def testShortPattern(self):
        self.genericLevenshteinVerify("short", "shirt", 1)

    # Verifies zero-length behavior
    def testZeroLengthPattern(self):
        nonEmpty = "target"
        self.genericLevenshteinVerify("", nonEmpty, len(nonEmpty))
        self.genericLevenshteinVerify(nonEmpty, "", len(nonEmpty))

    # Tests the default Levenshtein engine on a pair of strings
    def genericLevenshteinVerify(self, s1, s2, expectedResult):
        self.specificAlgorithmVerify(self.getInstance(s1), s1, s2, expectedResult)

    # Performs some edits on a string in a StringBuilder.
    # @param b string to be modified
    # @param alphabet some characters guaranteed not to be in the original
    # @param replaces how many single-character replacements to try
    # @param inserts how many characters to insert
    # @return the number of edits actually performed, the new string
    @staticmethod
    def performSomeEdits(b, alphabet, replaces, inserts):
        r = random.Random(768614336404564651)
        edits = 0
        b = list(b)

        for _ in range(inserts):
            b.insert(r.randint(0, len(b) - 1), r.choice(alphabet))
            edits += 1
        for _ in range(replaces):
            where = r.randint(0, len(b) - 1)
            if b[where] not in alphabet:
                b[where] = r.choice(alphabet)
                edits += 1
        return edits, ''.join(b)

    # Generates a long random alphabetic string,
    # suitable for use with verifySomeEdits (using digits for the alphabet).
    # @param size desired string length
    # @param seed random number generator seed
    # @return random alphabetic string of the requested length
    @staticmethod
    def generateRandomString(size, seed):
        alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

        # Create a (repeatable) random string from the alphabet
        rand = random.Random(seed)
        return ''.join(rand.choice(alphabet) for _ in range(size))

    # Exercises an edit distance engine across a wide range of limit values
    def genericVerification(self, ed, s1, s2, expectedResult):
        if len(s1) < 500:
             # For small strings, try every limit
            maxDiff = max(len(s1), len(s2)) + 2
            for k in range(maxDiff):
                self.verifyResult(s1, s2, expectedResult, k, ed.getDistance(s2, k))
        else:
            # For big strings, try a sampling of limits:
            #   0 to 3,
            #   another 4 on either side of the expected result
            #   s2 length
            for k in range(4):
                self.verifyResult(s1, s2, expectedResult, k, ed.getDistance(s2, k))
            for k in range(max(4, expectedResult - 4), expectedResult + 4):
                self.verifyResult(s1, s2, expectedResult, k, ed.getDistance(s2, k))
            self.verifyResult(s1, s2, expectedResult, len(s2),
                              ed.getDistance(s2, len(s2)))

        # Always try near MAX_VALUE
        self.assertEqual(ed.getDistance(s2, 2**63 - 1), expectedResult)
        self.assertEqual(ed.getDistance(s2, 2**63), expectedResult)

    # Tests a specific engine on a pair of strings
    def specificAlgorithmVerify(self, ed, s1, s2, expectedResult):
        self.genericVerification(ed, s1, s2, expectedResult)

        # Try again with the same instance
        self.genericVerification(ed, s1, s2, expectedResult)

    # Verifies the distance between an original string and some
    # number of simple edits on it.  The distance is assumed to
    # be unit-cost Levenshtein distance.
    def verifySomeEdits(self, original, replaces, inserts):
        edits, modified = self.performSomeEdits(original, "0123456789", replaces, inserts)

        self.specificAlgorithmVerify(self.getInstance(original), original, modified, edits)

        self.specificAlgorithmVerify(self.getInstance(modified), modified, original, edits)

        # we don't have duplicate() in Python, so...
        # self.specificAlgorithmVerify(self.getInstance(modified).duplicate(),
        #                              modified, original, edits)

    # Verifies a single edit distance result.
    # If the expected distance is within limit, result must b
    # be correct; otherwise, result must be over limit.
    #
    # @param s1 one string compared
    # @param s2 other string compared
    # @param expectedResult correct distance from s1 to s2
    # @param k limit applied to computation
    # @param d distance computed
    def verifyResult(self, s1, s2, expectedResult, k, d):
        if k >= expectedResult:
            self.assertEqual(
                expectedResult, d,
                'Distance from %r to %r should be %d (within limit=%d) but was %d' %
                (s1, s2, expectedResult, k, d))
        else:
            self.assertTrue(
                d > k,
                'Distance from %r to %r should be %d (exceeding limit=%d) but was %d' %
                (s1, s2, expectedResult, k, d))


# Test cases for the ModifiedBerghelRoachEditDistance class.
#
# The bulk of the test is provided by the superclass, for
# which we provide GeneralEditDistance instances.
#
# Since Berghel-Roach is superior for longer strings with moderately
# low edit distances, we try a few of those specifically.
# This Modified form uses less space, and can handle yet larger ones.

class BerghelRoachTest(unittest.TestCase, AbstractLevenshteinTestCase):
    @staticmethod
    def getInstance(s):
        return berghelroach.BerghelRoach(s)

    def testHugeEdit(self):
        SIZE = 10000
        SEED = 1

        self.verifySomeEdits(self.generateRandomString(SIZE, SEED), (SIZE // 50), (SIZE // 50))

    def testHugeString(self):
         # An even larger size is feasible, but the test would no longer
         # qualify as "small".
        SIZE = 20000
        SEED = 1

        self.verifySomeEdits(self.generateRandomString(SIZE, SEED), 30, 25)

    def testLongString(self):
        self.verifySomeEdits(MAGNA, 8, 10)

    def testLongStringMoreEdits(self):
        self.verifySomeEdits(MAGNA, 40, 30)


if __name__ == '__main__':
    unittest.main()
