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

import datetime

from webapp2_extras import securecookie

import gcs_async_test
from github import models
import main_test
import view_base
import view_pr

app = main_test.app
write = gcs_async_test.write


class PRTest(main_test.TestBase):
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
                path = '/%s/123/%s/%s/' % (view_pr.PR_PREFIX, job, build)
                if started:
                    write(path + 'started.json', started)
                if finished:
                    write(path + 'finished.json', finished)

    def test_pr_builds(self):
        self.init_pr_directory()
        builds = view_pr.pr_builds('123')
        self.assertEqual(builds, self.BUILDS)

    def test_pr_handler(self):
        self.init_pr_directory()
        response = app.get('/pr/123')
        self.assertIn('e2e/47', response)
        self.assertIn('PASSED', response)
        self.assertIn('colspan="3"', response)  # header
        self.assertIn('github.com/kubernetes/kubernetes/pull/123', response)
        self.assertIn('28 20:44', response)

    def test_pr_handler_missing(self):
        gcs_async_test.install_handler(self.testbed.get_stub('urlfetch'),
            {'124/': []})
        response = app.get('/pr/124')
        self.assertIn('No Results', response)

    def test_pr_build_log_redirect(self):
        path = '123/some-job/55/build-log.txt'
        response = app.get('/pr/' + path)
        self.assertEqual(response.status_code, 302)
        self.assertIn('https://storage.googleapis.com', response.location)
        self.assertIn(path, response.location)


def make_pr(number, involved, payload, repo='kubernetes/kubernetes'):
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

    def test_json(self):
        make_pr(12, ['a'], {'title': 'b'}, 'c/d')
        resp = app.get('/pr/all?format=json')
        self.assertEqual(resp.headers['Content-Type'], 'application/json')
        self.assertEqual(len(resp.json), 1)
        pr = resp.json[0]
        self.assertEqual(pr['involved'], ['a'])
        self.assertEqual(pr['number'], 12)
        self.assertEqual(pr['repo'], 'c/d')

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

    def test_pr_links_user(self):
        "Individual PR pages grab digest information"
        make_pr(12345, ['human'], {'title': 'huge pr!'})
        resp = app.get('/pr/12345')
        self.assertIn('href="/pr/human"', resp)
        self.assertIn('huge pr!', resp)

    def test_build_links_user(self):
        "Build pages show PR information"
        make_pr(12345, ['human'], {'title': 'huge pr!'})
        build_dir = '/%s/12345/e2e/5/' % view_base.PR_PREFIX
        write(build_dir + 'started.json', '{}')
        resp = app.get('/build' + build_dir)
        self.assertIn('href="/pr/human"', resp)
        self.assertIn('huge pr!', resp)
