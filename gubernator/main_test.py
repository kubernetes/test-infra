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


"""
To run these tests:
    $ pip install webtest nosegae
    $ nosetests --with-gae --gae-lib-root ~/google_appengine/
"""

import unittest

import webtest

import cloudstorage as gcs

import main
import gcs_async
import gcs_async_test

write = gcs_async_test.write

app = webtest.TestApp(main.app)

JUNIT_SUITE = """<testsuite tests="8" failures="0" time="1000.24">
    <testcase name="First" classname="Example e2e suite" time="0">
        <skipped/>
    </testcase>
    <testcase name="Second" classname="Example e2e suite" time="36.49"/>
    <testcase name="Third" classname="Example e2e suite" time="96.49">
        <failure>/go/src/k8s.io/kubernetes/test.go:123
Error Goes Here</failure>
    </testcase>
</testsuite>"""


def init_build(build_dir, started=True, finished=True,
               finished_has_version=False):
    """Create faked files for a build."""
    start_json = {'timestamp': 1406535800}
    finish_json = {'passed': True, 'result': 'SUCCESS', 'timestamp': 1406536800}
    (finish_json if finished_has_version else start_json)['revision'] = 'v1+56'
    if started:
        write(build_dir + 'started.json', start_json)
    if finished:
        write(build_dir + 'finished.json', finish_json)
    write(build_dir + 'artifacts/junit_01.xml', JUNIT_SUITE)



class TestBase(unittest.TestCase):
    def init_stubs(self):
        self.testbed.init_memcache_stub()
        self.testbed.init_app_identity_stub()
        self.testbed.init_urlfetch_stub()
        self.testbed.init_blobstore_stub()
        self.testbed.init_datastore_v3_stub()
        self.testbed.init_app_identity_stub()
        # redirect GCS calls to the local proxy
        gcs_async.GCS_API_URL = gcs.common.local_api_url()


