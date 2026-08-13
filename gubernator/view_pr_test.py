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
import unittest

# TODO(fejta): use non-relative imports
# https://google.github.io/styleguide/pyguide.html?showone=Packages#Packages
import gcs_async_test
from github import models
import main_test
import view_pr

from webapp2_extras import securecookie


app = main_test.app
write = gcs_async_test.write

class PathTest(unittest.TestCase):
    def test_org_repo(self):
        def check(path, org, repo):
            actual_org, actual_repo = view_pr.org_repo(path, 'kubernetes', 'kubernetes')
            self.assertEquals(actual_org, org)
            self.assertEquals(actual_repo, repo)

        check('', 'kubernetes', 'kubernetes')
        check('/test-infra', 'kubernetes', 'test-infra')
        check('/kubernetes', 'kubernetes', 'kubernetes')
        check('/kubernetes/test-infra', 'kubernetes', 'test-infra')
        check('/kubernetes/kubernetes', 'kubernetes', 'kubernetes')
        check('/google/cadvisor', 'google', 'cadvisor')

    def test_pr_path(self):
        def check(org, repo, pr, path):
            actual_path = view_pr.pr_path(org, repo, pr, 'kubernetes', 'kubernetes', 'pull_prefix')
            self.assertEquals(actual_path, '%s/%s' % ('pull_prefix', path))

        check('kubernetes', 'kubernetes', 1234, 1234)
        check('kubernetes', 'kubernetes', 'batch', 'batch')
        check('kubernetes', 'test-infra', 555, 'test-infra/555')
        check('kubernetes', 'test-infra', 'batch', 'test-infra/batch')
        check('google', 'cadvisor', '555', 'google_cadvisor/555')
        check('google', 'cadvisor', 'batch', 'google_cadvisor/batch')


class PRTest(main_test.TestBase):
    BUILDS = {
        'build': [('12', {'version': 'bb', 'timestamp': 1467147654}, None),
                  ('11', {'version': 'bb', 'timestamp': 1467146654},
                   {'result': 'PASSED', 'passed': True}),
                  ('10', {'version': 'aa', 'timestamp': 1467136654},
                   {'result': 'FAILED', 'passed': False})],
        'e2e': [('47', {'version': 'bb', 'timestamp': '1467147654'},
                 {'result': '[UNSET]', 'passed': False}),
                ('46', {'version': 'aa', 'timestamp': '1467136700'},
                 {'result': '[UNSET]', 'passed': False})]
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
                path = '/kubernetes-jenkins/pr-logs/pull/123/%s/%s/' % (job, build)
                if started:
                    write(path + 'started.json', started)
                if finished:
                    write(path + 'finished.json', finished)

    def test_pr_builds(self):
        self.init_pr_directory()
        org, repo = view_pr.org_repo('',
            app.app.config['default_org'],
            app.app.config['default_repo'],
        )
        builds = view_pr.pr_builds(view_pr.pr_path(org, repo, '123',
            app.app.config['default_repo'],
            app.app.config['default_repo'],
            app.app.config['default_external_services']['gcs_pull_prefix'],
        ))
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

    def test_case_insensitive(self):
        "Individual PR pages are case insensitive."
        make_pr(123, ['user'], {'attn': {'User': 'fix tests'}})
        resp = app.get('/pr/UseR')
        self.assertIn('123', resp)
        self.assertIn('Needs Attention (1)', resp)

    def test_milestone(self):
        "Milestone links filter by milestone."
        make_pr(123, ['user'], {'attn': {'User': 'fix tests'}})
        make_pr(124, ['user'], {'attn': {'user': 'fix tests'}, 'milestone': 'v1.24'})
        resp = app.get('/pr/user')
        self.assertIn('v1.24', resp)
        self.assertIn('123', resp)
        self.assertIn('124', resp)
        resp = app.get('/pr/user?milestone=v1.24')
        # Don't match timestamps that happen to include "123".
        self.assertNotRegexpMatches(str(resp), r'\b123\b')
        self.assertIn('124', resp)

    @staticmethod
    def make_session(**kwargs):
        # set the session cookie directly (easier than the full login flow)
        serializer = securecookie.SecureCookieSerializer(
            app.app.config['webapp2_extras.sessions']['secret_key'])
        return serializer.serialize('session', kwargs)

    def test_me(self):
        make_pr(124, ['human'], {'title': 'huge pr!'})

        # no cookie: we get redirected
        resp = app.get('/pr')
        self.assertEqual(resp.status_code, 302)
        self.assertEqual(resp.location, 'http://localhost/github_auth/pr')

        # we have a cookie now: we should get results for 'human'
        cookie = self.make_session(user='human')
        resp = app.get('/pr', headers={'Cookie': 'session=%s' % cookie})
        self.assertEqual(resp.status_code, 200)
        self.assertIn('huge pr!', resp)

    def test_pr_links_user(self):
        "Individual PR pages grab digest information"
        gcs_async_test.install_handler(self.testbed.get_stub('urlfetch'),
            {'12345/': []})
        make_pr(12345, ['human'], {'title': 'huge pr!'})
        resp = app.get('/pr/12345')
        self.assertIn('href="/pr/human"', resp)
        self.assertIn('huge pr!', resp)

    def test_build_links_user(self):
        "Build pages show PR information"
        make_pr(12345, ['human'], {'title': 'huge pr!'})
        build_dir = '/kubernetes-jenkins/pr-logs/pull/12345/e2e/5/'
        write(build_dir + 'started.json', '{}')
        resp = app.get('/build' + build_dir)
        self.assertIn('href="/pr/human"', resp)
        self.assertIn('huge pr!', resp)

    def test_acks(self):
        app.get('/')  # initialize session secrets

        make_pr(124, ['human'], {'title': 'huge pr', 'attn': {'human': 'help#123#456'}}, repo='k/k')
        cookie = self.make_session(user='human')
        headers = {'Cookie': 'session=%s' % cookie}

        def expect_count(count):
            resp = app.get('/pr', headers=headers)
            self.assertEqual(resp.body.count('huge pr'), count)

        # PR should appear twice
        expect_count(2)

        # Ack the PR...
        ack_params = {'command': 'ack', 'repo': 'k/k', 'number': 124, 'latest': 456}
        app.post_json('/pr', ack_params, headers=headers)
        expect_count(1)
        self.assertEqual(view_pr.get_acks('human', []), {'k/k 124': 456})

        # Clear the ack
        app.post_json('/pr', {'command': 'ack-clear'}, headers=headers)
        expect_count(2)
        self.assertEqual(view_pr.get_acks('human', []), {})

        # Ack with an older latest
        ack_params['latest'] = 123
        app.post_json('/pr', ack_params, headers=headers)
        expect_count(2)


if __name__ == '__main__':
    unittest.main()
