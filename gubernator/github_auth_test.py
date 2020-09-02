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

import unittest
import urlparse

import webtest

import gcs_async_test
import main

CLIENT_ID = '12345'
CLIENT_SECRET = 'swordfish'
GH_LOGIN_CODE = 'somerandomcode'

main.app.config['github_client'] = {
    'id': CLIENT_ID,
    'secret': CLIENT_SECRET,
}
main.app.config['webapp2_extras.sessions']['secret_key'] = 'abcd'

app = webtest.TestApp(main.app)

VEND_URL = 'https://github.com/login/oauth/access_token'
USER_URL = 'https://api.github.com/user'

class TestGithubAuth(unittest.TestCase):
    def setUp(self):
        app.reset()
        self.testbed.init_app_identity_stub()
        self.testbed.init_urlfetch_stub()
        self.calls = []
        self.results = {
            VEND_URL: ('{"access_token": "token"}', 200),
            USER_URL: ('{"login": "foo"}', 200),
        }
        gcs_async_test.install_handler_dispatcher(
            self.testbed.get_stub('urlfetch'),
            (lambda url: url in self.results),
            self.dispatcher)

    def dispatcher(self, method, url, payload, headers):
        self.calls.append([method, url, payload, headers])
        return self.results[url]

    @staticmethod
    def do_phase1(arg=''):
        return app.get('/github_auth' + arg)

    @staticmethod
    def parse_phase1(phase1):
        parsed = urlparse.urlparse(phase1.location)
        query = urlparse.parse_qs(parsed.query)
        state = query.pop('state')[0]
        return state, query

    def do_phase2(self, phase1=None, status=None):
        if not phase1:
            phase1 = self.do_phase1()
        state, query = self.parse_phase1(phase1)
        code = GH_LOGIN_CODE
        return app.get(
            query['redirect_uri'][0],
            {'code': code, 'state': state},
            status=status)

    def test_login_works(self):
        "oauth login works"
        # 1) Redirect to github
        resp = self.do_phase1()
        self.assertEqual(resp.status_code, 302)
        loc = resp.location
        assert loc.startswith('https://github.com/login/oauth/authorize'), loc
        state, query = self.parse_phase1(resp)
        self.assertEqual(query, {
            'redirect_uri': ['http://localhost/github_auth/done'],
            'client_id': [CLIENT_ID]})

        # 2) Github redirects back
        resp = self.do_phase2(resp)
        self.assertIn('Welcome, foo', resp)

        # Test that we received the right calls to our fake API.
        self.assertEqual(len(self.calls), 2)

        vend_call = self.calls[0]
        user_call = self.calls[1]

        self.assertEqual(vend_call[:2], ['POST', VEND_URL])
        self.assertEqual(user_call[:3], ['GET', USER_URL, None])

        self.assertEqual(
            urlparse.parse_qs(vend_call[2]),
            dict(client_secret=[CLIENT_SECRET], state=[state],
                 code=[GH_LOGIN_CODE], client_id=[CLIENT_ID]))
        vend_headers = {h.key(): h.value() for h in vend_call[3]}
        self.assertEqual(vend_headers, {'Accept': 'application/json'})

    def test_redirect_pr(self):
        "login can redirect to another page at the end"
        phase1 = self.do_phase1('/pr')
        phase2 = self.do_phase2(phase1)
        self.assertEqual(phase2.status_code, 302)
        self.assertEqual(phase2.location, 'http://localhost/pr')

    def test_redirect_ignored(self):
        "login only redirects to allowed URLs"
        phase1 = self.do_phase1('/bad/redirect')
        phase2 = self.do_phase2(phase1)
        self.assertEqual(phase2.status_code, 200)

    def test_phase2_missing_cookie(self):
        "missing cookie for phase2 fails (CSRF)"
        phase1 = self.do_phase1()
        app.reset()  # clears cookies
        self.do_phase2(phase1, status=400)

    def test_phase2_mismatched_state(self):
        "wrong state for phase2 fails (CSRF)"
        phase1 = self.do_phase1()
        phase1.location = phase1.location.replace('state=', 'state=NOPE')
        self.do_phase2(phase1, status=400)

    def test_phase2_vend_failure(self):
        "GitHub API error vending tokens raises 500"
        self.results[VEND_URL] = ('', 403)
        self.do_phase2(status=500)

    def test_phase2_user_failure(self):
        "GitHub API error getting user information raises 500"
        self.results[USER_URL] = ('', 403)
        self.do_phase2(status=500)
