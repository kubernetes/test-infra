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

# pylint: disable=invalid-name,missing-docstring

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

def dist(a, b, limit=None):
    return BerghelRoach(a).getDistance(b, limit or len(a) + len(b))

# This is a modification of the original Berghel-Roach edit
# distance (based on prior work by Ukkonen) described in
#   ACM Transactions on Information Systems, Vol. 14, No. 1,
#   January 1996, pages 94-106.
#
# I observed that only O(d) prior computations are required
# to compute edit distance.  Rather than keeping all prior
# f(k,p) results in a matrix, we keep only the two "outer edges"
# in the triangular computation pattern that will be used in
# subsequent rounds.  We cannot reconstruct the edit path,
# but many applications do not require that; for them, this
# modification uses less space (and empirically, slightly
# less time).
#
# First, some history behind the algorithm necessary to understand
# Berghel-Roach and our modification...
#
# The traditional algorithm for edit distance uses dynamic programming,
# building a matrix of distances for substrings:
# D[i,j] holds the distance for string1[0..i]=>string2[0..j].
# The matrix is initially populated with the trivial values
# D[0,j]=j and D[i,0]=i; and then expanded with the rule:
# <pre>
#    D[i,j] = min( D[i-1,j]+1,       // insertion
#                  D[i,j-1]+1,       // deletion
#                  (D[i-1,j-1]
#                   + (string1[i]==string2[j])
#                      ? 0           // match
#                      : 1           // substitution ) )
# </pre>
#
# Ukkonen observed that each diagonal of the matrix must increase
# by either 0 or 1 from row to row.  If D[i,j] = p, then the
# matching rule requires that D[i+x,j+x] = p for all x
# where string1[i..i+x) matches string2[j..j+j+x). Ukkonen
# defined a function f(k,p) as the highest row number in which p
# appears on the k-th diagonal (those D[i,j] where k=(i-j), noting
# that k may be negative).  The final result of the edit
# distance is the D[n,m] cell, on the (n-m) diagonal; it is
# the value of p for which f(n-m, p) = m.  The function f can
# also be computed dynamically, according to a simple recursion:
# <pre>
#    f(k,p) {
#      contains_p = max(f(k-1,p-1), f(k,p-1)+1, f(k+1,p-1)+1)
#      while (string1[contains_p] == string2[contains_p + k])
#        contains_p++;
#      return contains_p;
#    }
# </pre>
# The max() expression finds a row where the k-th diagonal must
# contain p by virtue of an edit from the prior, same, or following
# diagonal (corresponding to an insert, substitute, or delete);
# we need not consider more distant diagonals because row-to-row
# and column-to-column changes are at most +/- 1.
#
# The original Ukkonen algorithm computed f(k,p) roughly as
# follows:
# <pre>
#    for (p = 0; ; p++) {
#      compute f(k,p) for all valid k
#      if (f(n-m, p) == m) return p;
#    }
# </pre>
#
# Berghel and Roach observed that many values of f(k,p) are
# computed unnecessarily, and reorganized the computation into
# a just-in-time sequence.  In each iteration, we are primarily
# interested in the terminating value f(main,p), where main=(n-m)
# is the main diagonal.  To compute that we need f(x,p-1) for
# three values of x: main-1, main, and main+1.  Those depend on
# values for p-2, and so forth.  We will already have computed
# f(main,p-1) in the prior round, and thus f(main-1,p-2) and
# f(main+1,p-2), and so forth.  The only new values we need to compute
# are on the edges: f(main-i,p-i) and f(main+i,p-i).  Noting that
# f(k,p) is only meaningful when abs(k) is no greater than p,
# one of the Berghel-Roach reviewers noted that we can compute
# the bounds for i:
# <pre>
#    (main+i &le p-i) implies (i &le; (p-main)/2)
# </pre>
# (where main+i is limited on the positive side) and similarly
# <pre>
#    (-(main-i) &le p-i) implies (i &le; (p+main)/2).
# </pre>
# (where main-i is limited on the negative side).
#
# This reduces the computation sequence to
# <pre>
#   for (i = (p-main)/2; i > 0; i--) compute f(main+i,p-i);
#   for (i = (p+main)/2; i > 0; i--) compute f(main-i,p-i);
#   if (f(main, p) == m) return p;
# </pre>
#
# The original Berghel-Roach algorithm recorded prior values
# of f(k,p) in a matrix, using O(distance^2) space, enabling
# reconstruction of the edit path, but if all we want is the
# edit *distance*, we only need to keep O(distance) prior computations.
#
# The requisite prior k-1, k, and k+1 values are conveniently
# computed in the current round and the two preceding it.
# For example, on the higher-diagonal side, we compute:
# <pre>
#    current[i] = f(main+i, p-i)
# </pre>
# We keep the two prior rounds of results, where p was one and two
# smaller.  So, from the preceidng round
# <pre>
#    last[i] = f(main+i, (p-1)-i)
# </pre>
#  and from the prior round, but one position back:
# <pre>
#    prior[i-1] = f(main+(i-1), (p-2)-(i-1))
# </pre>
# In the current round, one iteration earlier:
# <pre>
#    current[i+1] = f(main+(i+1), p-(i+1))
# </pre>
# Note that the distance in all of these evaluates to p-i-1,
# and the diagonals are (main+i) and its neighbors... just
# what we need.  The lower-diagonal side behaves similarly.
#
# We need to materialize values that are not computed in prior
# rounds, for either of two reasons: <ul>
#    <li> Initially, we have no prior rounds, so we need to fill
#     all of the "last" and "prior" values for use in the
#     first round.  The first round uses only on one side
#     of the main diagonal or the other.
#    <li> In every other round, we compute one more diagonal than before.
# </ul>
# In all of these cases, the missing f(k,p) values are for abs(k) > p,
# where a real value of f(k,p) is undefined.  [The original Berghel-Roach
# algorithm prefills its F matrix with these values, but we fill
# them as we go, as needed.]  We define
# <pre>
#    f(-p-1,p) = p, so that we start diagonal -p with row p,
#    f(p+1,p) = -1, so that we start diagonal p with row 0.
# </pre>
# (We also allow f(p+2,p)=f(-p-2,p)=-1, causing those values to
# have no effect in the starting row computation.]
#
# We only expand the set of diagonals visited every other round,
# when (p-main) or (p+main) is even.  We keep track of even/oddness
# to save some arithmetic.  The first round is always even, as p=abs(main).
# Note that we rename the "f" function to "computeRow" to be Googley.

