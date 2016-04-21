#!/usr/bin/env python
#
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

import webtest
import webapp2

import cloudstorage as gcs

import main

app = webtest.TestApp(main.app)


def init_build(build_dir):
    """Create faked files for a build."""
    def write(path, data):
        if not isinstance(data, str):
            data = json.dumps(data)
        with gcs.open(path, 'w') as f:
            f.write(data)
    write(build_dir + 'started.json',
          {'version': 'v1+56', 'timestamp': 1406535800})
    write(build_dir + 'finished.json',
          {'result': 'SUCCESS', 'timestamp': 1406536800})
    write(build_dir + 'artifacts/junit_01.xml', '''
<testsuite tests="8" failures="0" time="1000.24">
    <testcase name="First" classname="Example e2e suite" time="0">
        <skipped/>
    </testcase>
    <testcase name="Second" classname="Example e2e suite" time="36.49"/>
    <testcase name="Third" classname="Example e2e suite" time="96.49">
        <failure>/go/src/k8s.io/kubernetes/test/example.go:123
Error Goes Here</failure>
    </testcase>
</testsuite>''')


class HelperTest(unittest.TestCase):
    def test_pad_numbers(self):
        self.assertEqual(main.pad_numbers('a3b45'),
                         'a' + '0' * 15 + '3b' + '0' * 14 + '45')


class AppTest(unittest.TestCase):
    BUILD_DIR = '/kubernetes-jenkins/logs/somejob/1234/'

    def setUp(self):
        self.testbed.init_memcache_stub()
        self.testbed.init_app_identity_stub()
        self.testbed.init_urlfetch_stub()
        self.testbed.init_blobstore_stub()
        self.testbed.init_datastore_v3_stub()
        init_build(self.BUILD_DIR)

    def test_index(self):
        """Test that the index works."""
        response = app.get('/')
        self.assertIn('kubernetes-e2e-gce', response)

    def test_missing(self):
        """Test that a missing build gives a 404."""
        response = app.get('/build' + self.BUILD_DIR.replace('1234', '1235'),
                           status=404)

    def test_build(self):
        """Test that the build page works in the happy case."""
        response = app.get('/build' + self.BUILD_DIR)
        self.assertIn('2014-07-28', response)  # started
        self.assertIn('16m40s', response)      # build duration
        self.assertIn('Third', response)       # test name
        self.assertIn('1m36s', response)       # test duration
        self.assertIn('Build Result: SUCCESS', response)
        self.assertIn('Error Goes Here', response)
        self.assertIn('example.go#L123">', response) # stacktrace link works

    def test_build_no_failures(self):
        """Test that builds with no Junit artifacts work."""
        gcs.delete(self.BUILD_DIR + 'artifacts/junit_01.xml')
        response = app.get('/build' + self.BUILD_DIR)
        self.assertIn('No Test Failures', response)

    def test_cache(self):
        """Test that caching works at some level."""
        response = app.get('/build' + self.BUILD_DIR)
        gcs.delete(self.BUILD_DIR + 'started.json')
        gcs.delete(self.BUILD_DIR + 'finished.json')
        response2 = app.get('/build' + self.BUILD_DIR)
        self.assertEqual(str(response), str(response2))

    def test_build_list(self):
        """Test that the job page shows a list of builds."""
        response = app.get('/builds' + os.path.dirname(self.BUILD_DIR[:-1]))
        self.assertIn('/1234/">1234</a>', response)

    def test_job_list(self):
        """Test that the job list shows our job."""
        response = app.get('/jobs/kubernetes-jenkins/logs')
        self.assertIn('somejob/">somejob</a>', response)

