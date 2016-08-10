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

import os
import unittest

import cloudstorage as gcs

import view_build

import main_test
import gcs_async_test
import view_pr

app = main_test.app
init_build = main_test.init_build
write = gcs_async_test.write


class ParseJunitTest(unittest.TestCase):
    @staticmethod
    def parse(xml):
        return list(view_build.parse_junit(xml, "fp"))

    def test_normal(self):
        failures = self.parse(main_test.JUNIT_SUITE)
        stack = '/go/src/k8s.io/kubernetes/test.go:123\nError Goes Here'
        self.assertEqual(failures, [('Third', 96.49, stack, "fp")])

    def test_testsuites(self):
        failures = self.parse('''
            <testsuites>
                <testsuite name="k8s.io/suite">
                    <properties>
                        <property name="go.version" value="go1.6"/>
                    </properties>
                    <testcase name="TestBad" time="0.1">
                        <failure>something bad</failure>
                    </testcase>
                </testsuite>
            </testsuites>''')
        self.assertEqual(failures,
                         [('k8s.io/suite TestBad', 0.1, 'something bad', "fp")])

    def test_bad_xml(self):
        self.assertEqual(self.parse('''<body />'''), [])


class BuildTest(main_test.TestBase):
    BUILD_DIR = '/kubernetes-jenkins/logs/somejob/1234/'

    def setUp(self):
        self.init_stubs()
        init_build(self.BUILD_DIR)

    def get_build_page(self):
        return app.get('/build' + self.BUILD_DIR)

    def test_missing(self):
        """Test that a missing build gives a 404."""
        response = app.get('/build' + self.BUILD_DIR.replace('1234', '1235'),
                           status=404)
        self.assertIn('1235', response)

    def test_missing_started(self):
        """Test that a missing started.json still renders a proper page."""
        build_dir = '/kubernetes-jenkins/logs/job-with-no-started/1234/'
        init_build(build_dir, started=False)
        response = app.get('/build' + build_dir)
        self.assertIn('Build Result: SUCCESS', response)
        self.assertIn('job-with-no-started', response)
        self.assertNotIn('Started', response)  # no start timestamp
        self.assertNotIn('github.com', response)  # no version => no src links

    def test_missing_finished(self):
        """Test that a missing finished.json still renders a proper page."""
        build_dir = '/kubernetes-jenkins/logs/job-still-running/1234/'
        init_build(build_dir, finished=False)
        response = app.get('/build' + build_dir)
        self.assertIn('Build Result: Not Finished', response)
        self.assertIn('job-still-running', response)
        self.assertIn('Started', response)

    def test_build(self):
        """Test that the build page works in the happy case."""
        response = self.get_build_page()
        self.assertIn('2014-07-28', response)  # started
        self.assertIn('16m40s', response)      # build duration
        self.assertIn('Third', response)       # test name
        self.assertIn('1m36s', response)       # test duration
        self.assertIn('Build Result: SUCCESS', response)
        self.assertIn('Error Goes Here', response)
        self.assertIn('test.go#L123">', response)  # stacktrace link works

    def test_build_no_failures(self):
        """Test that builds with no Junit artifacts work."""
        gcs.delete(self.BUILD_DIR + 'artifacts/junit_01.xml')
        response = self.get_build_page()
        self.assertIn('No Test Failures', response)

    def test_build_show_log(self):
        """Test that builds that failed with no failures show the build log."""
        gcs.delete(self.BUILD_DIR + 'artifacts/junit_01.xml')
        write(self.BUILD_DIR + 'finished.json',
              {'result': 'FAILURE', 'timestamp': 1406536800})

        # Unable to fetch build-log.txt, still works.
        response = self.get_build_page()
        self.assertNotIn('Error lines', response)

        self.testbed.init_memcache_stub()  # clear cached result
        write(self.BUILD_DIR + 'build-log.txt',
              u'ERROR: test \u039A\n\n\n\n\n\n\n\n\nblah'.encode('utf8'))
        response = self.get_build_page()
        self.assertIn('Error lines', response)
        self.assertIn('No Test Failures', response)
        self.assertIn('ERROR</span>: test', response)
        self.assertNotIn('blah', response)

    def test_build_failure_no_text(self):
        # Some failures don't have any associated text.
        write(self.BUILD_DIR + 'artifacts/junit_01.xml', '''
            <testsuites>
                <testsuite tests="1" failures="1" time="3.274" name="k8s.io/test/integration">
                    <testcase classname="integration" name="TestUnschedulableNodes" time="0.210">
                        <failure message="Failed" type=""/>
                    </testcase>
                </testsuite>
            </testsuites>''')
        response = self.get_build_page()
        self.assertIn('TestUnschedulableNodes', response)
        self.assertIn('junit_01.xml', response)

    def test_build_pr_link(self):
        ''' The build page for a PR build links to the PR results.'''
        build_dir = '/%s/123/e2e/567/' % view_pr.PR_PREFIX
        init_build(build_dir)
        response = app.get('/build' + build_dir)
        self.assertIn('PR #123', response)
        self.assertIn('href="/pr/123"', response)

    def test_cache(self):
        """Test that caching works at some level."""
        response = self.get_build_page()
        gcs.delete(self.BUILD_DIR + 'started.json')
        gcs.delete(self.BUILD_DIR + 'finished.json')
        response2 = self.get_build_page()
        self.assertEqual(str(response), str(response2))

    def test_build_list(self):
        """Test that the job page shows a list of builds."""
        response = app.get('/builds' + os.path.dirname(self.BUILD_DIR[:-1]))
        self.assertIn('/1234/">1234</a>', response)

    def test_job_list(self):
        """Test that the job list shows our job."""
        response = app.get('/jobs/kubernetes-jenkins/logs')
        self.assertIn('somejob/">somejob</a>', response)
