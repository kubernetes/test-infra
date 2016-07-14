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

# pylint: disable=no-self-use


"""
To run these tests:
    $ pip install webtest nosegae
    $ nosetests --with-gae --gae-lib-root ~/google_appengine/
"""

import json
import unittest

import webtest

import handlers
import main
import models

app = webtest.TestApp(main.app)


class TestBase(unittest.TestCase):
    def init_stubs(self):
        self.testbed.init_memcache_stub()
        self.testbed.init_app_identity_stub()
        self.testbed.init_urlfetch_stub()
        self.testbed.init_blobstore_stub()
        self.testbed.init_datastore_v3_stub()


class AppTest(TestBase):
    def setUp(self):
        self.init_stubs()

    def test_webhook(self):
        body_json = {'action': 'blah'}
        body = json.dumps(body_json)
        signature = handlers.make_signature(body)
        app.post('/webhook', body,
            {'X-Github-Event': 'test',
             'X-Hub-Signature': signature})
        hooks = list(models.GithubWebhookRaw.query())
        self.assertEqual(len(hooks), 1)
        self.assertIsNotNone(hooks[0].timestamp)

    def test_webhook_bad_sig(self):
        body = json.dumps({'action': 'blah'})
        signature = handlers.make_signature(body + 'foo')
        app.post('/webhook', body,
            {'X-Github-Event': 'test',
             'X-Hub-Signature': signature}, status=400)

    def test_webhook_missing_sig(self):
        app.post('/webhook', '{}',
            {'X-Github-Event': 'test'}, status=400)

    def test_webhook_unicode(self):
        body = json.dumps({'action': u'blah\u03BA'})
        signature = handlers.make_signature(body)
        app.post('/webhook', body,
            {'X-Github-Event': 'test',
             'X-Hub-Signature': signature})

    def test_webhook_status(self):
        args = {
            'name': 'owner/repo',
            'sha': '1234',
            'context': 'ci',
            'state': 'success',
            'target_url': 'http://example.com',
            'description': 'passed the tests!',
            'created_at': '2016-07-07T01:58:09Z',
            'updated_at': '2016-07-07T02:03:12Z',
        }
        body = json.dumps(args)
        signature = handlers.make_signature(body)
        app.post('/webhook', body,
            {'X-Github-Event': 'status',
             'X-Hub-Signature': signature})
        statuses = list(models.GHStatus.query_for_sha('owner/repo', '1234'))
        self.assertEqual(len(statuses), 1)
        status = statuses[0]
        args['repo'] = args.pop('name')
        for key, value in args.iteritems():
            status_val = getattr(status, key)
            try:
                status_val = status_val.strftime('%Y-%m-%dT%H:%M:%SZ')
            except AttributeError:
                pass
            assert status_val == value, '%r != %r' % (getattr(status, key), value)
