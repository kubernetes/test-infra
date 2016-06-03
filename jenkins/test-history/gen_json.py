#!/usr/bin/env python

# Copyright 2016 The Kubernetes Authors All rights reserved.
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

"""Generates a JSON file containing test history for the last day.

Writes the JSON out to tests.json.
"""

from __future__ import print_function

import argparse
import json
import logging
import os
import re
import random
import signal
import sys
import time
import urllib2
from xml.etree import ElementTree

import multiprocessing
import requests


MAX_AGE = 60 * 60 * 24  # 1 day


class GCSClient(object):

    def __init__(self, jobs_dir, metadata=None):
        self.jobs_dir = jobs_dir
        self.metadata = metadata or {}
        self.session = requests.Session()

    def request(self, path, params, as_json=True):
        """GETs a JSON resource from GCS, with retries on failure.

        Retries are based on guidance from
        cloud.google.com/storage/docs/gsutil/addlhelp/RetryHandlingStrategy
        """
        url = 'https://www.googleapis.com/storage/v1/b/%s' % path
        for retry in xrange(23):
            try:
                resp = self.session.get(url, params=params, stream=False)
                if 400 <= resp.status_code < 500 and resp.status_code != 429:
                    return None
                resp.raise_for_status()
                if as_json:
                    return resp.json()
                else:
                    return resp.content
            except requests.exceptions.RequestException:
                logging.exception('request failed %s', url)
            time.sleep(random.random() * min(60, 2 ** retry))

    def parse_uri(self, path):
        if not path.startswith('gs://'):
            raise ValueError("Bad GCS path")
        bucket, prefix = path[5:].split('/', 1)
        return bucket, prefix

    def get(self, path, as_json=False):
        """Get an object from GCS."""
        bucket, path = self.parse_uri(path)
        return self.request('%s/o/%s' % (bucket, urllib2.quote(path, '')),
                           {'alt': 'media'}, as_json=as_json)

    def ls(self, path, dirs=True, files=True):
        """Lists objects under a path on gcs."""
        bucket, path = self.parse_uri(path)
        params = {'delimiter': '/', 'prefix': path, 'fields': 'nextPageToken'}
        if dirs:
            params['fields'] += ',prefixes'
        if files:
            params['fields'] += ',items(name)'
        while True:
            resp = self.request('%s/o' % bucket, params)
            for prefix in resp.get('prefixes', []):
                yield 'gs://%s/%s' % (bucket, prefix)
            for item in resp.get('items', []):
                yield 'gs://%s/%s' % (bucket, item['name'])
            if 'nextPageToken' not in resp:
                break
            params['pageToken'] = resp['nextPageToken']

    def ls_dirs(self, path):
        return self.ls(path, dirs=True, files=False)

    def _ls_junit_paths(self, job, build):
        """Lists the paths of JUnit XML files for a build."""
        url = '%s%s/%s/artifacts/' % (self.jobs_dir, job, build)
        for path in self.ls(url):
            if re.match(r'.*/junit.*\.xml$', path):
                yield path

    def _get_tests_from_junit(self, path):
        """Generates test data out of the provided JUnit path.

        Returns None if there's an issue parsing the XML.
        Yields name, time, failed, skipped for each test.
        """
        data = self.get(path)

        try:
            root = ElementTree.fromstring(data)
        except ElementTree.ParseError:
            logging.exception("unable to parse %s" % path)
            return

        for child in root:
            name = child.attrib['name']
            ctime = float(child.attrib['time'])
            failed = False
            skipped = False
            for param in child:
                if param.tag == 'skipped':
                    skipped = True
                elif param.tag == 'failure':
                    failed = True
            yield name, ctime, failed, skipped

    def _get_jobs(self):
        """Generates all jobs in the bucket."""
        for job_path in self.ls_dirs(self.jobs_dir):
            yield os.path.basename(os.path.dirname(job_path))

    def _get_builds(self, job):
        if self.metadata.get('sequential', True):
            try:
                latest_build = int(self.get('%s%s/latest-build.txt'
                                            % (self.jobs_dir, job)))
            except (ValueError, TypeError):
                pass
            else:
                return (str(n) for n in xrange(latest_build, 0, -1))
        # Invalid latest-build or bucket is using timestamps
        build_paths = self.ls_dirs('%s%s/' % (self.jobs_dir, job))
        return sorted((os.path.basename(os.path.dirname(b))
                       for b in build_paths), key=int, reverse=True)

    def _get_build_finish_time(self, job, build):
        data = self.get('%s%s/%s/finished.json' % (self.jobs_dir, job, build),
                        as_json=True)
        if data is None:
            return None
        return int(data['timestamp'])

    def get_daily_builds(self, matcher, builds_have):
        """Generates all (job, build) pairs for the last day."""
        now = time.time()
        for job in self._get_jobs():
            if not matcher(job):
                continue
            for build in self._get_builds(job):
                if (job, build) in builds_have:
                    # assumption: we're only getting builds NEWER than those
                    # in builds_have.
                    break
                timestamp = self._get_build_finish_time(job, build)
                if timestamp is None:
                    continue
                # Quit once we've walked back over a day.
                if now - timestamp > MAX_AGE:
                    break
                yield job, build, timestamp

    def get_tests_from_build(self, job, build):
        """Generates all tests for a build."""
        for junit_path in self._ls_junit_paths(job, build):
            for test in self._get_tests_from_junit(junit_path):
                yield test


