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

import json
import unittest
import urlparse

import cloudstorage as gcs
import webtest

import gcs_async

app = webtest.TestApp(None)


def write(path, data):
    if not isinstance(data, basestring):
        data = json.dumps(data)
    with gcs.open(path, 'w') as f:
        f.write(data)


def install_handler_dispatcher(stub, matches, dispatch):
    def fetch_stub(url, payload, method, headers, request, response,
                   follow_redirects=False, deadline=None,
                   validate_certificate=None):
        # pylint: disable=too-many-arguments,unused-argument
        result, code = dispatch(method, url, payload, headers)
        response.set_statuscode(code)
        response.set_content(result)
        header = response.add_header()
        header.set_key('content-length')
        header.set_value(str(len(result)))

    # this is gross, but there doesn't appear to be a better way
    # pylint: disable=protected-access
    stub._urlmatchers_to_fetch_functions.append((matches, fetch_stub))


def install_handler(stub, structure, base='pr-logs/pull/'):
    """
    Add a stub to mock out GCS JSON API ListObject requests-- with
    just enough detail for our code.

    This is based on google.appengine.ext.cloudstorage.stub_dispatcher.

    Args:
        stub: a URLFetch stub, to register our new handler against.
        structure: a dictionary of {paths: subdirectory names}.
            This will be transformed into the (more verbose) form
            that the ListObject API returns.
    """
    prefixes_for_paths = {}

    for path, subdirs in structure.iteritems():
        path = base + path
        prefixes_for_paths[path] = ['%s%s/' % (path, d) for d in subdirs]

    def matches(url):
        return url.startswith(gcs_async.STORAGE_API_URL)

    def dispatch(method, url, _payload, _headers):
        if method != 'GET':
            raise ValueError('unhandled method %s' % method)
        parsed = urlparse.urlparse(url)
        param_dict = urlparse.parse_qs(parsed.query, True)
        prefix = param_dict['prefix'][0]
        return json.dumps({'prefixes': prefixes_for_paths[prefix]}), 200

    install_handler_dispatcher(stub, matches, dispatch)


class GCSAsyncTest(unittest.TestCase):
    def setUp(self):
        self.testbed.init_memcache_stub()
        self.testbed.init_app_identity_stub()
        self.testbed.init_urlfetch_stub()
        self.testbed.init_blobstore_stub()
        self.testbed.init_datastore_v3_stub()
        self.testbed.init_app_identity_stub()
        # redirect GCS calls to the local proxy
        gcs_async.GCS_API_URL = gcs.common.local_api_url()

    def test_read(self):
        write('/foo/bar', 'test data')
        self.assertEqual(gcs_async.read('/foo/bar').get_result(), 'test data')
        self.assertEqual(gcs_async.read('/foo/quux').get_result(), None)

    def test_listdirs(self):
        install_handler(self.testbed.get_stub('urlfetch'),
            {'foo/': ['bar', 'baz']}, base='base/')
        self.assertEqual(gcs_async.listdirs('buck/base/foo/').get_result(),
            ['buck/base/foo/bar/', 'buck/base/foo/baz/'])
