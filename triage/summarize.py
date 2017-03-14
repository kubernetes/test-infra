#!/usr/bin/env python2

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


import functools
import hashlib
import json
import os
import re
import sys
import zlib

import berghelroach

editdist = berghelroach.dist

flakeReasonDateRE = re.compile(
    r'[A-Z][a-z]{2}, \d+ \w+ 2\d{3} [\d.-: ]*([-+]\d+)?|'
    r'\w{3}\s+\d{1,2} \d+:\d+:\d+(\.\d+)?|(\d{4}-\d\d-\d\d.|.\d{4} )\d\d:\d\d:\d\d(.\d+)?')
# Find random noisy strings that should be renumberedplaced with renumbered strings, for more similar messages.
flakeReasonOrdinalRE = re.compile(
        r'0x[0-9a-fA-F]+' # hex constants
        r'|\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}(:\d+)?' # IPs + optional port
        r'|[0-9a-fA-F]{8}-\S{4}-\S{4}-\S{4}-\S{12}(-\d+)?' # UUIDs + trailing digits
        r'|[0-9a-f]{12,32}' # hex garbage
)


def normalize(s):
    """
    Given a traceback or error message from a text, reduce excess entropy to make
    clustering easier.

    This includes:
    - blanking dates and timestamps
    - renumbering unique information like
        - pointer addresses
        - UUIDs
        - IP addresses
    - sorting randomly ordered map[] strings.
    """

    # blank out dates
    s = flakeReasonDateRE.sub('TIME', s)

    # do alpha conversion-- rename random garbage strings (hex pointer values, node names, etc)
    # into 'UNIQ1', 'UNIQ2', etc.
    matches = {}
    def repl(m):
        s = m.group(0)
        if s not in matches:
            matches[s] = 'UNIQ%d' % (len(matches) + 1)
        return matches[s]

    if 'map[' in s:
        # Go's maps are in a random order. Try to sort them to reduce diffs.
        s = re.sub(r'map\[([^][]*)\]',
            lambda m: 'map[%s]' % ' '.join(sorted(m.group(1).split()))
            , s)

    return flakeReasonOrdinalRE.sub(repl, s)


def make_ngram_counts(s, ngram_counts={}):
    """
    Convert a string into a histogram of frequencies for different byte combinations.
    This can be used as a heuristic to estimate edit distance between two strings in
    constant time.

    Instead of counting each ngram individually, they are hashed into buckets.
    This makes the output count size constant.
    """
    size = 64
    if s not in ngram_counts:
        counts = [0] * size
        for x in xrange(len(s)-3):
            counts[zlib.crc32(s[x:x+4].encode('utf8')) & (size - 1)] += 1
        ngram_counts[s] = counts  # memoize
    return ngram_counts[s]


def ngram_editdist(a, b):
    """
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
    """
    counts_a = make_ngram_counts(a)
    counts_b = make_ngram_counts(b)
    return sum(abs(x-y) for x, y in zip(counts_a, counts_b))/4


def make_ngram_counts_digest(s):
    return hashlib.sha1(str(make_ngram_counts(s))).hexdigest()[:20]


def file_memoize(description, name):
    def inner(func):
        @functools.wraps(func)
        def wrapper(*args, **kwargs):
            if os.path.exists(name):
                data = json.load(open(name))
                print 'done (cached)', description
                return data
            else:
                data = func(*args, **kwargs)
                json.dump(data, open(name, 'w'))
                print 'done', description
                return data
        wrapper.__wrapped__ = func
        return wrapper
    return inner


@file_memoize('loading failed tests', 'failed.json')
def load_failures(builds_file, tests_file):
    builds = {}
    for build in json.load(open(builds_file)):
        if not build['started'] or not build['number']:
            continue
        for attr in ('started', 'tests_failed', 'number', 'tests_run'):
            build[attr] = int(build[attr])
        build['elapsed'] = int(float(build['elapsed']))
        if 'pr-logs' in build['path']:
            build['pr'] = build['path'].split('/')[-3]
        builds[build['path']] = build

    failed_tests = {}
    for test in json.load(open(tests_file)):
        failed_tests.setdefault(test['name'], []).append(test)
    for tests in failed_tests.itervalues():
        tests.sort(key=lambda t: t['build'])

    return builds, failed_tests


def find_match(fnorm, clusters):
    for ngram_dist, other in sorted((ngram_editdist(fnorm, x), x) for x in clusters):
        # allow up to 10% differences
        limit = int((len(fnorm)+len(other))/2.0 * 0.10)

        if ngram_dist > limit:
            continue

        if limit <= 1 and other != fnorm:  # no chance
            continue

        if len(fnorm) < 1000 and len(other) < 1000:
            dist = editdist(fnorm, other, limit)
        else:
            dist = ngram_dist

        if dist < limit:
            return other


def cluster_test(tests):
    """
    Compute failure clusters given a list of failures for one test.

    Args:
        tests: list of failed test dictionaries, with 'failure_text' keys
    Returns:
        {failure_text: [failure_in_cluster_1, failure_in_cluster_2, ...]}
    """
    clusters = {}

    for test in tests:
        ftext = test['failure_text']
        fnorm = normalize(ftext)
        if fnorm in clusters:
            clusters[fnorm].append(test)
        else:
            other = find_match(fnorm, clusters)
            if other:
                clusters[other].append(test)
            else:
                clusters[fnorm] = [test]
    return clusters


