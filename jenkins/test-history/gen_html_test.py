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

"""Tests for gen_html."""

import json
import os
import shutil
import StringIO
import tempfile
import unittest

import yaml

import gen_html


TEST_DATA = {
    'test_names': ['test1', 'test2'],
    'buckets': {
        'gs://kubernetes-jenkins/logs/': {
            'kubernetes-release': {
                '3': {'tests': [{'name': 0, 'time': 3.52}]},
                '4': {'tests': [{'name': 1, 'time': 63.21, 'failed': True}]},
            },
            'kubernetes-debug': {
                '5': {'tests': [{'name': 0, 'time': 7.56}]},
                '6': {'tests': [
                    {'name': 0, 'time': 8.43},
                    {'name': 1, 'failed': True, 'time': 3.53},
                ]},
            },
        },
        'gs://rktnetes-jenkins/logs/': {
            'kubernetes-release': {
                '7': {'tests': [{'name': 0, 'time': 0.0, 'skipped': True}]},
            },
        },
        'gs://kube_azure_log/': {},
    },
}


TEST_BUCKETS_DATA = {
    "gs://kubernetes-jenkins/logs/": { "prefix": "" },
    "gs://rktnetes-jenkins/logs/": { "prefix": "rktnetes$" },
    "gs://kube_azure_log/": { "prefix": "azure$" },
}


class GenHtmlTest(unittest.TestCase):
    """Unit tests for gen_html.py."""
    # pylint: disable=invalid-name

    def test_job_results(self):
        """Test that job_results returns what we want."""
        bucket = 'gs://kubernetes-jenkins/logs/'
        job = 'kubernetes-release'
        summary, tests = gen_html.job_results(
            bucket,
            '',
            job,
            TEST_DATA['buckets'][bucket][job],
            TEST_DATA['test_names'])
        self.assertEqual(summary.name, job)
        self.assertEqual(summary.passed, 1)
        self.assertEqual(summary.failed, 1)
        self.assertEqual(summary.tests, 2)
        self.assertEqual(summary.stable, 1)
        self.assertEqual(summary.unstable, 0)
        self.assertEqual(summary.broken, 1)
        self.assertEqual(len(tests), 2)
        self.assertEqual(tests[0]['name'], 'test2')
        self.assertEqual(tests[1]['name'], 'test1')
        self.assertEqual(tests[0]['runs'], 1)
        self.assertEqual(tests[0]['failed'], 1)

        # Skipped tests don't show up.
        bucket = 'gs://rktnetes-jenkins/logs/'
        job = 'kubernetes-release'
        summary, tests = gen_html.job_results(
            bucket,
            'rktnetes$',
            job,
            TEST_DATA['buckets'][bucket][job],
            TEST_DATA['test_names'])
        self.assertEqual(summary.name, 'rktnetes$' + job)
        self.assertEqual(summary.passed, 1)
        self.assertEqual(summary.failed, 0)
        self.assertEqual(summary.tests, 0)

    def test_list_jobs(self):
        """Test that list_jobs gives job data of the right length."""
        expecteds = [
            ('gs://kubernetes-jenkins/logs/', 'kubernetes-debug', 2),
            ('gs://kubernetes-jenkins/logs/', 'kubernetes-release', 2),
            ('gs://rktnetes-jenkins/logs/', 'kubernetes-release', 1),
        ]
        actuals = sorted(list(gen_html.list_jobs(TEST_DATA)))
        self.assertEqual(len(expecteds), len(actuals))
        for expected, actual in zip(expecteds, actuals):
            self.assertEqual(expected[0], actual[0])
            self.assertEqual(expected[1], actual[1])
            self.assertEqual(expected[2], len(actual[2]))

    def test_merge_bad_tests(self):
        """Test that merge_bad_tests merges failed tests."""
        def check(bad, new, expected):
            gen_html.merge_bad_tests(bad, new)
            self.assertEqual(bad, expected)

        check({}, [], {})
        check({}, [{'failed': 0}], {})
        failed_test = {
            'failed': 1,
            'name': 'failed_test',
            'runs': 1,
            'passed': 0,
            'latest_failure': None,
        }
        check({}, [failed_test], {'failed_test': failed_test})
        check({'failed_test': failed_test}, [failed_test], {
            'failed_test': {
                'failed': 2,
                'name': 'failed_test',
                'runs': 2,
                'passed': 0,
                'latest_failure': None,
            }
        })
        check({'failed_test2': failed_test}, [failed_test], {
            'failed_test2': failed_test, 'failed_test': failed_test})

    def test_get_options(self):
        """Test argument parsing works correctly."""

        def check(args, expected_output_dir, expected_input,
                  expected_buckets):
            """Check that args is parsed correctly."""
            options = gen_html.get_options(args)
            self.assertEquals(expected_output_dir, options.output_dir)
            self.assertEquals(expected_input, options.input)
            self.assertEquals(expected_buckets, options.buckets)


        check(['--output-dir=foo', '--input=bar', '--buckets=baz'],
              'foo', 'bar', 'baz')
        check(['--output-dir', 'foo', '--input', 'bar', '--buckets', 'baz'],
              'foo', 'bar', 'baz')
        check(['--buckets=baz', '--input=bar', '--output-dir=foo'],
              'foo', 'bar', 'baz')

    def test_get_options_missing(self):
        """Test missing arguments raise an exception."""
        def check(args):
            """Check that args raise an exception."""
            with self.assertRaises(SystemExit):
                gen_html.get_options(args)

        check([])
        check(['--output-dir=foo'])
        check(['--input=bar'])
        check(['--output-dir=foo', '--input=bar'])
        check(['--buckets=baz', '--input=bar'])
        check(['--buckets=baz', '--output-dir=foo'])

    def test_load_prefixes(self):
        """Test load_prefixes does what we think."""
        data = '{ "gs://bucket/": { "prefix": "bucket_prefix" } }'
        prefixes = gen_html.load_prefixes(StringIO.StringIO(data))
        self.assertEquals(prefixes['gs://bucket/'], 'bucket_prefix')

    def test_main(self):
        """Test main() creates pages."""
        temp_dir = tempfile.mkdtemp(prefix='kube-test-hist-')
        try:
            tests_json = os.path.join(temp_dir, 'tests.json')
            with open(tests_json, 'w') as buf:
                json.dump(TEST_DATA, buf)
            buckets_yaml = os.path.join(temp_dir, 'buckets.yaml')
            with open(buckets_yaml, 'w') as buf:
                yaml.dump(TEST_BUCKETS_DATA, buf)
            gen_html.main(tests_json, buckets_yaml, temp_dir)
            for page in (
                    'index',
                    'suite-kubernetes-release',
                    'suite-kubernetes-debug'):
                self.assertTrue(os.path.exists('%s/%s.html' % (temp_dir, page)))
        finally:
            shutil.rmtree(temp_dir)


if __name__ == '__main__':
    unittest.main()
