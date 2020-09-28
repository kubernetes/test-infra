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

import io as StringIO
import json
import time
import unittest

from parameterized import parameterized

import make_json
import model


class ValidateBuckets(unittest.TestCase):
    def test_buckets(self):
        prefixes = set()
        for name, options in sorted(make_json.BUCKETS.items()):
            if name == 'gs://kubernetes-jenkins/logs/':
                continue  # only bucket without a prefix
            prefix = options.get('prefix', '')
            self.assertNotEqual(prefix, '', 'bucket %s must have a prefix' % name)
            self.assertNotIn(prefix, prefixes, "bucket %s prefix %r isn't unique" % (name, prefix))
            self.assertEqual(prefix[-1], ':', "bucket %s prefix should be %s:" % (name, prefix))


class BuildObjectTests(unittest.TestCase):
    @parameterized.expand([
        (
            "Empty_Build",
            make_json.Build.__new__(make_json.Build),
            {},
        ),
        (
            "Base_Build",
            make_json.Build("gs://kubernetes-jenkins/pr-logs/path", []),
            {"path":"gs://kubernetes-jenkins/pr-logs/path",
             "test": [],
             "tests_run": 0,
             "tests_failed": 0,
             "job": "pr:pr-logs",
            },
        ),
        (
            "Tests_populate",
            make_json.Build(
                "gs://kubernetes-jenkins/pr-logs/path",
                [{'name': 'Test1', 'failed': True}],
            ),
            {"path":"gs://kubernetes-jenkins/pr-logs/path",
             "test": [{'name': 'Test1', 'failed': True}],
             "tests_run": 1,
             "tests_failed": 1,
             "job": "pr:pr-logs",
            },
        ),
    ])
    def test_as_dict(self, _, build, expected):
        self.assertEqual(build.as_dict(), expected)

    @parameterized.expand([
        (
            "No started",
            {},
            {},
        ),
        (
            "CI Decorated",
            {
                "timestamp":1595284709,
                "repos":{"kubernetes/kubernetes":"master"},
                "repo-version":"5a529aa3a0dd3a050c5302329681e871ef6c162e",
                "repo-commit":"5a529aa3a0dd3a050c5302329681e871ef6c162e",
            },
            {
                "started": 1595284709,
                "repo_commit":"5a529aa3a0dd3a050c5302329681e871ef6c162e",
                "repos": '{"kubernetes/kubernetes": "master"}',
            },
        ),
        (
            "PR Decorated",
            {
                "timestamp":1595277241,
                "pull":"93264",
                "repos":{"kubernetes/kubernetes":"master:5feab0"},
                "repo-version":"30f64c5b1fc57a3beb1476f9beb29280166954d1",
                "repo-commit":"30f64c5b1fc57a3beb1476f9beb29280166954d1",
            },
            {
                "started": 1595277241,
                "repo_commit":"30f64c5b1fc57a3beb1476f9beb29280166954d1",
                "repos": '{"kubernetes/kubernetes": "master:5feab0"}',
            },
        ),
        (
            "PR Bootstrap",
            {
                "node": "0790211c-cacb-11ea-a4b9-4a19d9b965b2",
                "pull": "master:5a529",
                "repo-version": "v1.20.0-alpha.0.261+06ea384605f172",
                "timestamp": 1595278460,
                "repos": {"k8s.io/kubernetes": "master:5a529", "k8s.io/release": "master"},
                "version": "v1.20.0-alpha.0.261+06ea384605f172"
            },
            {
                "started": 1595278460,
                "repo_commit":"v1.20.0-alpha.0.261+06ea384605f172",
                "repos": '{"k8s.io/kubernetes": "master:5a529", "k8s.io/release": "master"}',
                "executor": "0790211c-cacb-11ea-a4b9-4a19d9b965b2",
            },
        ),
        (
            "CI Bootstrap",
            {
                "timestamp":1595263104,
                "node":"592473ae-caa7-11ea-b130-525df2b76a8d",
                "repos":{
                    "k8s.io/kubernetes":"master",
                    "k8s.io/release":"master"
                },
                "repo-version":"v1.20.0-alpha.0.255+5feab0aa1e592a",
            },
            {
                "started": 1595263104,
                "repo_commit":"v1.20.0-alpha.0.255+5feab0aa1e592a",
                "repos": '{"k8s.io/kubernetes": "master", "k8s.io/release": "master"}',
                "executor": "592473ae-caa7-11ea-b130-525df2b76a8d",
            },
        ),
    ])
    def test_populate_start(self, _, started, updates):
        build = make_json.Build("gs://kubernetes-jenkins/pr-logs/path", [])
        attrs = {
            "path":"gs://kubernetes-jenkins/pr-logs/path",
            "test": [],
            "tests_run": 0,
            "tests_failed": 0,
            "job": "pr:pr-logs",
                }
        attrs.update(updates)
        build.populate_start(started)
        self.assertEqual(build.as_dict(), attrs)

    @parameterized.expand([
        (
            "No finished",
            {},
            {},
        ),
        (
            "CI Decorated",
            {
                "timestamp":1595286616,
                "passed":True,
                "result":"SUCCESS",
                "revision":"master",
            },
            {
                "finished": 1595286616,
                "result": "SUCCESS",
                "passed": True,
            },
        ),
        (
            "PR Decorated",
            {
                "timestamp":1595279434,
                "passed":True,
                "result":"SUCCESS",
                "revision":"5dd9241d43f256984358354d1fec468f274f9ac4"
            },
            {
                "finished": 1595279434,
                "result": "SUCCESS",
                "passed": True,
            },
        ),
        (
            "PR Bootstrap",
            {
                "timestamp": 1595282312,
                "version": "v1.20.0-alpha.0.261+06ea384605f172",
                "result": "SUCCESS",
                "passed": True,
                "job-version": "v1.20.0-alpha.0.261+06ea384605f172",
            },
            {
                "finished": 1595282312,
                "version": "v1.20.0-alpha.0.261+06ea384605f172",
                "result": "SUCCESS",
                "passed": True,
            },
        ),
        (
            "CI Bootstrap",
            {
                "timestamp": 1595263185,
                "version": "v1.20.0-alpha.0.255+5feab0aa1e592a",
                "result": "SUCCESS",
                "passed": True,
                "job-version": "v1.20.0-alpha.0.255+5feab0aa1e592a",
            },
            {
                "finished": 1595263185,
                "version": "v1.20.0-alpha.0.255+5feab0aa1e592a",
                "result": "SUCCESS",
                "passed": True,
            },
        ),
    ])
    def test_populate_finish(self, _, finished, updates):
        build = make_json.Build("gs://kubernetes-jenkins/pr-logs/path", [])
        attrs = {"path":"gs://kubernetes-jenkins/pr-logs/path",
                 "test": [],
                 "tests_run": 0,
                 "tests_failed": 0,
                 "job": "pr:pr-logs",
                }
        build.populate_finish(finished)
        attrs.update(updates)
        self.assertEqual(build.as_dict(), attrs)


