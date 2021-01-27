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

"""Tests for make_db."""

import time
import sys
import unittest

import make_db
import model



TEST_BUCKETS_DATA = {
    'gs://kubernetes-jenkins/logs/': {'prefix': ''},
    'gs://bucket1/': {'prefix': 'bucket1_prefix'},
    'gs://bucket2/': {'prefix': 'bucket2_prefix'}
}


class MockedClient(make_db.GCSClient):
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
        'gs://kubernetes-jenkins/pr-logs/directory/': [],
        ART_DIR: [ART_DIR + 'junit_01.xml'],
        ART_DIR.replace('123', '122'): [],
    }
    gets = {
        JOB_DIR + 'finished.json': {'timestamp': NOW, 'result': 'SUCCESS'},
        JOB_DIR + 'started.json': {'timestamp': NOW - 5},
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

    def get(self, path, as_json=True):
        return self.gets.get(path)

    def ls(self, path, **_kwargs):  # pylint: disable=arguments-differ
        return self.lists[path]


class GCSClientTest(unittest.TestCase):
    """Unit tests for GCSClient"""

    # pylint: disable=protected-access

    JOBS_DIR = 'gs://kubernetes-jenkins/logs/'

    def setUp(self):
        self.client = MockedClient(self.JOBS_DIR)

    def test_get_junits(self):
        junits = self.client.get_junits_from_build(self.JOBS_DIR + 'fake/123')
        self.assertEqual(
            sorted(junits),
            ['gs://kubernetes-jenkins/logs/fake/123/artifacts/junit_01.xml'])

    def test_get_builds_normal_list(self):
        # normal case: lists a directory
        self.assertEqual((True, ['123', '122']), self.client._get_builds('fake'))

    def test_get_builds_latest(self):
        # optimization: does a range based on build-latest.txt
        precise, gen = self.client._get_builds('latest')
        self.assertFalse(precise)
        self.assertEqual(['4', '3', '2', '1'], list(gen))

    def test_get_builds_limit(self):
        # optimization: does a range based on build-latest.txt
        precise, gen = self.client._get_builds('latest', build_limit=2)
        self.assertFalse(precise)
        self.assertEqual(['4', '3'], list(gen))

    def test_get_builds_latest_fallback(self):
        # fallback: still lists a directory when build-latest.txt isn't an int
        self.assertEqual((True, ['6']), self.client._get_builds('bad-latest'))

    def test_get_builds_non_sequential(self):
        # fallback: setting sequential=false causes directory listing
        self.client.metadata = {'sequential': False}
        self.assertEqual((True, ['4', '3']),
                         self.client._get_builds('latest'))

    def test_get_builds_exclude_list_no_match(self):
        # special case: job is not in excluded list
        self.client.metadata = {'exclude_jobs': ['notfake']}
        self.assertEqual([('fake', '123'), ('fake', '122')], list(self.client.get_builds(set())))

    def test_get_builds_exclude_list_match(self):
        # special case: job is in excluded list
        self.client.metadata = {'exclude_jobs': ['fake']}
        self.assertEqual([], list(self.client.get_builds(set())))

class MainTest(unittest.TestCase):
    """End-to-end test of the main function's output."""
    JOBS_DIR = GCSClientTest.JOBS_DIR

    def test_remove_system_out(self):
        self.assertEqual(make_db.remove_system_out('not<xml<lol'), 'not<xml<lol')
        self.assertEqual(
            make_db.remove_system_out('<a><b>c<system-out>bar</system-out></b></a>'),
            '<a><b>c</b></a>')

    @staticmethod
    def get_expected_builds():
        return {
            MockedClient.JOB_DIR.replace('123', '122')[:-1]:
                (None, {'timestamp': 123}, []),
            MockedClient.JOB_DIR[:-1]:
                ({'timestamp': MockedClient.NOW - 5},
                 {'timestamp': MockedClient.NOW, 'result': 'SUCCESS'},
                 [MockedClient.gets[MockedClient.ART_DIR + 'junit_01.xml']])
        }

    def assert_main_output(self, threads, expected=None, db=None,
                           client=MockedClient):
        if expected is None:
            expected = self.get_expected_builds()
        if db is None:
            db = model.Database(':memory:')
        make_db.main(db, {self.JOBS_DIR: {}}, threads, True, sys.maxsize, client)

        result = {path: (started, finished, db.test_results_for_build(path))
                  for _rowid, path, started, finished in db.get_builds()}

        self.assertEqual(result, expected)
        return db

    def test_clean(self):
        for threads in [1, 32]:
            self.assert_main_output(threads)

    def test_incremental_new(self):
        db = self.assert_main_output(1)

        new_junit = '''
            <testsuite>
                <testcase name="New" time="8"/>
                <testcase name="Foo" time="2.3"/>
            </testsuite>
        '''

        class MockedClientNewer(MockedClient):
            NOW = int(time.time())
            LOG_DIR = 'gs://kubernetes-jenkins/logs/'
            JOB_DIR = LOG_DIR + 'fake/124/'
            ART_DIR = JOB_DIR + 'artifacts/'
            lists = {
                LOG_DIR: [LOG_DIR + 'fake/'],
                LOG_DIR + 'fake/': [JOB_DIR, LOG_DIR + 'fake/123/'],
                ART_DIR: [ART_DIR + 'junit_01.xml'],
                'gs://kubernetes-jenkins/pr-logs/directory/': [],
            }
            gets = {
                JOB_DIR + 'finished.json': {'timestamp': NOW},
                ART_DIR + 'junit_01.xml': new_junit,
            }

        expected = self.get_expected_builds()
        expected[MockedClientNewer.JOB_DIR[:-1]] = (
            None, {'timestamp': MockedClientNewer.NOW}, [new_junit])

        self.assert_main_output(1, expected, db, MockedClientNewer)


if __name__ == '__main__':
    unittest.main()
