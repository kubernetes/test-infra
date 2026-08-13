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

import collections
import cPickle as pickle
import logging
import os

import webapp2

from google.appengine.api import urlfetch
from google.appengine.ext import deferred
from google.appengine.ext import ndb

import models
import handlers

# ndb model.query likes to use == True
# pylint: disable=singleton-comparison

class RecomputeOpenPRs(object):
    keys_only = True

    @staticmethod
    def query():
        return models.GHIssueDigest.query(
            models.GHIssueDigest.is_open == True,
            models.GHIssueDigest.is_pr == True
        )

    @staticmethod
    def handle_entity(entity):
        repo, number = entity.id().split(' ')
        handlers.update_issue_digest(repo, number, always_put=True)
        return {'puts': 1}

@ndb.toplevel
def migrate(migration, cursor=None, last_parent=None, stop=False):
    entities, next_cursor, more = migration.query().fetch_page(
        10, start_cursor=cursor, keys_only=migration.keys_only)

    counters = collections.Counter()

    for entity in entities:
        changes = migration.handle_entity(entity)
        counters.update(changes)

    summary = ', '.join('%s: %d' % x for x in sorted(counters.items()))
    if entities:
        logging.info('fetched %d. %s. (%r-%r)',
                     len(entities), summary, entities[0], entities[-1])

    if stop:
        return

    if more and next_cursor:
        deferred.defer(migrate, migration, cursor=next_cursor, last_parent=last_parent)


class Digest(webapp2.RequestHandler):
    def get(self):
        results = models.GHIssueDigest.query(
            models.GHIssueDigest.is_open == True)
        self.response.headers['content-type'] = 'text/plain'
        self.response.write(pickle.dumps(list(results), pickle.HIGHEST_PROTOCOL))


class AdminDash(webapp2.RequestHandler):
    def get(self):
        self.response.write("""
<form action="/admin/reprocess" method="post">
<button>Reprocess Open Issues/PRs</button><input type="checkbox" name="background">Background
</form>
<form action="/admin/digest_sync" method="post">
<button>Download GHIssueDigest from production</button>
</form>
        """)

    def check_csrf(self):
        # https://www.owasp.org/index.php/Cross-Site_Request_Forgery_(CSRF)_Prevention_Cheat_Sheet
        #     #Checking_The_Referer_Header
        origin = self.request.headers.get('origin') + '/'
        expected = self.request.host_url + '/'
        if not (origin and origin == expected):
            logging.error('csrf check failed for %s, origin: %r', self.request.url, origin)
            self.abort(403)


class Reprocessor(AdminDash):
    def post(self):
        self.check_csrf()
        migration = RecomputeOpenPRs()
        if self.request.get('background'):
            deferred.defer(migrate, migration)
            self.response.write('running.')
        else:
            migrate(migration, stop=True)


class DigestSync(AdminDash):
    def post(self):
        if not os.environ['SERVER_SOFTWARE'].startswith('Development/'):
            self.abort(400)
        # For local development, download GHIssueDigests from the production
        # server.
        result = urlfetch.fetch(
            'https://github-dot-k8s-gubernator.appspot.com/digest', deadline=60)
        if result.status_code != 200:
            self.abort(result.status_code)
        body = result.content
        self.response.headers['content-type'] = 'text/plain'
        self.response.write('%s\n' % len(body))
        self.response.write(repr(body[:8]))
        results = pickle.loads(body)
        for res in results:
            res.key = ndb.Key(models.GHIssueDigest, res.key.id())
            self.response.write('%s\n' % res.key)
            res.put()


app = webapp2.WSGIApplication([
    (r'/digest', Digest),
    (r'/admin/?', AdminDash),
    (r'/admin/reprocess', Reprocessor),
    (r'/admin/digest_sync', DigestSync),
], debug=True)