class GenerateBuilds(unittest.TestCase):
    @parameterized.expand([
        (
            "Basic_pass",
            "gs://kubernetes-jenkins/pr-logs/path",
            [{'name': "Test1", 'failed': False}],
            None,
            None,
            None,
            None,
            {
                'job': 'pr:pr-logs',
                'path': 'gs://kubernetes-jenkins/pr-logs/path',
                'test': [{'name': 'Test1', 'failed': False}],
                'tests_run': 1,
                'tests_failed':0,
            },
        ),
        (
            "Basic_fail",
            "gs://kubernetes-jenkins/pr-logs/path",
            [{'name': "Test1", 'failed': True}],
            None,
            None,
            None,
            None,
            {
                'job': 'pr:pr-logs',
                'path': 'gs://kubernetes-jenkins/pr-logs/path',
                'test': [{'name': 'Test1', 'failed': True}],
                'tests_run': 1,
                'tests_failed':1,
            },
        ),
        (
            "Ci_decorated",
            "gs://kubernetes-jenkins/pr-logs/path",
            [{'name': "Test1", 'failed': True}],
            {
                "timestamp":1595284709,
                "repos":{"kubernetes/kubernetes":"master"},
                "repo-version":"5a529aa3a0dd3a050c5302329681e871ef6c162e",
                "repo-commit":"5a529aa3a0dd3a050c5302329681e871ef6c162e",
            },
            {
                "timestamp":1595286616,
                "passed":True,
                "result":"SUCCESS",
                "revision":"master",
            },
            None,
            None,
            {
                'job': 'pr:pr-logs',
                'path': 'gs://kubernetes-jenkins/pr-logs/path',
                'test': [{'name': 'Test1', 'failed': True}],
                'passed': True,
                'result': 'SUCCESS',
                'elapsed': 1907,
                'tests_run': 1,
                'tests_failed':1,
                'started': 1595284709,
                'finished': 1595286616,
                'repo_commit': '5a529aa3a0dd3a050c5302329681e871ef6c162e',
                'repos': '{"kubernetes/kubernetes": "master"}',
            },
        ),
        (
            "Pr_decorated",
            "gs://kubernetes-jenkins/pr-logs/path",
            [{'name': "Test1", 'failed': True}],
            {
                "timestamp":1595277241,
                "pull":"93264",
                "repos":{"kubernetes/kubernetes":"master:5feab0"},
                "repo-version":"30f64c5b1fc57a3beb1476f9beb29280166954d1",
                "repo-commit":"30f64c5b1fc57a3beb1476f9beb29280166954d1",
            },
            {
                "timestamp":1595279434,
                "passed":True,
                "result":"SUCCESS",
                "revision":"5dd9241d43f256984358354d1fec468f274f9ac4"
            },
            None,
            None,
            {
                'job': 'pr:pr-logs',
                'path': 'gs://kubernetes-jenkins/pr-logs/path',
                'test': [{'name': 'Test1', 'failed': True}],
                'passed': True,
                'result': 'SUCCESS',
                'elapsed': 2193,
                'tests_run': 1,
                'tests_failed':1,
                'started': 1595277241,
                'finished': 1595279434,
                'repo_commit': '30f64c5b1fc57a3beb1476f9beb29280166954d1',
                'repos': '{"kubernetes/kubernetes": "master:5feab0"}',
            },
        ),
        (
            "Pr_bootstrap",
            "gs://kubernetes-jenkins/pr-logs/path",
            [{'name': "Test1", 'failed': True}],
            {
                "node": "0790211c-cacb-11ea-a4b9-4a19d9b965b2",
                "pull": "master:5a529",
                "repo-version": "v1.20.0-alpha.0.261+06ea384605f172",
                "timestamp": 1595278460,
                "repos": {
                    "k8s.io/kubernetes": "master:5a529",
                    "k8s.io/release": "master"
                },
                "version": "v1.20.0-alpha.0.261+06ea384605f172"
            },
            {
                "timestamp": 1595282312,
                "version": "v1.20.0-alpha.0.261+06ea384605f172",
                "result": "SUCCESS",
                "passed": True,
                "job-version": "v1.20.0-alpha.0.261+06ea384605f172",
            },
            {
                "node_os_image": "cos-81-12871-59-0",
                "infra-commit": "2a9a0f868",
                "repo": "k8s.io/kubernetes",
                "master_os_image": "cos-81-12871-59-0",
            },
            {
                "k8s.io/kubernetes": "master:5a529",
                "k8s.io/release": "master"
            },
            {
                'job': 'pr:pr-logs',
                'executor': '0790211c-cacb-11ea-a4b9-4a19d9b965b2',
                'path': 'gs://kubernetes-jenkins/pr-logs/path',
                'test': [{'name': 'Test1', 'failed': True}],
                'passed': True,
                'result': 'SUCCESS',
                'elapsed': 3852,
                'tests_run': 1,
                'tests_failed':1,
                'started': 1595278460,
                'finished': 1595282312,
                'version': 'v1.20.0-alpha.0.261+06ea384605f172',
                'repo_commit': 'v1.20.0-alpha.0.261+06ea384605f172',
                'repos': '{"k8s.io/kubernetes": "master:5a529", "k8s.io/release": "master"}',
                'metadata': {
                    "node_os_image": "cos-81-12871-59-0",
                    "infra-commit": "2a9a0f868",
                    "repo": "k8s.io/kubernetes",
                    "master_os_image": "cos-81-12871-59-0",
                },
            },
        ),
        (
            "Ci_bootstrap",
            "gs://kubernetes-jenkins/pr-logs/path",
            [{'name': "Test1", 'failed': True}],
            {
                "timestamp":1595263104,
                "node":"592473ae-caa7-11ea-b130-525df2b76a8d",
                "repos":{
                    "k8s.io/kubernetes":"master",
                    "k8s.io/release":"master"
                },
                "repo-version":"v1.20.0-alpha.0.255+5feab0aa1e592a",
            },
            {
                "timestamp": 1595263185,
                "version": "v1.20.0-alpha.0.255+5feab0aa1e592a",
                "result": "SUCCESS",
                "passed": True,
                "job-version": "v1.20.0-alpha.0.255+5feab0aa1e592a",
            },
            {
                "repo": "k8s.io/kubernetes",
                "repos": {
                    "k8s.io/kubernetes": "master",
                    "k8s.io/release": "master"
                },
                "infra-commit": "5f39b744b",
                "repo-commit": "5feab0aa1e592ab413b461bc3ad08a6b74a427b4"
            },
            {
                "k8s.io/kubernetes":"master",
                "k8s.io/release":"master"
            },
            {
                'job': 'pr:pr-logs',
                'executor': '592473ae-caa7-11ea-b130-525df2b76a8d',
                'path': 'gs://kubernetes-jenkins/pr-logs/path',
                'test': [{'name': 'Test1', 'failed': True}],
                'passed': True,
                'result': 'SUCCESS',
                'elapsed': 81,
                'tests_run': 1,
                'tests_failed':1,
                'started': 1595263104,
                'finished': 1595263185,
                'version': 'v1.20.0-alpha.0.255+5feab0aa1e592a',
                'repo_commit': 'v1.20.0-alpha.0.255+5feab0aa1e592a',
                'repos': '{"k8s.io/kubernetes": "master", "k8s.io/release": "master"}',
                'metadata': {
                    "repo": "k8s.io/kubernetes",
                    "repos": {
                        "k8s.io/kubernetes": "master",
                        "k8s.io/release": "master"
                    },
                    "infra-commit": "5f39b744b",
                    "repo-commit": "5feab0aa1e592ab413b461bc3ad08a6b74a427b4"
                },
            },
        ),
        (
            "Started_no_meta_repo",
            "gs://kubernetes-jenkins/pr-logs/path",
            [{'name': "Test1", 'failed': False}],
            {
                "timestamp":1595263104,
                "node":"592473ae-caa7-11ea-b130-525df2b76a8d",
                "repos":{
                    "k8s.io/kubernetes":"master",
                    "k8s.io/release":"master"
                },
                "repo-version":"v1.20.0-alpha.0.255+5feab0aa1e592a",
            },
            None,
            None,
            None,
            {
                'job': 'pr:pr-logs',
                'executor': '592473ae-caa7-11ea-b130-525df2b76a8d',
                'path': 'gs://kubernetes-jenkins/pr-logs/path',
                'test': [{'name': 'Test1', 'failed': False}],
                'tests_run': 1,
                'tests_failed':0,
                'repo_commit': 'v1.20.0-alpha.0.255+5feab0aa1e592a',
                'repos': '{"k8s.io/kubernetes": "master", "k8s.io/release": "master"}',
                'started': 1595263104,
            },
        ),
    ])
    def test_gen_build(self, _, path, tests, started, finished, metadata, repos, expected):
        self.maxDiff = None
        build = make_json.Build.generate(path, tests, started, finished, metadata, repos)
        build_dict = build.as_dict()
        self.assertEqual(build_dict, expected)


