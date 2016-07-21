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

import cgi
import datetime
import hashlib
import hmac
import logging
import json
import traceback

import webapp2

from google.appengine.datastore.datastore_query import Cursor

import classifier
import models


try:
    WEBHOOK_SECRET = open('webhook_secret').read().strip()
except IOError:
    logging.warning('unable to load webhook secret')
    WEBHOOK_SECRET = 'default'

def secure_equal(a, b):
    '''
    Compare a and b without leaking timing information.

    May reveal length of b.
    '''
    if len(a) != len(b):
        return False
    return sum(ord(a[i]) ^ ord(b[i]) for i in xrange(len(a))) == 0


def make_signature(body):
    hmac_instance = hmac.HMAC(WEBHOOK_SECRET, body, hashlib.sha1)
    return 'sha1=' + hmac_instance.hexdigest()


class GithubHandler(webapp2.RequestHandler):
    '''
    Handle POSTs delivered using GitHub's webhook interface. Posts are
    authenticated with HMAC signatures and a shared secret.

    Each event is saved to a database, and can trigger additional
    processing.
    '''
    def post(self):
        event = self.request.headers.get('x-github-event')
        signature = self.request.headers.get('x-hub-signature', '')
        body = self.request.body

        expected_signature = make_signature(body)
        if not secure_equal(signature, expected_signature):
            logging.error('webhook failed signature check')
            self.abort(400)

        body_json = json.loads(body)
        repo = body_json.get('repository', {}).get('full_name')
        number = None
        if 'pull_request' in body_json:
            number = body_json['pull_request']['number']
        elif 'issue' in body_json:
            number = body_json['issue']['number']

        parent = None
        if number:
            parent = models.GithubResource.make_key(repo, number)

        kwargs = {}
        timestamp = self.request.headers.get('x-timestamp')
        if timestamp is not None:
            kwargs['timestamp'] = datetime.datetime.strptime(
                timestamp, '%Y-%m-%d %H:%M:%S.%f')

        webhook = models.GithubWebhookRaw(
            parent=parent,
            repo=repo, number=number, event=event, body=body, **kwargs)
        webhook.put()

        if event == 'status':
            models.save_if_newer(models.GHStatus.from_json(body_json))

        if number is not None:
            update_issue_digest(repo, number)


def update_issue_digest(repo, number, always_put=False):
    digest = models.GHIssueDigest.make(repo, number,
        *classifier.classify_issue(repo, number))
    if always_put:
        digest.put()
    else:
        models.save_if_newer(digest)


class Events(webapp2.RequestHandler):
    '''
    Perform input/output on a series of webhook events from the datastore, for
    debugging purposes.
    '''
    def get(self):
        cursor = Cursor(urlsafe=self.request.get('cursor'))
        repo = self.request.get('repo')
        number = int(self.request.get('number', 0)) or None
        count = int(self.request.get('count', 500))
        if repo is not None and number is not None:
            q = models.GithubWebhookRaw.query(
                models.GithubWebhookRaw.repo == repo,
                models.GithubWebhookRaw.number == number)
        else:
            q = models.GithubWebhookRaw.query()
        q = q.order(models.GithubWebhookRaw.timestamp)
        events, next_cursor, more = q.fetch_page(count, start_cursor=cursor)
        out = []
        for event in events:
            out.append({'repo': event.repo, 'event': event.event,
                        'timestamp': str(event.timestamp),
                        'body': json.loads(event.body)})
        resp = {'next': more and next_cursor.urlsafe(), 'calls': out}
        self.response.headers['content-type'] = 'text/json'
        self.response.write(json.dumps(resp, indent=4, sort_keys=True))


class Status(webapp2.RequestHandler):
    def get(self):
        repo = self.request.get('repo')
        sha = self.request.get('sha')
        if not repo or not sha:
            self.abort(403)
            return
        results = models.GHStatus.query_for_sha(repo, sha)
        self.response.write('<table>')
        for res in results:
            self.response.write('<tr><td>%s<td>%s<td><a href="%s">%s</a>\n' %
                (res.context, res.state, res.target_url, res.description))


def shrink(body):
    '''
    Recursively remove Github API urls from an object, to make it
    more human-readable.
    '''
    toremove = []
    for key, value in body.iteritems():
        if isinstance(value, basestring):
            if key.endswith('url'):
                if (value.startswith('https://api.github.com/') or
                    value.startswith('https://avatars.githubusercontent.com')):
                    toremove.append(key)
        elif isinstance(value, dict):
            shrink(value)
        elif isinstance(value, list):
            for el in value:
                if isinstance(el, dict):
                    shrink(el)
    for key in toremove:
        body.pop(key)


class Timeline(webapp2.RequestHandler):
    '''
    Render all the information in the datastore about a particular issue.

    This is used for debugging and investigations.
    '''
    def emit_classified(self, repo, number):
        try:
            ret = classifier.classify_issue(repo, number)
            self.response.write('<pre>%s</pre>' % cgi.escape(
                repr(ret[:3]) + "\n" + json.dumps(ret[3], indent=2, sort_keys=True)))
            self.__getattribute__esponse.write(len(json.dumps(ret[3])))
        except BaseException:
            self.response.write('<pre>%s</pre>' % traceback.format_exc())

    def emit_events(self, repo, number):
        ancestor = models.GithubResource.make_key(repo, number)
        events = list(models.GithubWebhookRaw.query(ancestor=ancestor))
        events.sort(key=lambda e: e.timestamp)
        self.response.write('<h3>%d Results</h3>' % (len(events)))
        self.response.write('<table border=2>')
        merged = {}
        for event in events:
            body_json = json.loads(event.body)
            shrink(body_json)
            if 'issue' in body_json:
                merged.update(body_json['issue'])
            elif 'pull_request' in body_json:
                merged.update(body_json['pull_request'])
            body = json.dumps(body_json, indent=2)
            action = body_json.get('action')
            sender = body_json.get('sender', {}).get('login')
            self.response.write('<tr><td>%s\n' % '<td>'.join(str(x) for x in
                [event.timestamp, event.event, action, sender,
                 '<pre>' + cgi.escape(body)]))
        return merged

    def get(self):
        self.response.write(
            '<style>td pre{max-height:200px;overflow:scroll}</style>')
        repo = self.request.get('repo')
        number = self.request.get('number')
        self.emit_classified(repo, number)
        merged = self.emit_events(repo, number)
        if 'head' in merged:
            sha = merged['head']['sha']
            results = models.GHStatus.query_for_sha(repo, sha)
            self.response.write('</table><table>')
            for res in results:
                self.response.write('<tr><td>%s<td>%s<td><a href="%s">%s</a>\n'
                   % (res.context, res.state, res.target_url, res.description))
        shrink(merged)
        self.response.write('</table><pre>%s</pre>' % cgi.escape(
            json.dumps(merged, indent=2, sort_keys=True)))
