#!/usr/bin/env python

# Copyright 2016 The Kubernetes Authors.
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

"""Tests for gen_json."""

import json
import os
import shutil
import tempfile
import unittest

import gen_json

import time


TEST_BUCKETS_DATA = {
    'gs://kubernetes-jenkins/logs/': { 'prefix': '' },
    'gs://bucket1/': { 'prefix': 'bucket1_prefix' },
    'gs://bucket2/': { 'prefix': 'bucket2_prefix' }
}


class OptionsTest(unittest.TestCase):
    """Tests for gen_json.get_options."""

    def test_get_options(self):
        """Test argument parsing works correctly."""
        def check(args, expected_buckets, expected_match):
            """Check that all args are parsed as expected."""
            options = gen_json.get_options(args)
            self.assertEquals(expected_buckets, options.buckets)
            self.assertEquals(expected_match, options.match)

        check(['--buckets=foo', '--match=bar'], 'foo', 'bar')
        check(['--buckets', 'foo', '--match', 'bar'], 'foo', 'bar')
        check(['--match=bar', '--buckets=foo'], 'foo', 'bar')

    def test_get_options_missing(self):
        """Test missing arguments raise an exception."""
        def check(args):
            """Check that missing args raise an exception."""
            with self.assertRaises(SystemExit):
                gen_json.get_options(args)

        check([])
        check(['--match=regex'])
        check(['--buckets=file'])


class MockedClient(gen_json.GCSClient):
    """A GCSClient with stubs for external interactions."""
    NOW = int(time.time())
    LOG_DIR = 'gs://kubernetes-jenkins/logs/'
    JOB_DIR = LOG_DIR + 'fake/123/'
    ART_DIR = JOB_DIR + 'artifacts/'
    lists = {
        LOG_DIR: [LOG_DIR + 'fake/'],
        LOG_DIR + 'fake/': [JOB_DIR, LOG_DIR + 'fake/122/'],
        LOG_DIR + 'bad-latest/': [LOG_DIR + 'bad-latest/6/'],
        LOG_DIR + 'latest/': [LOG_DIR + 'latest/4/', LOG_DIR + 'latest/3/'],
        ART_DIR: [ART_DIR + 'junit_01.xml']}
    gets = {
        JOB_DIR + 'finished.json': {'timestamp': NOW},
        LOG_DIR + 'latest/latest-build.txt': '4',
        LOG_DIR + 'bad-latest/latest-build.txt': 'asdf',
        LOG_DIR + 'fake/122/finished.json': {'timestamp': 123},
        ART_DIR + 'junit_01.xml': '''
    <testsuite>
        <testcase name="Foo" time="3" />
        <testcase name="Bad" time="4">
            <failure>stacktrace</failure>
        </testcase>
        <testcase name="Lazy" time="0">
            <skipped />
        </testcase>
    </testsuite>
    '''}

    def get(self, path, **kwargs):
        return self.gets.get(path)

    def ls(self, path, **kwargs):
        return self.lists[path]


class IndexedListTest(unittest.TestCase):
    def test_basic(self):
        l = gen_json.IndexedList()
        self.assertEqual(l.index('foo'), 0)
        self.assertEqual(l.index('bar'), 1)
        self.assertEqual(l.index('foo'), 0)
        self.assertEqual(l, ['foo', 'bar'])

    def test_init(self):
        l = gen_json.IndexedList(['foo', 'bar'])
        self.assertEqual(l.index('baz'), 2)
        self.assertEqual(l.index('bar'), 1)
        self.assertEqual(l.index('foo'), 0)
        self.assertEqual(l, ['foo', 'bar', 'baz'])


