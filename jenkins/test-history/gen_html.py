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

"""Creates an HTML report out of the JSON."""


from __future__ import division
from __future__ import print_function

import argparse
import collections
import json
import os
import re
import sys
import time

import jinja2
import yaml


BLOCKING_JOBS = [
    'kubelet-gce-e2e-ci',
    'kubernetes-build',
    'kubernetes-e2e-gce',
    'kubernetes-e2e-gce-scalability',
    'kubernetes-e2e-gce-slow',
    'kubernetes-e2e-gke',
    'kubernetes-e2e-gke-slow',
    'kubernetes-kubemark-5-gce',
    'kubernetes-kubemark-500-gce',
    'kubernetes-test-go',
]

JobSummary = collections.namedtuple('JobSummary', [
    'name',
    'passed',
    'failed',
    'latest_failure',
    'tests',
    'stable',
    'unstable',
    'broken',
])


BASE_DIR = os.path.dirname(os.path.abspath(__file__))

JINJA_ENV = jinja2.Environment(
    loader=jinja2.FileSystemLoader(BASE_DIR + '/templates/'),
    extensions=['jinja2.ext.autoescape'],
    trim_blocks=True,
    autoescape=True)


def failure_class(passed, failed):
    if failed == 0:
        return ''
    elif passed == 0:
        return 'job-broken'
    elif passed / 10 < failed:
        return 'job-troubled'
    else:
        return 'job-flaky'

JINJA_ENV.globals['failure_class'] = failure_class


def load_prefixes(in_file):
    """Returns a dictionary from bucket to prefix using in_file."""
    result = {}
    buckets = yaml.load(in_file)
    for bucket, data in buckets.iteritems():
        result[bucket] = data['prefix']
    return result


def slugify(inp):
    """Convert a string into a url-safe slug fragment.

    This matches the slugify code in Gubernator.
    """
    inp = re.sub(r'[^\w\s-]+', '', inp)
    return re.sub(r'\s+', '-', inp).lower()


def gubernator_url(bucket, job, build, test_name=''):
    """Build a link to a test failure on Gubernator."""
    return 'https://k8s-gubernator.appspot.com/build/{}{}/{}#{}'.format(
        bucket[5:], job, build, slugify(test_name))


def job_results(bucket, prefix, job_name, job_data, test_names):
    """Generates a JobSummary namedtuple along with a list of test results.

    This function is a bit of a monolith and should probably be cleared up.

    Args:
        bucket: The bucket name.
        prefix: The contributor prefix to prepend to the job name.
        job_name: The job name.
        job_data: The JSON data as returned by list_jobs.
        test_names: The JSON test names data.
    Returns:
        A (JobSummary, tests) tuple where tests is a list of dicts with this
        structure, sorted by failure count, passed count, then name: [{
            'name': 'abc',
            'runs': 1,
            'passed': 1,
            'failed': 0,
            'duration': 0.0,
            'latest_failure': None or 'https://gubernater-link...',
        }]
    """
    full_job_name = '{}{}'.format(prefix, job_name)
    num_failed = 0
    num_passed = 0
    latest_failure = None
    test_summaries = {}
    for build in job_data:
        build_passed = True
        tests_data = job_data[build]['tests']
        for test in tests_data:
            test_id = test['name']
            if test_id not in test_summaries:
                test_summaries[test_id] = {
                    'name': test_names[test_id],
                    'runs': 0,
                    'passed': 0,
                    'failed': 0,
                    'duration': 0.0,
                    'latest_failure': None,
                }
            if 'skipped' in test:
                pass
            else:
                test_summaries[test_id]['runs'] += 1
                test_summaries[test_id]['duration'] += test['time']
                if 'failed' in test:
                    test_summaries[test_id]['latest_failure'] = build
                    latest_failure = build
                    build_passed = False
                    test_summaries[test_id]['failed'] += 1
                else:
                    test_summaries[test_id]['passed'] += 1
        if build_passed:
            num_passed += 1
        else:
            num_failed += 1
    stable = 0
    unstable = 0
    broken = 0
    tests = []
    for test in sorted(test_summaries.values(), key=lambda t: t['name'].lower()):
        if test['latest_failure'] is not None:
            test['latest_failure'] = gubernator_url(bucket, job_name, test['latest_failure'], test['name'])
        if test['runs'] > 0:
            tests.append(test)
            if test['failed'] == 0:
                stable += 1
            elif test['failed'] < test['runs']:
                unstable += 1
            else:
                broken += 1
    if latest_failure is not None:
        latest_failure = gubernator_url(bucket, job_name, latest_failure)
    job_summary = JobSummary(
        full_job_name,
        num_passed,
        num_failed,
        latest_failure,
        stable + unstable + broken,
        stable,
        unstable,
        broken)
    tests.sort(key=lambda t: (t['failed'], t['passed']), reverse=True)
    return job_summary, tests


