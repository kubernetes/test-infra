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
	"k8s.io/test-infra/triage/utils"
)

// Dist takes two strings and returns the edit distance between them.
// It is the only name exported from this package. If limit is 0, the
// limit is len(a)+len(b).
func Dist(a string, b string, limit int) int {
	br := berghelRoach{pattern: a}

	if limit != 0 {
		return br.getDistance(b, limit)
	}
	return br.getDistance(b, len(a)+len(b))
}

// berghelRoach maintains state as the strings are compared.
type berghelRoach struct {
	// The "pattern" string against which others are compared.
	pattern string

	/*
		The current and two preceding sets of Ukkonen f(k,p) values for diagonals
		around the main, computed by the main loop of {@code getDistance}.  These
		arrays are retained between calls to save allocation costs.  (They are all
		initialized to a real array so that we can indiscriminately use length
		when ensuring/resizing.)
	*/
	currentLeft  []int
	currentRight []int
	lastLeft     []int
	lastRight    []int

	priorLeft  []int
	priorRight []int
}

// getDistance calculates the distance internally. This should not be called directly.
// Dist should be used instead.
func (br *berghelRoach) getDistance(target string, limit int) int {
	// Compute the main diagonal number.
	// The final result lies on this diagonal.
	main := len(br.pattern) - len(target)
	// Compute our initial distance candidate.
	// The result cannot be less than the difference in
	// string lengths, so we start there.
	distance := utils.Abs(main)
	if distance > limit {
		// More than we wanted.  Give up right away
		return distance
	}

	/*
	   In the main loop below, the current{Right,Left} arrays record results
	   from the current outer loop pass.  The last{Right,Left} and
	   prior{Right,Left} arrays hold the results from the preceding two passes.
	   At the end of the outer loop, we shift them around (reusing the prior
	   array as the current for the next round, to avoid reallocating).
	   The Right reflects higher-numbered diagonals, Left lower-numbered.
	   Fill in "prior" values for the first two passes through
	   the distance loop.  Note that we will execute only one side of
	   the main diagonal in these passes, so we only need
	   initialize one side of prior values.
	*/

	if main <= 0 {
		br.ensureCapacityRight(distance, false)
		for j := 0; j < distance; j++ {
			br.lastRight[j] = distance - j - 1 // Make diagonal -k start in row k
			br.priorRight[j] = -1
		}
	} else {
		br.ensureCapacityLeft(distance, false)
		for j := 0; j < distance; j++ {
			br.lastLeft[j] = -1 // Make diagonal +k start in row 0
			br.priorLeft[j] = -1
		}
	}

	// Keep track of even rounds.  Only those rounds consider new diagonals,
	// and thus only they require artificial "last" values below.
	even := true

	// MAIN LOOP: try each successive possible distance until one succeeds.
	for {
		// Before calling computeRow(main, distance), we need to fill in
		// missing cache elements.  See the high-level description above.
		// Higher-numbered diagonals
		var offDiagonal int = (distance - main) / 2
		br.ensureCapacityRight(offDiagonal, true)

		if even {
			// Higher diagonals start at row 0
			br.lastRight[offDiagonal] = -1
		}

		immediateRight := -1
		for offDiagonal > 0 {
			immediateRight = computeRow(
				(main + offDiagonal),
				(distance - offDiagonal),
				br.pattern,
				target,
				br.priorRight[offDiagonal-1],
				br.lastRight[offDiagonal],
				immediateRight)
			br.currentRight[offDiagonal] = immediateRight
			offDiagonal--
		}
		// Lower-numbered diagonals
		offDiagonal = (distance + main) / 2
		br.ensureCapacityLeft(offDiagonal, true)

		if even {
			// Lower diagonals, fictitious values for f(-x-1,x) = x
			br.lastLeft[offDiagonal] = (distance-main)/2 - 1
		}

		var immediateLeft int
		if even {
			immediateLeft = -1
		} else {
			immediateLeft = (distance - main) / 2
		}

		for offDiagonal > 0 {
			immediateLeft = computeRow(
				(main - offDiagonal),
				(distance - offDiagonal),
				br.pattern, target,
				immediateLeft,
				br.lastLeft[offDiagonal],
				br.priorLeft[offDiagonal-1])
			br.currentLeft[offDiagonal] = immediateLeft
			offDiagonal--
		}

		// We are done if the main diagonal has distance in the last row.
		mainRow := computeRow(main, distance, br.pattern, target,
			immediateLeft, br.lastLeft[0], immediateRight)

		if mainRow == len(target) {
			break
		}
		distance++
		if distance > limit || distance < 0 {
			break
		}

		// The [0] element goes to both sides.
		br.currentRight[0] = mainRow
		br.currentLeft[0] = mainRow

		// Rotate rows around for next round: current=>last=>prior (=>current)
		br.priorLeft, br.lastLeft, br.currentLeft = br.lastLeft, br.currentLeft, br.priorLeft
		br.priorRight, br.lastRight, br.currentRight = br.lastRight, br.currentRight, br.priorRight

		// Update evenness, too
		even = !even
	}

	return distance
}

