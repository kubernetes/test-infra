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

import webapp2

import handlers

class Warmup(webapp2.RequestHandler):
    """Warms up gubernator."""
    def get(self):
        """Receives the warmup request."""
        self.response.headers['Content-Type'] = 'text/plain'
        handlers.make_signature('load the secret!')
        self.response.write('Warmup successful')


app = webapp2.WSGIApplication([
    ('/_ah/warmup', Warmup),
    (r'/webhook', handlers.GithubHandler),
    (r'/events', handlers.Events),
    (r'/status', handlers.Status),
    (r'/timeline', handlers.Timeline),
], debug=True)