class IndexedList(list):
    """
    Convert values into indexes into a separate list of values.

    Provides a normal list interface, with an overriden index operator
    to return a position or insert into the list.
    """
    def __init__(self, iterable=None):
        self._index = {}
        if iterable:
            super(IndexedList, self).__init__(iterable)
            for n, el in enumerate(self):
                self._index[el] = n

    def index(self, value):
        """Return value's position, appending it to the end if not present."""
        if value not in self._index:
            n = len(self)
            self.append(value)
            self._index[value] = n
        return self._index[value]


def mp_init_worker(jobs_dir, metadata, client_class):
    """
    Initialize the environment for multiprocessing-based multithreading.
    """
    # Multiprocessing doesn't allow local variables for each worker, so we need
    # to make a GCSClient global variable.
    global WORKER_CLIENT
    WORKER_CLIENT = client_class(jobs_dir, metadata)
    signal.signal(signal.SIGINT, signal.SIG_IGN)  # make Ctrl-C kill the worker


def mp_get_tests((job, build, timestamp)):
    """
    Inside a multiprocessing worker, get the tests for a given job and build.
    """
    return job, build, timestamp, list(
        WORKER_CLIENT.get_tests_from_build(job, build))


def get_existing_builds(jobs):
    out = set()
    for job_name, job in jobs.iteritems():
        for build_num in job.iterkeys():
            out.add((job_name, build_num))
    return out


def get_tests(names, jobs_dir, metadata, matcher, threads, client_class, jobs):
    """
    Adds information about tests to a dictionary.

    Args:
        names: an IndexedList of test names.
        jobs_dir: the GCS path containing jobs.
        metadata: a dict of metadata about the jobs_dir.
        matcher: a function str->bool that determines whether to include a job.
        threads: how many threads to use to download build information.
        client_class: a constructor for a GCSClient (or a subclass).
        jobs: a dictionary to place new test information into. Builds already
            present in this dictionary will be skipped.
    Returns:
        jobs is modified to contain the new test information.
    """
    gcs = client_class(jobs_dir, metadata)

    print('Loading builds from %s' % jobs_dir)

    builds_have = get_existing_builds(jobs)
    if builds_have:
        print('already have %d builds' % len(builds_have))

    jobs_and_builds = gcs.get_daily_builds(matcher, builds_have)
    if threads > 1:
        pool = multiprocessing.Pool(threads, mp_init_worker,
                                    (jobs_dir, metadata, client_class))
        builds_tests_iterator = pool.imap_unordered(
            mp_get_tests, jobs_and_builds)
    else:
        # for debugging, have a single-threaded mode without multiprocessing
        builds_tests_iterator = (
            (job, build, timestamp, gcs.get_tests_from_build(job, build))
            for job, build, timestamp in jobs_and_builds)

    for job, build, timestamp, build_tests in builds_tests_iterator:
        print('%s/%s' % (job, build))
        build_info = jobs.setdefault(job, {}).setdefault(build, {})
        build_info['timestamp'] = timestamp
        build_info['tests'] = []
        for name, duration, failed, skipped in build_tests:
            result = {
                'name': names.index(name),
                'time': duration,
            }
            if failed:
                result['failed'] = True
            if skipped:
                result['skipped'] = True
            build_info['tests'].append(result)


def remove_old_builds(buckets, now):
    pruned = 0
    for jobs in buckets.itervalues():
        for job_name, job in jobs.iteritems():
            for build_num, build in job.items():  # intentional copy
                if build['timestamp'] < now - MAX_AGE:
                    pruned += 1
                    job.pop(build_num)
    print('pruned %d old builds' % pruned)


def main(jobs_dirs, match, outfile, threads, client_class=GCSClient):
    """Collect test info in matching jobs."""
    print('Finding tests in jobs matching %s' % match)
    matcher = re.compile(match).match
    tests = None
    if os.path.exists(outfile):
        try:
            tests = json.load(open(outfile))
            if 'test_names' not in tests:
                raise ValueError
        except ValueError:
            tests = None
        else:
            print('Resuming from previous run...')
            names = IndexedList(tests['test_names'])
            tests['test_names'] = names
            remove_old_builds(tests['buckets'], time.time())
    if tests is None:
        names = IndexedList()
        tests = {'test_names': names, 'buckets': {}}
    for bucket, metadata in jobs_dirs.iteritems():
        if not bucket.endswith('/'):
            bucket += '/'
        bucket_jobs = tests['buckets'].setdefault(bucket, {})
        get_tests(names, bucket, metadata, matcher, threads, client_class,
                  bucket_jobs)
    with open(outfile, 'w') as buf:
        json.dump(tests, buf, sort_keys=True)


def get_options(argv):
    """Process command line arguments."""
    parser = argparse.ArgumentParser()
    parser.add_argument(
        '--buckets',
        help='JSON file with GCS bucket locations',
        required=True,
    )
    parser.add_argument(
        '--match',
        help='filter to job names matching this re',
        required=True,
    )
    parser.add_argument(
        '--outfile',
        help='file to write output JSON to',
        default='tests.json',
    )
    parser.add_argument(
        '--threads',
        help='number of concurrent threads to download results with',
        default=32,
        type=int,
    )
    return parser.parse_args(argv)


if __name__ == '__main__':
    if os.getenv('REQ_CACHE'):
        # for fast test iterations, enable caching GCS HTTP responses
        import requests_cache
        requests_cache.install_cache(os.getenv('REQ_CACHE'))
    OPTIONS = get_options(sys.argv[1:])
    jobs_dirs = json.load(open(OPTIONS.buckets))
    main(jobs_dirs, OPTIONS.match, OPTIONS.outfile, OPTIONS.threads)
