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

import json
import logging
import os

import yaml
import webapp2
from webapp2_extras import security

from google.appengine.api import app_identity
from google.appengine.api import modules

import github_auth
import view_base
import view_build
import view_logs
import view_pr


hostname = app_identity.get_default_version_hostname()
if 'testbed' not in os.environ.get('SERVER_SOFTWARE', 'testbed'):
    current_version = modules.modules.get_current_version_name()
    if modules.modules.get_default_version() != current_version:
        hostname = '%s-dot-%s' % (current_version, hostname)

def get_secret(key):
    data = json.load(open('secrets.json'))
    return data[hostname][key]


def get_session_secret():
    try:
        return str(get_secret('session'))
    except (IOError, KeyError):
        if hostname:  # no scary error messages when testing
            logging.error(
                'unable to load secret key! sessions WILL NOT persist!')
        # This fallback is enough for testing, but in production
        # will end up invalidating sessions whenever a different instance
        # is created.
        return security.generate_random_string(entropy=256)


def get_github_client():
    try:
        return get_secret('github_client')
    except (IOError, KeyError):
        if hostname:
            logging.warning('no github client id found')
        return None


def get_app_config():
    with open('config.yaml') as config_file:
        return yaml.load(config_file)

config = {
    'webapp2_extras.sessions': {
        'secret_key': get_session_secret(),
        'cookie_args': {
            # we don't have SSL For local development
            'secure': hostname and 'appspot.com' in hostname,
            'httponly': True,
        },
    },
    'github_client': get_github_client(),
}

config.update(get_app_config())

class Warmup(webapp2.RequestHandler):
    """Warms up gubernator."""
    def get(self):
        """Receives the warmup request."""
        # TODO(fejta): warmup something useful
        self.response.headers['Content-Type'] = 'text/plain'
        self.response.write('Warmup successful')

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
    (r'/github_auth(.*)', github_auth.Endpoint)
], debug=True, config=config)