@file_memoize('clustering inside each test', 'failed_clusters_local.json')
def cluster_local(builds, failed_tests):
    """Cluster together the failures for each test. """
    clustered = {}
    for test_name, tests in sorted(failed_tests.iteritems(), key=lambda x:len(x[1]), reverse=True):
        print len(tests), test_name
        clustered[test_name] = cluster_test(tests)
    return clustered


@file_memoize('clustering across tests', 'failed_clusters_global.json')
def cluster_global(clustered):
    """Combine together clustered failures for each test.

    This is done hierarchically for efficiency-- each test's failures are likely to be similar,
    reducing the number of clusters that need to be paired up at this stage.

    Args:
        {test_name: {failure_text: [failure_1, failure_2, ...], ...}, ...}
    Returns:
        {failure_text: [(test_name, [failure_1, failure_2, ...]), ...], ...}
    """
    clusters = {}

    for n, (test_name, cluster) in enumerate(
            sorted(clustered.iteritems(), key=lambda (k, v): sum(len(x) for x in v.itervalues()), reverse=True),
            1):
        print '%d/%d %d %s' % (n, len(clustered), len(cluster), test_name)
        for key, tests in sorted(cluster.iteritems(), key=lambda x: len(x[1]), reverse=True):
            test_cluster_tuple = (test_name, tests)
            if key in clusters:
                clusters[key].setdefault(test_name, []).extend(tests)
            else:
                other = find_match(key, clusters)
                if other:
                    clusters[other].setdefault(test_name, []).extend(tests)
                else:
                    clusters[key] = {test_name: list(tests)}

    return clusters


def tests_group_by_job(tests, builds):
    """Turn a list of test failures into {job: [buildnumber, ...], ...}"""
    groups = {}
    for test in tests:
        try:
            build = builds[test['build']]
        except KeyError:
            continue
        if 'number' in build:
            groups.setdefault(build['job'], []).append(build['number'])
    return sorted(groups.iteritems(), key=lambda (k, v): (-len(v), k))


def clusters_to_display(clustered, builds):
    """Transpose and sort the output of cluster_global."""
    for key, key_id, clusters in clustered:
        test_names = set()
        for test_name, tests in clusters:
            if test_name in test_names:
                print 'WTF', test_name
            test_names.add(test_name)


    return [
        [key, key_id, clusters[0][1][0]['failure_text'],
            [
                [test_name, tests_group_by_job(tests, builds)]
                for test_name, tests in sorted(clusters, key=lambda (n, t): (-len(t), n))
            ]
        ]
        for key, key_id, clusters in clustered
    ]


def builds_to_columns(builds):
    """Convert a list of build dictionaries into a columnar form.

    This compresses much better with gzip."""

    jobs = {}

    cols = {v: [] for v in 'started tests_failed elapsed tests_run result executor pr'.split()}
    out = {'jobs': jobs, 'cols': cols, 'job_paths': {}}
    for build in sorted(builds.itervalues(), key=lambda b: (b['job'], b['number'])):
        if 'number' not in build:
            continue
        index = len(cols['started'])
        for key, entries in cols.iteritems():
            entries.append(build.get(key))
        job = jobs.setdefault(build['job'], {})
        if not job:
            out['job_paths'][build['job']] = build['path'][:build['path'].rindex('/')]
        job[build['number']] = index

    for k, indexes in jobs.items():
        numbers = sorted(indexes)
        base = indexes[numbers[0]]
        count = len(numbers)

        # optimization: if we have a dense sequential mapping of builds=>indexes,
        # store only the first build number, the run length, and the first index number.
        if numbers[-1] == numbers[0] + count - 1 and \
                all(indexes[k] == n + base for n, k in enumerate(numbers)):
            jobs[k] = [numbers[0], count, base]
            for n in numbers:
                assert n <= numbers[0] + len(numbers), (k, n, jobs[k], len(numbers), numbers)
    return out


def render(builds, failed_tests, clustered):
    clustered_sorted = sorted(clustered.iteritems(), key=lambda (k, v): (-sum(len(ts) for ts in v.itervalues()), k))
    clustered_tuples = [(k,
                         make_ngram_counts_digest(k),
                         sorted(clusters.items(), key=lambda (n, t): (-len(t), n)))
                         for k, clusters in clustered_sorted]

    return {'clustered': clusters_to_display(clustered_tuples, builds), 'builds': builds_to_columns(builds)}


def main(builds_file, tests_file):
    builds, failed_tests = load_failures(builds_file, tests_file)
    clustered_local = cluster_local(builds, failed_tests)
    clustered = cluster_global(clustered_local)
    print '%d clusters' % len(clustered)

    json.dump(render(builds, failed_tests, clustered), open('failure_data.json', 'w'),
              sort_keys=True)


if __name__ == '__main__':
    main(sys.argv[1], sys.argv[2])
