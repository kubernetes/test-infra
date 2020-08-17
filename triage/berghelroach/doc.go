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

/*
Package berghelraoch is a modification of the original Berghel-Roach edit
distance (based on prior work by Ukkonen) described in
  ACM Transactions on Information Systems, Vol. 14, No. 1,
  January 1996, pages 94-106.

I observed that only O(d) prior computations are required
to compute edit distance.  Rather than keeping all prior
f(k,p) results in a matrix, we keep only the two "outer edges"
in the triangular computation pattern that will be used in
subsequent rounds.  We cannot reconstruct the edit path,
but many applications do not require that; for them, this
modification uses less space (and empirically, slightly
less time).

First, some history behind the algorithm necessary to understand
Berghel-Roach and our modification...

The traditional algorithm for edit distance uses dynamic programming,
building a matrix of distances for substrings:
D[i,j] holds the distance for string1[0..i]=>string2[0..j].
The matrix is initially populated with the trivial values
D[0,j]=j and D[i,0]=i; and then expanded with the rule:

   D[i,j] = min( D[i-1,j]+1,       // insertion
                 D[i,j-1]+1,       // deletion
                 (D[i-1,j-1]
                  + (string1[i]==string2[j])
                     ? 0           // match
                     : 1           // substitution ) )

Ukkonen observed that each diagonal of the matrix must increase
by either 0 or 1 from row to row.  If D[i,j] = p, then the
matching rule requires that D[i+x,j+x] = p for all x
where string1[i..i+x) matches string2[j..j+j+x). Ukkonen
defined a function f(k,p) as the highest row number in which p
appears on the k-th diagonal (those D[i,j] where k=(i-j), noting
that k may be negative).  The final result of the edit
distance is the D[n,m] cell, on the (n-m) diagonal; it is
the value of p for which f(n-m, p) = m.  The function f can
also be computed dynamically, according to a simple recursion:

   f(k,p) {
     contains_p = max(f(k-1,p-1), f(k,p-1)+1, f(k+1,p-1)+1)
     while (string1[contains_p] == string2[contains_p + k])
       contains_p++;
     return contains_p;
   }

The max() expression finds a row where the k-th diagonal must
contain p by virtue of an edit from the prior, same, or following
diagonal (corresponding to an insert, substitute, or delete);
we need not consider more distant diagonals because row-to-row
and column-to-column changes are at most +/- 1.

The original Ukkonen algorithm computed f(k,p) roughly as
follows:

   for (p = 0; ; p++) {
     compute f(k,p) for all valid k
     if (f(n-m, p) == m) return p;
   }


Berghel and Roach observed that many values of f(k,p) are
computed unnecessarily, and reorganized the computation into
a just-in-time sequence.  In each iteration, we are primarily
interested in the terminating value f(main,p), where main=(n-m)
is the main diagonal.  To compute that we need f(x,p-1) for
three values of x: main-1, main, and main+1.  Those depend on
values for p-2, and so forth.  We will already have computed
f(main,p-1) in the prior round, and thus f(main-1,p-2) and
f(main+1,p-2), and so forth.  The only new values we need to compute
are on the edges: f(main-i,p-i) and f(main+i,p-i).  Noting that
f(k,p) is only meaningful when abs(k) is no greater than p,
one of the Berghel-Roach reviewers noted that we can compute
the bounds for i:

   (main+i &le p-i) implies (i &le; (p-main)/2)

(where main+i is limited on the positive side) and similarly

   (-(main-i) &le p-i) implies (i &le; (p+main)/2).

(where main-i is limited on the negative side).

This reduces the computation sequence to

  for (i = (p-main)/2; i > 0; i--) compute f(main+i,p-i);
  for (i = (p+main)/2; i > 0; i--) compute f(main-i,p-i);
  if (f(main, p) == m) return p;


The original Berghel-Roach algorithm recorded prior values
of f(k,p) in a matrix, using O(distance^2) space, enabling
reconstruction of the edit path, but if all we want is the
edit *distance*, we only need to keep O(distance) prior computations.

The requisite prior k-1, k, and k+1 values are conveniently
computed in the current round and the two preceding it.
For example, on the higher-diagonal side, we compute:

   current[i] = f(main+i, p-i)

We keep the two prior rounds of results, where p was one and two
smaller.  So, from the preceidng round

   last[i] = f(main+i, (p-1)-i)

 and from the prior round, but one position back:

   prior[i-1] = f(main+(i-1), (p-2)-(i-1))

In the current round, one iteration earlier:

   current[i+1] = f(main+(i+1), p-(i+1))

Note that the distance in all of these evaluates to p-i-1,
and the diagonals are (main+i) and its neighbors... just
what we need.  The lower-diagonal side behaves similarly.

We need to materialize values that are not computed in prior
rounds, for either of two reasons:

- Initially, we have no prior rounds, so we need to fill
all of the "last" and "prior" values for use in the
first round.  The first round uses only on one side
of the main diagonal or the other.

- In every other round, we compute one more diagonal than before.

In all of these cases, the missing f(k,p) values are for abs(k) > p,
where a real value of f(k,p) is undefined.  [The original Berghel-Roach
algorithm prefills its F matrix with these values, but we fill
them as we go, as needed.]  We define

   f(-p-1,p) = p, so that we start diagonal -p with row p,
   f(p+1,p) = -1, so that we start diagonal p with row 0.

(We also allow f(p+2,p)=f(-p-2,p)=-1, causing those values to
have no effect in the starting row computation.]

We only expand the set of diagonals visited every other round,
when (p-main) or (p+main) is even.  We keep track of even/oddness
to save some arithmetic.  The first round is always even, as p=abs(main).
Note that we rename the "f" function to "computeRow" to be Googley.
*/
package berghelroach