// ensureCapacityLeft ensures that the Left arrays can be indexed through
// a certain index (inclusively), resizing (and copying) as necessary.
func (br *berghelRoach) ensureCapacityLeft(index int, cp bool) {
	if len(br.currentLeft) <= index {
		index++
		br.priorLeft = resize(br.priorLeft, index, cp)
		br.lastLeft = resize(br.lastLeft, index, cp)
		br.currentLeft = resize(br.currentLeft, index, false)
	}
}

// ensureCapacityLeft ensures that the Left arrays can be indexed through
// a certain index (inclusively), resizing (and copying) as necessary.
func (br *berghelRoach) ensureCapacityRight(index int, cp bool) {
	if len(br.currentRight) <= index {
		index++
		br.priorRight = resize(br.priorRight, index, cp)
		br.lastRight = resize(br.lastRight, index, cp)
		br.currentRight = resize(br.currentRight, index, false)
	}
}

// resize resizes an array, copying old contents if requested.
func resize(array []int, size int, cp bool) []int {
	if cp {
		new := make([]int, size)
		copy(new, array)
		return new
	}
	return make([]int, size)
}

/*
computeRow computes the highest row in which the distance p appears
in diagonal k of the edit distance computation for
strings a and b.  The diagonal number is
represented by the difference in the indices for the two strings;
it can range from

	-b.length()

through

	a.length()

More precisely, this computes the highest value x such that

	p = edit-distance(a[0:(x+k)), b[0:x)).

This is the "f" function described by Ukkonen.

The caller must assure that the absolute value of k <= p,
the only values for which this is well-defined.

The implementation depends on the cached results of prior
computeRow calls for diagonals k-1, k, and k+1 for distance p-1.
These must be supplied in knownLeft, knownAbove,
and knownRight, respectively.

k: diagonal number

p: edit distance

a: one string to be compared

b: other string to be compared

knownLeft: value of `computeRow(k-1, p-1, ...)`

knownAbove: value of `computeRow(k, p-1, ...)`

knownRight: value of `computeRow(k+1, p-1, ...)`
*/
func computeRow(k int, p int, a string, b string,
	knownLeft int, knownAbove int, knownRight int) int {
	if !(utils.Abs(k) <= p) {
		panic("Abs(k) is not less than or equal to p")
	}
	if !(p >= 0) {
		panic("p is not greater than or equal to 0")
	}
	/*
	   Compute our starting point using the recurrence.
	   That is, find the first row where the desired edit distance
	   appears in our diagonal.  This is at least one past
	   the highest row for
	*/
	var t int
	if p == 0 {
		t = 0
	} else {
		/*
		   We look at the adjacent diagonals for the next lower edit distance.
		   We can start in the next row after the prior result from
		   our own diagonal (the "substitute" case), or the next diagonal
		   ("delete"), but only the same row as the prior result from
		   the prior diagonal ("insert").
		*/
		t = utils.Max(utils.Max(knownAbove, knownRight)+1, knownLeft)
	}
	// Look down our diagonal for matches to find the maximum
	// row with edit-distance p.
	tmax := utils.Min(len(b), len(a)-k)
	for t < tmax && b[t] == a[t+k] {
		t++
	}

	return t
}
