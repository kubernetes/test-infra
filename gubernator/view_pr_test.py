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

import datetime

from webapp2_extras import securecookie

from github import models
import main_test

app = main_test.app


def make_pr(number, involved, payload, repo='acme/a'):
    payload.setdefault('attn', {})
    payload.setdefault('assignees', [])
    payload.setdefault('author', involved[0])
    payload.setdefault('labels', {})
    digest = models.GHIssueDigest.make(repo, number, is_pr=True, is_open=True,
        involved=involved, payload=payload, updated_at=datetime.datetime.now())
    digest.put()


class TestDashboard(main_test.TestBase):
    def setUp(self):
        app.reset()
        self.init_stubs()

    def test_empty(self):
        resp = app.get('/pr/all')
        self.assertIn('No Results', resp)
        resp = app.get('/pr/nobody')
        self.assertIn('No Results', resp)

    def test_all(self):
        make_pr(12, ['foo'], {'title': 'first'}, 'google/cadvisor')
        make_pr(13, ['bar'], {'title': 'second'}, 'kubernetes/kubernetes')
        resp = app.get('/pr/all')
        self.assertIn('Open Kubernetes PRs', resp)
        self.assertIn('first', resp)
        self.assertIn('second', resp)

    def test_one_entry(self):
        make_pr(123, ['user'], {'attn': {'user': 'fix tests'}})
        resp = app.get('/pr/user')
        self.assertIn('123', resp)

    def test_me(self):
        make_pr(124, ['human'], {'title': 'huge pr!'})

        # no cookie: we get redirected
        resp = app.get('/pr')
        self.assertEqual(resp.status_code, 302)
        self.assertEqual(resp.location, 'http://localhost/github_auth/pr')

        # set the session cookie directly (easier than the full login flow)
        serializer = securecookie.SecureCookieSerializer(
            app.app.config['webapp2_extras.sessions']['secret_key'])
        cookie = serializer.serialize('session', {'user': 'human'})

        # we have a cookie now: we should get results for 'human'
        app.cookies['session'] = cookie
        resp = app.get('/pr', headers={'Cookie': 'session=%s' % cookie})
        self.assertEqual(resp.status_code, 200)
        self.assertIn('huge pr!', resp)
