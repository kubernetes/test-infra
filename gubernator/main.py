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

import os
import yaml
import webapp2

from google.appengine.api import users

import github_auth
import view_base
import view_build
import view_logs
import view_pr
import secrets


hostname = secrets.get_hostname()


def get_app_config():
    with open('config.yaml') as config_file:
        return yaml.safe_load(config_file)

config = {
    'webapp2_extras.sessions': {
        'secret_key': None,  # filled in on the first request
        'cookie_args': {
            # we don't have SSL For local development
            'secure': os.getenv('SERVER_SOFTWARE', '').startswith('Google App Engine/'),
            'httponly': True,
        },
    },
    'github_client': None,  # filled in the first time auth is needed
}

config.update(get_app_config())

class Warmup(webapp2.RequestHandler):
    """Warms up gubernator."""
    def get(self):
        """Receives the warmup request."""
        try:
            self.app.config['github_client'] = secrets.get('github_client')
        except KeyError:
            pass  # dev server, generally

        self.response.headers['Content-Type'] = 'text/plain'
        self.response.write('Warmup successful')


class ConfigHandler(view_base.BaseHandler):
    """Handles admin config of gubernator."""
    def get(self):
        self.render('config.html', {'hostname': hostname})

    def post(self):
        self.check_csrf()
        if users.is_current_user_admin():
            oauth_set = False
            webhook_set = False
            token_set = False

            github_id = self.request.get('github_id')
            github_secret = self.request.get('github_secret')
            github_token = self.request.get('github_token')
            github_client_key = 'github_client'
            if self.request.get('github_client_host'):
                # enable custom domains pointed at the same app to have their
                # own github oauth config.
                github_client_key = 'github_client_%s' % \
                    self.request.get('github_client_host')
            if github_id and github_secret:
                value = {'id': github_id, 'secret': github_secret}
                secrets.put(github_client_key, value)
                app.config[github_client_key] = value
                oauth_set = True
            github_webhook_secret = self.request.get('github_webhook_secret')
            if github_webhook_secret:
                secrets.put('github_webhook_secret',
                            github_webhook_secret,
                            per_host=False)
                webhook_set = True
            if github_token:
                secrets.put('github_token', github_token, per_host=False)
                token_set = True
            self.render('config.html',
                        dict(hostname=hostname,
                             oauth_set=oauth_set,
                             webhook_set=webhook_set,
                             token_set=token_set))
        else:
            self.abort(403)


app = webapp2.WSGIApplication([
    ('/_ah/warmup', Warmup),
    (r'/', view_base.IndexHandler),
    (r'/jobs/(.*)$', view_build.JobListHandler),
    (r'/builds/(.*)/([^/]+)/?', view_build.BuildListHandler),
    (r'/build/(.*)/([^/]+)/([-\da-f_:.]+)/?', view_build.BuildHandler),
    (r'/build/(.*)/([^/]+)/([-\da-f_:.]+)/nodelog*', view_logs.NodeLogHandler),
    (r'/pr((?:/[^/]+){0,2})/(\d+|batch)', view_pr.PRHandler),
    (r'/pr/?', view_pr.PRDashboard),
    (r'/pr/([-\w]+)', view_pr.PRDashboard),
    (r'/pr/(.*/build-log.txt)', view_pr.PRBuildLogHandler),
    (r'/github_auth(.*)', github_auth.Endpoint),
    (r'/config', ConfigHandler),
    (r'/gcsproxy', view_build.GcsProxyHandler)
], debug=True, config=config)