def list_jobs(data):
    """Generates a (bucket, name, data) tuple for each job."""
    for bucket, jobs in data['buckets'].iteritems():
        for name, job in jobs.iteritems():
            yield bucket, name, job


def merge_bad_tests(bad_tests, new_tests):
    """Merge unstable and broken tests from new_tests into bad_tests.

    Args:
        bad_tests: Dictionary from test name to test results.
        new_tests: List of new test results to merge.
    Returns:
        Nothing, modifies bad_tests in place.
    """
    for new_test in new_tests:
        if new_test['failed'] > 0:
            name = new_test['name']
            if name not in bad_tests:
                bad_tests[name] = {
                    'name': name,
                    'runs': 0,
                    'passed': 0,
                    'failed': 0,
                    'latest_failure': None,
                }
            bad_tests[name]['runs'] += new_test['runs']
            bad_tests[name]['passed'] += new_test['passed']
            bad_tests[name]['failed'] += new_test['failed']
            bad_tests[name]['latest_failure'] = new_test['latest_failure']


def main(in_path, buckets_path, out_dir):
    """Uses in_path and buckets_path to write a static report under out_dir."""
    with open(in_path) as data_file:
        data = json.load(data_file)

    templates_path = '{}/templates'.format(os.path.abspath(os.path.dirname(__file__)))

    summaries = []
    bad_tests = {}
    with open(buckets_path) as buckets_file:
        prefixes = load_prefixes(buckets_file)
    for bucket, job_name, job_data in list_jobs(data):
        if bucket not in prefixes:
            raise ValueError('Unknown bucket: {}'.format(bucket))
        prefix = prefixes[bucket]
        full_name = '{}{}'.format(prefix, job_name)
        job, tests = job_results(bucket, prefix, job_name, job_data, data['test_names'])
        if full_name in BLOCKING_JOBS:
            merge_bad_tests(bad_tests, tests)
        summaries.append(job)
        if job.tests > 0:
            job_template = JINJA_ENV.get_template('job.html')
            job_html = job_template.render({
                'job_name': job_name,
                'tests': tests,
            })
            with open('{}/suite-{}.html'.format(out_dir, full_name), 'w') as job_file:
                job_file.write(job_html)
    summaries.sort()
    blocking_job_summaries = filter(lambda s: s.name in BLOCKING_JOBS, summaries)

    index_template = JINJA_ENV.get_template('index.html')
    index_html = index_template.render({
        'last_updated': time.strftime('%a %b %d %T %Z'),
        'job_groups': [blocking_job_summaries, summaries],
        'bad_tests': sorted(bad_tests.values(), key=lambda t: t['failed'] / (t['passed'] + t['failed']), reverse=True),
    })
    with open('{}/index.html'.format(out_dir), 'w') as index_file:
        index_file.write(index_html)


def get_options(argv):
    """Process command line arguments."""
    parser = argparse.ArgumentParser()
    parser.add_argument('--output-dir', required=True,
                        help='where to write output pages')
    parser.add_argument('--input', required=True,
                        help='JSON test data to read for input')
    parser.add_argument('--buckets', required=True,
                        help='JSON GCS buckets to read for test results')
    return parser.parse_args(argv)


if __name__ == '__main__':
    OPTIONS = get_options(sys.argv[1:])
    main(OPTIONS.input, OPTIONS.buckets, OPTIONS.output_dir)
