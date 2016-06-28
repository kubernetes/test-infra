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


"""
To run these tests:
    $ pip install webtest nosegae
    $ nosetests --with-gae --gae-lib-root ~/google_appengine/
"""

import json
import os
import unittest
import urlparse

import webtest

import cloudstorage as gcs

import main
import gcs_async
import gcs_async_test

write = gcs_async_test.write

app = webtest.TestApp(main.app)


JUNIT_SUITE = '''<testsuite tests="8" failures="0" time="1000.24">
    <testcase name="First" classname="Example e2e suite" time="0">
        <skipped/>
    </testcase>
    <testcase name="Second" classname="Example e2e suite" time="36.49"/>
    <testcase name="Third" classname="Example e2e suite" time="96.49">
        <failure>/go/src/k8s.io/kubernetes/test.go:123
Error Goes Here</failure>
    </testcase>
</testsuite>'''


def write(path, data):
    if not isinstance(data, str):
        data = json.dumps(data)
    with gcs.open(path, 'w') as f:
        f.write(data)


def init_build(build_dir, started=True, finished=True):
    """Create faked files for a build."""
    if started:
        write(build_dir + 'started.json',
              {'version': 'v1+56', 'timestamp': 1406535800})
    if finished:
        write(build_dir + 'finished.json',
              {'result': 'SUCCESS', 'timestamp': 1406536800})
    write(build_dir + 'artifacts/junit_01.xml', JUNIT_SUITE)


class HelperTest(unittest.TestCase):
    def test_pad_numbers(self):
        self.assertEqual(main.pad_numbers('a3b45'),
                         'a' + '0' * 15 + '3b' + '0' * 14 + '45')


class ParseJunitTest(unittest.TestCase):
    def parse(self, xml):
        return list(main.parse_junit(xml))

    def test_normal(self):
        failures = self.parse(JUNIT_SUITE)
        stack = '/go/src/k8s.io/kubernetes/test.go:123\nError Goes Here'
        self.assertEqual(failures, [('Third', 96.49, stack)])

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
                         [('k8s.io/suite TestBad', 0.1, 'something bad')])

    def test_bad_xml(self):
        self.assertEqual(self.parse('''<body />'''), [])


class TestMixin(object):
    def init_stubs(self):
        self.testbed.init_memcache_stub()
        self.testbed.init_app_identity_stub()
        self.testbed.init_urlfetch_stub()
        self.testbed.init_blobstore_stub()
        self.testbed.init_datastore_v3_stub()
        # redirect GCS calls to the local proxy
        gcs_async.GCS_API_URL = gcs.common.local_api_url()


class AppTest(unittest.TestCase, TestMixin):
    BUILD_DIR = '/kubernetes-jenkins/logs/somejob/1234/'

    def setUp(self):
        self.init_stubs()
        init_build(self.BUILD_DIR)

    def get_build_page(self):
        return app.get('/build' + self.BUILD_DIR)

    def test_index(self):
        """Test that the index works."""
        response = app.get('/')
        self.assertIn('kubernetes-e2e-gce', response)

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

    def test_build_pr_link(self):
        ''' The build page for a PR build links to the PR results.'''
        build_dir = '/%s/123/e2e/567/' % main.PR_PREFIX
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

class PRTest(unittest.TestCase, TestMixin):
    BUILDS = {
        'build': [('12', {'version': 'bb', 'timestamp': 1467147654}, None),
                  ('11', {'version': 'bb', 'timestamp': 1467146654}, {'result': 'PASSED'}),
                  ('10', {'version': 'aa', 'timestamp': 1467136654}, {'result': 'FAILED'})],
        'e2e': [('47', {'version': 'bb', 'timestamp': '1467147654'}, {'result': '[UNSET]'}),
                ('46', {'version': 'aa', 'timestamp': '1467136700'}, {'result': '[UNSET]'})]
    }

    def setUp(self):
        self.init_stubs()

    def init_pr_directory(self):
        gcs_async_test.install_handler(self.testbed.get_stub('urlfetch'),
            {'123/': ['build', 'e2e'],
             '123/build/': ['11', '10', '12'],  # out of order
             '123/e2e/': ['47', '46']})

        for job, builds in self.BUILDS.iteritems():
            for build, started, finished in builds:
                path = '/%s/123/%s/%s/' % (main.PR_PREFIX, job, build)
                if started:
                    write(path + 'started.json', started)
                if finished:
                    write(path + 'finished.json', finished)

    def test_pr_builds(self):
        self.init_pr_directory()
        builds = main.pr_builds('123')
        self.assertEqual(builds, self.BUILDS)

    def test_pr_handler(self):
        self.init_pr_directory()
        response = app.get('/pr/123')
        self.assertIn('e2e/47', response)
        self.assertIn('PASSED', response)
        self.assertIn('colspan="3"', response)  # header
        self.assertIn('github.com/kubernetes/kubernetes/pull/123', response)
        self.assertIn('28 13:44', response)

    def test_pr_handler_missing(self):
        gcs_async_test.install_handler(self.testbed.get_stub('urlfetch'),
            {'124/': []})
        response = app.get('/pr/124')
        self.assertIn('No Results', response)