class MakeJsonTest(unittest.TestCase):
    def setUp(self):
        self.db = model.Database(':memory:')

    def test_path_to_job_and_number(self):
        def expect(path, job, number):
            build = make_json.Build(path, [])
            self.assertEqual((build.job, build.number), (job, number))

        expect('gs://kubernetes-jenkins/logs/some-build/123', 'some-build', 123)
        expect('gs://kubernetes-jenkins/logs/some-build/123asdf', 'some-build', None)
        expect('gs://kubernetes-jenkins/pr-logs/123/e2e-node/456', 'pr:e2e-node', 456)

        with self.assertRaises(ValueError):
            expect('gs://unknown-bucket/foo/123', None, None)
            expect('gs://unknown-bucket/foo/123/', None, None)

    def test_row_for_build(self):
        def expect(path, start, finish, results, **kwargs):
            expected = {
                'path': path,
                'test': [],
                'tests_failed': 0,
                'tests_run': 0,
            }
            if finish:
                expected['passed'] = kwargs.get('result') == 'SUCCESS'
            expected.update(kwargs)
            row = make_json.row_for_build(path, start, finish, results)
            self.assertEqual(row, expected)

        path = 'gs://kubernetes-jenkins/logs/J/123'
        expect(path, None, None, [], job='J', number=123)
        expect(path, None, None, [], job='J', number=123)
        expect(path,
               {'timestamp': 10, 'node': 'agent-34'},
               {'timestamp': 15, 'result': 'SUCCESS', 'version': 'v1.2.3'},
               [],
               job='J', number=123,
               started=10, finished=15, elapsed=5,
               version='v1.2.3', result='SUCCESS', executor='agent-34',
              )
        expect(path,
               {'timestamp': 10},
               {'timestamp': 15, 'passed': True},
               [],
               job='J', number=123,
               started=10, finished=15, elapsed=5,
               result='SUCCESS',
              )
        expect(path, None,
               {'timestamp': 15, 'result': 'FAILURE',
                'metadata': {'repo': 'ignored', 'pull': 'asdf'}}, [],
               result='FAILURE', job='J', number=123, finished=15,
               metadata=[{'key': 'pull', 'value': 'asdf'}, {'key': 'repo', 'value': 'ignored'}])
        expect(path, None, None, ['''
                   <testsuite>
                    <properties><property name="test" value="don't crash!"></property></properties>
                    <testcase name="t1" time="1.0"><failure>stacktrace</failure></testcase>
                    <testcase name="t2" time="2.0"></testcase>
                    <testcase name="t2#1" time="2.0"></testcase>
                   </testsuite>'''],
               job='J', number=123,
               tests_run=2, tests_failed=1,
               test=[{'name': 't1', 'time': 1.0, 'failed': True, 'failure_text': 'stacktrace'},
                     {'name': 't2', 'time': 2.0}])

    def test_main(self):
        now = time.time()
        last_month = now - (60 * 60 * 24 * 30)
        junits = ['<testsuite><testcase name="t1" time="3.0"></testcase></testsuite>']

        def add_build(path, start, finish, result, junits):
            path = 'gs://kubernetes-jenkins/logs/%s' % path
            self.db.insert_build(
                path, {'timestamp': start}, {'timestamp': finish, 'result': result})
            # fake build rowid doesn't matter here
            self.db.insert_build_junits(
                hash(path),
                {'%s/artifacts/junit_%d.xml' % (path, n): junit for n, junit in enumerate(junits)})
            self.db.commit()

        def expect(args, needles, negneedles, expected_ret=None):
            buf = StringIO.StringIO()
            opts = make_json.parse_args(args)
            ret = make_json.main(self.db, opts, buf)
            result = buf.getvalue()

            if expected_ret is not None:
                self.assertEqual(ret, expected_ret)

            # validate that output is newline-delimited JSON
            for line in result.split('\n'):
                if line.strip():
                    json.loads(line)

            # test for expected patterns / expected missing patterns
            for needle in needles:
                self.assertIn(needle, result)
            for needle in negneedles:
                # Only match negative needles in the middle of a word, to avoid
                # failures on timestamps that happen to contain a short number.
                self.assertNotRegexpMatches(result, r'\b%s\b' % needle) # pylint: disable=deprecated-method

        add_build('some-job/123', last_month, last_month + 10, 'SUCCESS', junits)
        add_build('some-job/456', now - 10, now, 'FAILURE', junits)

        expect([], ['123', '456', 'SUCCESS', 'FAILURE'], [])  # everything
        expect([], [], ['123', '456', 'SUCCESS', 'FAILURE'])  # nothing

        expect(['--days=1'], ['456'], [])  # recent
        expect(['--days', '1'], [], ['456'])  # nothing (already emitted)

        add_build('some-job/457', now + 1, now + 11, 'SUCCESS', junits)
        expect(['--days=1'], ['457'], ['456'])  # latest (day)
        expect([], ['457'], ['456'])         # latest (all)

        expect(['--days=1', '--reset-emitted'], ['456', '457'], [])  # both (reset)
        expect([], [], ['123', '456', '457'])                     # reset only works for given day

        # verify that direct paths work
        expect(['gs://kubernetes-jenkins/logs/some-job/123'], ['123'], [])
        expect(['gs://kubernetes-jenkins/logs/some-job/123'], ['123'], [])

        # verify that assert_oldest works
        expect(['--days=30'], ['123', '456'], [])
        expect(['--days=30', '--assert-oldest=60'], [], [], 0)
        expect(['--days=30', '--assert-oldest=25'], [], [], 1)


if __name__ == '__main__':
    unittest.main()