class AppTest(TestBase):
    # pylint: disable=too-many-public-methods
    BUILD_DIR = '/kubernetes-jenkins/logs/somejob/1234/'

    def setUp(self):
        self.init_stubs()
        init_build(self.BUILD_DIR)

    def test_index(self):
        """Test that the index works."""
        response = app.get('/')
        self.assertIn('kubernetes-e2e-gce', response)

    def test_nodelog_missing_files(self):
        """Test that a missing all files gives a 404."""
        build_dir = self.BUILD_DIR + 'nodelog?pod=abc'
        response = app.get('/build' + build_dir, status=404)
        self.assertIn('Unable to find', response)

    def test_nodelog_kubelet(self):
        """Test for a kubelet file with junit file.
         - missing the default kube-apiserver"""
        nodelog_url = self.BUILD_DIR + 'nodelog?pod=abc&junit=junit_01.xml'
        init_build(self.BUILD_DIR)
        write(self.BUILD_DIR + 'artifacts/tmp-node-image/junit_01.xml', JUNIT_SUITE)
        write(self.BUILD_DIR + 'artifacts/tmp-node-image/kubelet.log',
            'abc\nEvent(api.ObjectReference{Name:"abc", UID:"podabc"})\n')
        response = app.get('/build' + nodelog_url)
        self.assertIn("Wrap line", response)

    def test_nodelog_apiserver(self):
        """Test for default apiserver file
         - no kubelet file to find objrefdict
         - no file with junit file"""
        nodelog_url = self.BUILD_DIR + 'nodelog?pod=abc&junit=junit_01.xml'
        init_build(self.BUILD_DIR)
        write(self.BUILD_DIR + 'artifacts/tmp-node-image/junit_01.xml', JUNIT_SUITE)
        write(self.BUILD_DIR + 'artifacts/tmp-node-image/kube-apiserver.log',
            'apiserver pod abc\n')
        response = app.get('/build' + nodelog_url)
        self.assertIn("Wrap line", response)

    def test_nodelog_no_junit(self):
        """Test for when no junit in same folder
         - multiple folders"""
        nodelog_url = self.BUILD_DIR + 'nodelog?pod=abc&junit=junit_01.xml'
        init_build(self.BUILD_DIR)
        write(self.BUILD_DIR + 'artifacts/junit_01.xml', JUNIT_SUITE)
        write(self.BUILD_DIR + 'artifacts/tmp-node-image/kube-apiserver.log',
            'apiserver pod abc\n')
        write(self.BUILD_DIR + 'artifacts/tmp-node-2/kube-apiserver.log',
            'apiserver pod abc\n')
        write(self.BUILD_DIR + 'artifacts/tmp-node-image/kubelet.log',
            'abc\nEvent(api.ObjectReference{Name:"abc", UID:"podabc"})\n')
        response = app.get('/build' + nodelog_url)
        self.assertIn("tmp-node-2", response)

    def test_nodelog_no_junit_apiserver(self):
        """Test for when no junit in same folder
         - multiple folders
         - no kube-apiserver.log"""
        nodelog_url = self.BUILD_DIR + 'nodelog?pod=abc&junit=junit_01.xml'
        init_build(self.BUILD_DIR)
        write(self.BUILD_DIR + 'artifacts/junit_01.xml', JUNIT_SUITE)
        write(self.BUILD_DIR + 'artifacts/tmp-node-image/docker.log',
            'Containers\n')
        write(self.BUILD_DIR + 'artifacts/tmp-node-2/kubelet.log',
            'apiserver pod abc\n')
        write(self.BUILD_DIR + 'artifacts/tmp-node-image/kubelet.log',
            'abc\nEvent(api.ObjectReference{Name:"abc", UID:"podabc"})\n')
        response = app.get('/build' + nodelog_url)
        self.assertIn("tmp-node-2", response)

    def test_no_failed_pod(self):
        """Test that filtering page still loads when no failed pod name is given"""
        nodelog_url = self.BUILD_DIR + 'nodelog?junit=junit_01.xml'
        init_build(self.BUILD_DIR)
        write(self.BUILD_DIR + 'artifacts/tmp-node-image/junit_01.xml', JUNIT_SUITE)
        write(self.BUILD_DIR + 'artifacts/tmp-node-image/kubelet.log',
            'abc\nEvent(api.ObjectReference{Name:"abc", UID:"podabc"} failed)\n')
        response = app.get('/build' + nodelog_url)
        self.assertIn("Wrap line", response)

    def test_parse_by_timestamp(self):
        """Test parse_by_timestamp and get_woven_logs
         - Weave separate logs together by timestamp
         - Check that lines without timestamp are combined
         - Test different timestamp formats"""
        kubelet_filepath = self.BUILD_DIR + 'artifacts/tmp-node-image/kubelet.log'
        kubeapi_filepath = self.BUILD_DIR + 'artifacts/tmp-node-image/kube-apiserver.log'
        query_string = 'nodelog?pod=abc&junit=junit_01.xml&weave=on&logfiles=%s&logfiles=%s' % (
            kubelet_filepath, kubeapi_filepath)
        nodelog_url = self.BUILD_DIR + query_string
        init_build(self.BUILD_DIR)
        write(self.BUILD_DIR + 'artifacts/tmp-node-image/junit_01.xml', JUNIT_SUITE)
        write(kubelet_filepath,
            'abc\n0101 01:01:01.001 Event(api.ObjectReference{Name:"abc", UID:"podabc"})\n')
        write(kubeapi_filepath,
            '0101 01:01:01.000 kubeapi\n0101 01:01:01.002 pod\n01-01T01:01:01.005Z last line')
        expected = ('0101 01:01:01.000 kubeapi\n'
                    '<span class="highlight">abc0101 01:01:01.001 Event(api.ObjectReference{Name:'
                    '&#34;<span class="keyword">abc</span>&#34;, UID:&#34;podabc&#34;})</span>\n'
                    '0101 01:01:01.002 pod\n'
                    '01-01T01:01:01.005Z last line')
        response = app.get('/build' + nodelog_url)
        print response
        self.assertIn(expected, response)

    def test_timestamp_no_apiserver(self):
        """Test parse_by_timestamp and get_woven_logs without an apiserver file
         - Weave separate logs together by timestamp
         - Check that lines without timestamp are combined
         - Test different timestamp formats
         - no kube-apiserver.log"""
        kubelet_filepath = self.BUILD_DIR + 'artifacts/tmp-node-image/kubelet.log'
        proxy_filepath = self.BUILD_DIR + 'artifacts/tmp-node-image/kube-proxy.log'
        query_string = 'nodelog?pod=abc&junit=junit_01.xml&weave=on&logfiles=%s&logfiles=%s' % (
            kubelet_filepath, proxy_filepath)
        nodelog_url = self.BUILD_DIR + query_string
        init_build(self.BUILD_DIR)
        write(self.BUILD_DIR + 'artifacts/tmp-node-image/junit_01.xml', JUNIT_SUITE)
        write(kubelet_filepath,
            'abc\n0101 01:01:01.001 Event(api.ObjectReference{Name:"abc", UID:"podabc"})\n')
        write(proxy_filepath,
            '0101 01:01:01.000 proxy\n0101 01:01:01.002 pod\n01-01T01:01:01.005Z last line')
        expected = ('0101 01:01:01.000 proxy\n'
                    '<span class="highlight">abc0101 01:01:01.001 Event(api.ObjectReference{Name:'
                    '&#34;<span class="keyword">abc</span>&#34;, UID:&#34;podabc&#34;})</span>\n'
                    '0101 01:01:01.002 pod\n'
                    '01-01T01:01:01.005Z last line')
        response = app.get('/build' + nodelog_url)
        self.assertIn(expected, response)
