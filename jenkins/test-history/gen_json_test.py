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

"""Tests for gen_json."""

import json
import tempfile
import unittest

import gen_json

import time


class GenJsonTest(unittest.TestCase):
    """Unit tests for gen_json.py."""

    def test_get_options(self):
        """Test argument parsing works correctly."""
        def check(args, expected_jobs_dir, expected_match):
            """Check that all args are parsed as expected."""
            options = gen_json.get_options(args)
            self.assertEquals(expected_jobs_dir, options.jobs_dir)
            self.assertEquals(expected_match, options.match)


        check(['--jobs_dir=foo', '--match=bar'], 'foo', 'bar')
        check(['--jobs_dir', 'foo', '--match', 'bar'], 'foo', 'bar')
        check(['--match=bar', '--jobs_dir=foo'], 'foo', 'bar')

    def test_get_options_missing(self):
        """Test missing arguments raise an exception."""
        def check(args):
            """Check that missing args raise an exception."""
            with self.assertRaises(SystemExit):
                gen_json.get_options(args)

        check([])


class MockedClient(gen_json.GCSClient):
    LOG_DIR = 'gs://kubernetes-jenkins/logs/'
    JOB_DIR = LOG_DIR + 'fake/123/'
    ART_DIR = JOB_DIR + 'artifacts/'
    lists = {
        LOG_DIR: [LOG_DIR + 'fake/'],
        LOG_DIR + 'fake/': [JOB_DIR, LOG_DIR + 'fake/122/'],
        ART_DIR: [ART_DIR + 'junit_01.xml']}
    gets = {
        JOB_DIR + 'finished.json': {'timestamp': time.time()},
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
        return self.gets[path]

    def ls(self, path, **kwargs):
        return self.lists[path]


class GCSClientTest(unittest.TestCase):
    """Unit tests for GCSClient"""

    JOBS_DIR = 'gs://kubernetes-jenkins/logs/'

    def setUp(self):
        self.client = gen_json.GCSClient(self.JOBS_DIR)

    def test_get(self):
        latest = int(self.client.get(self.JOBS_DIR +
            'kubernetes-e2e-gce/latest-build.txt'))
        self.assertGreater(latest, 1000)

    def test_ls(self):
        build_dir = self.JOBS_DIR + 'kubernetes-build-1.2/'
        paths = list(self.client.ls(build_dir))
        self.assertIn(build_dir + 'latest-build.txt', paths)

    def test_get_tests(self):
        client = MockedClient(self.JOBS_DIR)
        tests = list(client.get_tests_from_build('fake', '123'))
        self.assertEqual(tests, [
            ('Foo', 3, False, False),
            ('Bad', 4, True, False),
            ('Lazy', 0, False, True),
        ])

    def test_get_daily_builds(self):
        client = MockedClient(self.JOBS_DIR)
        builds = list(client.get_daily_builds(lambda x: True))
        self.assertEqual(builds, [('fake', 123)])

    def test_main(self):
        outfile = tempfile.NamedTemporaryFile(prefix='test-history-')
        gen_json.main(self.JOBS_DIR, 'fa', outfile.name, MockedClient)
        output = json.load(outfile)
        expected_output = {
            "Bad": {"fake": [{"failed": True, "build": 123, "time": 4}]},
            "Foo": {"fake": [{"failed": False, "build": 123, "time": 3.0}]},
            "Lazy": {}
        }
        self.assertEqual(output, expected_output)


if __name__ == '__main__':
    unittest.main()