class BerghelRoach:
    def __init__(self, pattern):
        # The "pattern" string against which others are compared.
        self.pattern = pattern

        # The current and two preceding sets of Ukkonen f(k,p) values for diagonals
        # around the main, computed by the main loop of {@code getDistance}.  These
        # arrays are retained between calls to save allocation costs.  (They are all
        # initialized to a real array so that we can indiscriminately use length
        # when ensuring/resizing.)
        self.currentLeft = []
        self.currentRight = []
        self.lastLeft = []
        self.lastRight = []

        self.priorLeft = []
        self.priorRight = []

    def getDistance(self, target, limit):
        # pylint: disable=too-many-branches

        # Compute the main diagonal number.
        # The final result lies on this diagonal.
        main = len(self.pattern) - len(target)
        # Compute our initial distance candidate.
        # The result cannot be less than the difference in
        # string lengths, so we start there.
        distance = abs(main)
        if distance > limit:
            # More than we wanted.  Give up right away
            return distance

        # In the main loop below, the current{Right,Left} arrays record results
        # from the current outer loop pass.  The last{Right,Left} and
        # prior{Right,Left} arrays hold the results from the preceding two passes.
        # At the end of the outer loop, we shift them around (reusing the prior
        # array as the current for the next round, to avoid reallocating).
        # The Right reflects higher-numbered diagonals, Left lower-numbered.
        # Fill in "prior" values for the first two passes through
        # the distance loop.  Note that we will execute only one side of
        # the main diagonal in these passes, so we only need
        # initialize one side of prior values.

        if main <= 0:
            self.ensureCapacityRight(distance, False)
            for j in range(distance):
                self.lastRight[j] = distance - j - 1 # Make diagonal -k start in row k
                self.priorRight[j] = -1
        else:
            self.ensureCapacityLeft(distance, False)
            for j in range(distance):
                self.lastLeft[j] = -1 # Make diagonal +k start in row 0
                self.priorLeft[j] = -1

        # Keep track of even rounds.  Only those rounds consider new diagonals,
        # and thus only they require artificial "last" values below.
        even = True

        # MAIN LOOP: try each successive possible distance until one succeeds.
        while True:
            # Before calling computeRow(main, distance), we need to fill in
            # missing cache elements.  See the high-level description above.
            # Higher-numbered diagonals
            offDiagonal = (distance - main) // 2
            self.ensureCapacityRight(offDiagonal, True)

            if even:
                # Higher diagonals start at row 0
                self.lastRight[offDiagonal] = -1

            immediateRight = -1
            while offDiagonal > 0:
                immediateRight = computeRow(
                    (main + offDiagonal),
                    (distance - offDiagonal),
                    self.pattern,
                    target,
                    self.priorRight[offDiagonal-1],
                    self.lastRight[offDiagonal],
                    immediateRight)
                self.currentRight[offDiagonal] = immediateRight
                offDiagonal -= 1
            # Lower-numbered diagonals
            offDiagonal = (distance + main) // 2
            self.ensureCapacityLeft(offDiagonal, True)

            if even:
                # Lower diagonals, fictitious values for f(-x-1,x) = x
                self.lastLeft[offDiagonal] = (distance-main)//2 - 1

            if even:
                immediateLeft = -1
            else:
                immediateLeft = (distance - main) // 2

            while offDiagonal > 0:
                immediateLeft = computeRow(
                    (main - offDiagonal),
                    (distance - offDiagonal),
                    self.pattern, target,
                    immediateLeft,
                    self.lastLeft[offDiagonal],
                    self.priorLeft[offDiagonal-1])
                self.currentLeft[offDiagonal] = immediateLeft
                offDiagonal -= 1

            # We are done if the main diagonal has distance in the last row.
            mainRow = computeRow(main, distance, self.pattern, target,
                                 immediateLeft, self.lastLeft[0], immediateRight)

            if mainRow == len(target):
                break
            distance += 1
            if distance > limit or distance < 0:
                break

            # The [0] element goes to both sides.
            self.currentRight[0] = mainRow
            self.currentLeft[0] = mainRow

            # Rotate rows around for next round: current=>last=>prior (=>current)
            tmp = self.priorLeft
            self.priorLeft = self.lastLeft
            self.lastLeft = self.currentLeft
            self.currentLeft = tmp

            tmp = self.priorRight
            self.priorRight = self.lastRight
            self.lastRight = self.currentRight
            self.currentRight = tmp

            # Update evenness, too
            even = not even

        return distance

    def ensureCapacityLeft(self, index, cp):
        # Ensures that the Left arrays can be indexed through {@code index},
        # inclusively, resizing (and copying) as necessary.
        if len(self.currentLeft) <= index:
            index += 1
            self.priorLeft = resize(self.priorLeft, index, cp)
            self.lastLeft = resize(self.lastLeft, index, cp)
            self.currentLeft = resize(self.currentLeft, index, False)

    def ensureCapacityRight(self, index, cp):
        # Ensures that the Right arrays can be indexed through {@code index},
        # inclusively, resizing (and copying) as necessary.
        if len(self.currentRight) <= index:
            index += 1
            self.priorRight = resize(self.priorRight, index, cp)
            self.lastRight = resize(self.lastRight, index, cp)
            self.currentRight = resize(self.currentRight, index, False)


