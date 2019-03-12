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

import cgi
import datetime
import hashlib
import hmac
import logging
import json
import traceback

import webapp2
from webapp2_extras import security

from google.appengine.api.runtime import memory_usage
from google.appengine.datastore import datastore_query
from google.appengine.ext import deferred

import classifier
import models
import secrets


_webhook_secret = None
def get_webhook_secret():
    global _webhook_secret  # pylint: disable=global-statement
    if not _webhook_secret:
        try:
            _webhook_secret = str(secrets.get('github_webhook_secret', per_host=False))
        except KeyError:
            logging.exception('unable to load webhook secret')
    return _webhook_secret


def make_signature(body):
    hmac_instance = hmac.HMAC(get_webhook_secret(), body, hashlib.sha1)
    return 'sha1=' + hmac_instance.hexdigest()


class GithubHandler(webapp2.RequestHandler):
    """
    Handle POSTs delivered using GitHub's webhook interface. Posts are
    authenticated with HMAC signatures and a shared secret.

    Each event is saved to a database, and can trigger additional
    processing.
    """
    def post(self):
        event = self.request.headers.get('x-github-event', '')
        signature = self.request.headers.get('x-hub-signature', '')
        body = self.request.body

        expected_signature = make_signature(body)
        if not security.compare_hashes(signature, expected_signature):
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
            repo=repo,
            number=number,
            event=event,
            guid=self.request.headers.get('x-github-delivery', ''),
            body=body,
            **kwargs
        )
        webhook.put()

        # Defer digest updates, so they'll retry on failure.
        if event == 'status':
            status = models.GHStatus.from_json(body_json)
            models.save_if_newer(status)
            query = models.GHIssueDigest.find_head(repo, status.sha)
            for issue in query.fetch():
                deferred.defer(update_issue_digest, issue.repo, issue.number)

        if number:
            deferred.defer(update_issue_digest, repo, number)


def update_issue_digest(repo, number, always_put=False):
    digest = models.GHIssueDigest.make(repo, number,
        *classifier.classify_issue(repo, number))
    if always_put:
        digest.put()
    else:
        models.save_if_newer(digest)


class BaseHandler(webapp2.RequestHandler):
    def dispatch(self):
        # Eh, this is less work than making all the debug pages escape properly.
        # No resources allowed except for inline CSS, no iframing of content.
        self.response.headers['Content-Security-Policy'] = \
            "default-src none; style-src 'unsafe-inline'; frame-ancestors none"
        super(BaseHandler, self).dispatch()


class Events(BaseHandler):
    """
    Perform input/output on a series of webhook events from the datastore, for
    debugging purposes.
    """
    def get(self):
        cursor = datastore_query.Cursor(urlsafe=self.request.get('cursor'))
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
            out.append({'repo': event.repo,
                        'event': event.event,
                        'guid': event.guid,
                        'timestamp': str(event.timestamp),
                        'body': json.loads(event.body)})
        resp = {'next': more and next_cursor.urlsafe(), 'calls': out}
        self.response.headers['content-type'] = 'text/json'
        self.response.write(json.dumps(resp, indent=4, sort_keys=True))


class Status(BaseHandler):
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


class Timeline(BaseHandler):
    """
    Render all the information in the datastore about a particular issue.

    This is used for debugging and investigations.
    """
    def emit_classified(self, repo, number):
        try:
            self.response.write('<h3>Classifier Output</h3>')
            ret = classifier.classify_issue(repo, number)
            self.response.write('<ul><li>pr: %s<li>open: %s<li>involved: %s'
                % tuple(ret[:3]))
            self.response.write('<li>last_event_timestamp: %s' % ret[4])
            self.response.write('<li>payload len: %d' %len(json.dumps(ret[3])))
            self.response.write('<pre>%s</pre></ul>' % cgi.escape(
                json.dumps(ret[3], indent=2, sort_keys=True)))
        except BaseException:
            self.response.write('<pre>%s</pre>' % traceback.format_exc())

    def emit_events(self, repo, number):
        ancestor = models.GithubResource.make_key(repo, number)
        events = list(models.GithubWebhookRaw.query(ancestor=ancestor)
            .order(models.GithubWebhookRaw.timestamp))

        self.response.write('<h3>Distilled Events</h3>')
        self.response.write('<pre>')
        event_pairs = [event.to_tuple() for event in events]
        for ev in classifier.distill_events(event_pairs):
            self.response.write(cgi.escape('%s, %s %s\n' % ev))
        self.response.write('</pre>')

        self.response.write('<h3>%d Raw Events</h3>' % (len(events)))
        self.response.write('<table border=2>')
        self.response.write('<tr><th>Timestamp<th>Event<th>Action<th>Sender<th>Body</tr>')
        merged = {}
        for event in events:
            body_json = json.loads(event.body)
            models.shrink(body_json)
            if 'issue' in body_json:
                merged.update(body_json['issue'])
            elif 'pull_request' in body_json:
                merged.update(body_json['pull_request'])
            body = json.dumps(body_json, indent=2)
            action = body_json.get('action')
            sender = body_json.get('sender', {}).get('login')
            self.response.write('<tr><td>%s\n' % '<td>'.join(str(x) for x in
                [   # Table columns
                    event.timestamp,
                    '%s<br><code>%s</code>' % (event.event, event.guid),
                    action,
                    sender,
                    '<pre>' + cgi.escape(body)
                ]))
        return merged

    def get(self):
        repo = self.request.get('repo')
        number = self.request.get('number')
        if self.request.get('format') == 'json':
            ancestor = models.GithubResource.make_key(repo, number)
            events = list(models.GithubWebhookRaw.query(ancestor=ancestor))
            self.response.headers['content-type'] = 'application/json'
            self.response.write(json.dumps([e.body for e in events], indent=True))
            return
        self.response.write(
            '<style>td pre{max-height:200px;max-width:800px;overflow:scroll}</style>')
        self.response.write('<p>Memory: %s' % memory_usage().current())
        self.emit_classified(repo, number)
        self.response.write('<p>Memory: %s' % memory_usage().current())
        if self.request.get('classify_only'):
            return
        merged = self.emit_events(repo, number)
        self.response.write('<p>Memory: %s' % memory_usage().current())
        if 'head' in merged:
            sha = merged['head']['sha']
            results = models.GHStatus.query_for_sha(repo, sha)
            self.response.write('</table><table>')
            for res in results:
                self.response.write('<tr><td>%s<td>%s<td><a href="%s">%s</a>\n'
                   % (res.context, res.state, res.target_url, res.description))
        models.shrink(merged)
        self.response.write('</table><pre>%s</pre>' % cgi.escape(
            json.dumps(merged, indent=2, sort_keys=True)))
        self.response.write('<p>Memory: %s' % memory_usage().current())
