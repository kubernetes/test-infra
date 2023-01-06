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

# pylint: disable=no-self-use


"""
To run these tests:
    $ pip install webtest nosegae
    $ nosetests --with-gae --gae-lib-root ~/google_appengine/
"""

import json
import unittest

import webtest

from google.appengine.ext import deferred
from google.appengine.ext import testbed

import handlers
import main
import models
import secrets

app = webtest.TestApp(main.app)


class TestBase(unittest.TestCase):
    def init_stubs(self):
        self.testbed.init_memcache_stub()
        self.testbed.init_app_identity_stub()
        self.testbed.init_urlfetch_stub()
        self.testbed.init_blobstore_stub()
        self.testbed.init_datastore_v3_stub()
        self.testbed.init_taskqueue_stub()


class AppTest(TestBase):
    def setUp(self):
        self.init_stubs()
        self.taskqueue = self.testbed.get_stub(testbed.TASKQUEUE_SERVICE_NAME)
        secrets.put('github_webhook_secret', 'some_secret', per_host=False)

    def get_response(self, event, body):
        if isinstance(body, dict):
            body = json.dumps(body)
        signature = handlers.make_signature(body)
        resp = app.post('/webhook', body,
            {'X-Github-Event': event,
             'X-Hub-Signature': signature})
        for task in self.taskqueue.get_filtered_tasks():
            deferred.run(task.payload)
        return resp

    def test_webhook(self):
        self.get_response('test', {'action': 'blah'})
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
        self.get_response('test', {'action': u'blah\u03BA'})

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
        self.get_response('status', args)
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

    PR_EVENT_BODY = {
        'repository': {'full_name': 'test/test'},
        'pull_request': {
            'number': 123,
            'head': {'sha': 'cafe'},
            'updated_at': '2016-07-07T02:03:12+00:00',
            'state': 'open',
            'user': {'login': 'rmmh'},
            'assignees': [{'login': 'spxtr'}],
            'title': 'test pr',
        },
        'action': 'opened',
    }

    def test_webhook_pr_open(self):
        body = json.dumps(self.PR_EVENT_BODY)
        self.get_response('pull_request', body)
        digest = models.GHIssueDigest.get('test/test', 123)
        self.assertTrue(digest.is_pr)
        self.assertTrue(digest.is_open)
        self.assertEqual(digest.involved, ['rmmh', 'spxtr'])
        self.assertEqual(digest.payload['title'], 'test pr')
        self.assertEqual(digest.payload['needs_rebase'], False)

    def test_webhook_pr_open_and_status(self):
        self.get_response('pull_request', self.PR_EVENT_BODY)
        self.get_response('status', {
            'repository': self.PR_EVENT_BODY['repository'],
            'name': self.PR_EVENT_BODY['repository']['full_name'],
            'sha': self.PR_EVENT_BODY['pull_request']['head']['sha'],
            'context': 'test-ci',
            'state': 'success',
            'target_url': 'example.com',
            'description': 'woop!',
            'created_at': '2016-07-07T01:58:09Z',
            'updated_at': '2016-07-07T02:03:15Z',
        })
        digest = models.GHIssueDigest.get('test/test', 123)
        self.assertEqual(digest.payload['status'],
            {'test-ci': ['success', 'example.com', 'woop!']})