# Resize an array, copying old contents if requested
def resize(array, size, cp):
    if cp:
        return array + [0] * (size - len(array))
    return [0] * size

# Computes the highest row in which the distance {@code p} appears
# in diagonal {@code k} of the edit distance computation for
# strings {@code a} and {@code b}.  The diagonal number is
# represented by the difference in the indices for the two strings;
# it can range from {@code -b.length()} through {@code a.length()}.
#
# More precisely, this computes the highest value x such that
# <pre>
#     p = edit-distance(a[0:(x+k)), b[0:x)).
# </pre>
#
# This is the "f" function described by Ukkonen.
#
# The caller must assure that abs(k) &le; p, the only values for
# which this is well-defined.
#
# The implementation depends on the cached results of prior
# computeRow calls for diagonals k-1, k, and k+1 for distance p-1.
# These must be supplied in {@code knownLeft}, {@code knownAbove},
# and {@code knownRight}, respectively.
# @param k diagonal number
# @param p edit distance
# @param a one string to be compared
# @param b other string to be compared
# @param knownLeft value of {@code computeRow(k-1, p-1, ...)}
# @param knownAbove value of {@code computeRow(k, p-1, ...)}
# @param knownRight value of {@code computeRow(k+1, p-1, ...)}
def computeRow(k, p, a, b,
               knownLeft, knownAbove, knownRight):
    assert abs(k) <= p
    assert p >= 0
    # Compute our starting point using the recurrence.
    # That is, find the first row where the desired edit distance
    # appears in our diagonal.  This is at least one past
    # the highest row for
    if p == 0:
        t = 0
    else:
        # We look at the adjacent diagonals for the next lower edit distance.
        # We can start in the next row after the prior result from
        # our own diagonal (the "substitute" case), or the next diagonal
        # ("delete"), but only the same row as the prior result from
        # the prior diagonal ("insert").
        t = max(max(knownAbove, knownRight)+1, knownLeft)
    # Look down our diagonal for matches to find the maximum
    # row with edit-distance p.
    tmax = min(len(b), len(a)-k)
    while t < tmax and b[t] == a[t+k]:
        t += 1

    return t