class GCSClientTest(unittest.TestCase):
    """Unit tests for GCSClient"""

    JOBS_DIR = 'gs://kubernetes-jenkins/logs/'

    def setUp(self):
        self.client = MockedClient(self.JOBS_DIR)

    def test_real_get(self):
        client = gen_json.GCSClient(self.JOBS_DIR)
        latest = int(client.get(self.JOBS_DIR +
            'kubernetes-e2e-gce/latest-build.txt'))
        self.assertGreater(latest, 1000)

    def test_real_ls(self):
        client = gen_json.GCSClient(self.JOBS_DIR)
        build_dir = self.JOBS_DIR + 'kubernetes-build-1.2/'
        paths = list(client.ls(build_dir))
        self.assertIn(build_dir + 'latest-build.txt', paths)

    def test_get_tests(self):
        tests = list(self.client.get_tests_from_build('fake', '123'))
        self.assertEqual(tests, [
            ('Foo', 3, False, False),
            ('Bad', 4, True, False),
            ('Lazy', 0, False, True),
        ])

    def test_get_tests_empty_time(self):
        gets = dict(self.client.gets)
        gets[self.client.ART_DIR + 'junit_01.xml'] = (
            '<testsuite><testcase name="Empty" time="" /></testsuite>')
        self.client.gets = gets
        tests = list(self.client.get_tests_from_build('fake', '123'))
        self.assertEqual(tests, [('Empty', 0.0, False, False)])

    def test_get_builds_normal_list(self):
        # normal case: lists a directory
        self.assertEqual(['123', '122'], self.client._get_builds('fake'))

    def test_get_builds_latest(self):
        # optimization: does a range based on build-latest.txt
        self.assertEqual(['4', '3', '2', '1'],
                         list(self.client._get_builds('latest')))

    def test_get_builds_latest_list_fallback(self):
        # fallback: still lists a directory when build-latest.txt isn't an int
        self.assertEqual(['6'], list(self.client._get_builds('bad-latest')))

    def test_get_builds_non_sequential(self):
        # fallback: setting sequential=false causes directory listing
        self.client.metadata = {'sequential': False}
        self.assertEqual(['4', '3'],
                         list(self.client._get_builds('latest')))

    def test_get_daily_builds(self):
        builds = list(self.client.get_daily_builds(lambda x: True, set()))
        self.assertEqual(builds, [('fake', '123', self.client.NOW)])

    def test_get_daily_builds_skip(self):
        # builds that we already have are filtered out.
        have = {('fake', '123')}
        builds = list(self.client.get_daily_builds(lambda x: True, have))
        self.assertEqual(builds, [])


class MainTest(unittest.TestCase):
    """End-to-end test of the main function's output."""
    JOBS_DIR = GCSClientTest.JOBS_DIR

    def get_expected_json(self):
        return {
            'test_names': ['Foo', 'Bad', 'Lazy'],
            'buckets': {'gs://kubernetes-jenkins/logs/':
                {'fake': {'123':
                    {'timestamp': MockedClient.NOW,
                     'tests': [
                             {'name': 0, 'time': 3.0},
                             {'failed': True, 'name': 1, 'time': 4.0},
                             {'skipped': True, 'name': 2, 'time': 0.0}
        ]}}}}}

    def assert_main_output(self, outfile, threads, expected=None,
                           client=MockedClient):
        if expected is None:
            expected = self.get_expected_json()
        gen_json.main({self.JOBS_DIR: {}}, 'fa', outfile.name, 32, client)
        output = json.load(outfile)
        self.assertEqual(output, expected)

    def test_clean(self):
        for threads in [1, 32]:
            outfile = tempfile.NamedTemporaryFile(prefix='test-history-')
            self.assert_main_output(outfile, threads)

    def test_incremental_noop(self):
        outfile = tempfile.NamedTemporaryFile(prefix='test-history-')
        self.assert_main_output(outfile, 1)
        outfile.seek(0)
        self.assert_main_output(outfile, 1)

    def test_incremental_new(self):
        outfile = tempfile.NamedTemporaryFile(prefix='test-history-')
        self.assert_main_output(outfile, 1)
        outfile.seek(0)

        class MockedClientNewer(MockedClient):
            NOW = int(time.time())
            LOG_DIR = 'gs://kubernetes-jenkins/logs/'
            JOB_DIR = LOG_DIR + 'fake/124/'
            ART_DIR = JOB_DIR + 'artifacts/'
            lists = {
                LOG_DIR: [LOG_DIR + 'fake/'],
                LOG_DIR + 'fake/': [JOB_DIR, LOG_DIR + 'fake/123/'],
                ART_DIR: [ART_DIR + 'junit_01.xml'],
            }
            gets = {
                JOB_DIR + 'finished.json': {'timestamp': NOW},
                ART_DIR + 'junit_01.xml': '''
                    <testsuite>
                        <testcase name="New" time="8"/>
                        <testcase name="Foo" time="2.3"/>
                    </testsuite>
                '''
            }

        expected = self.get_expected_json()
        expected['test_names'].append('New')
        expected['buckets'][MockedClient.LOG_DIR]['fake']['124'] = {
            'timestamp': MockedClientNewer.NOW,
            'tests': [{'name': 3, 'time': 8},
                      {'name': 0, 'time': 2.3}]
        }
        self.assert_main_output(outfile, 1, expected, MockedClientNewer)


if __name__ == '__main__':
    unittest.main()
