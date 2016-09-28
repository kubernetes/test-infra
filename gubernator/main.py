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

import webapp2
from webapp2_extras import security

from google.appengine.api import app_identity

import github_auth
import view_base
import view_build
import view_logs
import view_pr


hostname = app_identity.get_default_version_hostname()


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
        return security.generate_random_string(entropy=256)


def get_github_client():
    try:
        return get_secret('github_client')
    except (IOError, KeyError):
        if hostname:
            logging.warning('no github client id found')
        return None


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


app = webapp2.WSGIApplication([
    (r'/', view_base.IndexHandler),
    (r'/jobs/(.*)$', view_build.JobListHandler),
    (r'/builds/(.*)/([^/]+)/?', view_build.BuildListHandler),
    (r'/build/(.*)/([^/]+)/([\d_:.]+)/?', view_build.BuildHandler),
    (r'/build/(.*)/([^/]+)/([\d_:.]+)/nodelog*', view_logs.NodeLogHandler),
    (r'/pr/([^/]*/)?(\d+)', view_pr.PRHandler),
    (r'/pr/?', view_pr.PRDashboard),
    (r'/pr/([-\w]+)', view_pr.PRDashboard),
    (r'/pr/(.*/build-log.txt)', view_pr.PRBuildLogHandler),
    (r'/github_auth(.*)', github_auth.Endpoint)
], debug=True, config=config)
