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
import logging
import urllib

from webapp2_extras import security

from google.appengine.api import urlfetch

import secrets
import view_base


class Endpoint(view_base.BaseHandler):
    def github_client(self):
        client_key = 'github_client'
        if '.appspot.com' not in self.request.host and \
            not self.request.host.startswith('localhost:'):
            client_key = 'github_client_' + self.request.host
        if not self.app.config.get(client_key):
            try:
                self.app.config[client_key] = secrets.get(client_key)
            except KeyError:
                self.abort(500,
                           body_template=(
                           'An admin must <a href="/config">'
                           'configure Github secrets</a> for %r first.'
                           % self.request.host))
        client = self.app.config[client_key]
        return client['id'], client['secret']

    def maybe_redirect(self, target):
        """
        Redirect to a given URL if it's determined to be safe.
        """
        if target.startswith('/pr'):
            self.redirect(target)

    def get(self, arg):
        # Documentation here: https://developer.github.com/v3/oauth/
        client_id, client_secret = self.github_client()

        if arg.endswith('/done'):
            target, done = arg[:-len('/done')], True
        else:
            target, done = arg, False

        if not done:
            # 1) Redirect users to request GitHub access
            if self.session.get('user'):
                # User already logged in, no need to continue.
                self.maybe_redirect(target)
                return

            state = security.generate_random_string(entropy=128)
            args = {
                'client_id': client_id,
                'redirect_uri': self.request.url + '/done',
                'scope': '',  # Username only needs "public data" permission!
                'state': state
            }
            self.session['gh_state'] = state
            self.redirect('https://github.com/login/oauth/authorize?'
                + urllib.urlencode(args))
        else:
            # 2) GitHub redirects back to your site
            code = self.request.get('code')
            state = self.request.get('state')
            session_state = self.session.pop('gh_state', '')

            if not state or not code:
                self.abort(400)
            if not security.compare_hashes(state, session_state):
                # States must match to avoid CSRF.
                # Full attack details here:
                # http://homakov.blogspot.com/2012/07/saferweb-most-common-oauth2.html
                self.abort(400)

            # 3) Use the access token to access the API
            params = {
                'client_id': client_id,
                'client_secret': client_secret,
                'code': code,
                'state': session_state,
            }
            resp = urlfetch.fetch(
                'https://github.com/login/oauth/access_token',
                payload=urllib.urlencode(params),
                method='POST',
                headers={'Accept': 'application/json'}
            )
            if resp.status_code != 200:
                logging.error('failed to vend token: status %d', resp.status_code)
                self.abort(500)
            vended = json.loads(resp.content)

            resp = urlfetch.fetch(
                'https://api.github.com/user',
                headers={'Authorization': 'token ' + vended['access_token']})

            if resp.status_code != 200:
                logging.error('failed to get user name: status %d', resp.status_code)
                self.abort(500)

            user_info = json.loads(resp.content)
            login = user_info['login']
            logging.info('successful login for %s', login)

            # Save the GitHub username to the session.
            # Note: we intentionally discard the access_token here,
            # since we don't need it for anything more.
            self.session['user'] = login
            self.response.write('<h1>Welcome, %s</h1>' % login)

            self.maybe_redirect(target)
