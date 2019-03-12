#!/usr/bin/env python

# Copyright 2018 The Kubernetes Authors.
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

"""Periodically synchronize our Datastore view of PRs with Github.

Various things can cause the local status of a PR to diverge from upstream:
dropped hooks from bugs in the app, upstream GitHub bugs (webhooks aren't
guaranteed!), or a repo *just* starting sending hooks to Gubernator.

Divergent PR state make the PR dashboard less useful, since old PRs accumulate
and clutter out real items, decreasing signal-to-noise ratio and user trust.

To handle these, on a regular schedule we perform a reconciliation step:
- for each repository that we're tracking:
  - A = all open PRs from Datastore
  - B = all open PRs from Github
  - A-B is the set of improperly open PRs. For each PR, add a synthetic
    webhook event to Datastore with state=closed, and reprocess.
  - B-A is the set of improperly closed or missing PRs. Again, inject a
    synthetic webhook with the details received from GitHub and reprocess.

This requires a Github token set like other secrets with /config in the root.
Total token usage is low: number of open PRs / 100 PRs per list call.
As of 2018-01-10, 1666 open PRs in the k8s org translates into ~56 list calls.
"""

import json
import logging
import re

import webapp2

from google.appengine.api import urlfetch
from google.appengine.ext import deferred

import handlers
import models
import secrets

PULL_API = 'https://api.github.com/repos/%s/pulls?state=open&per_page=100'


def get_prs_from_github(token, repo):
    headers = {'Authorization': 'token %s' % token}
    url = PULL_API % repo
    prs = []
    while True:
        logging.info('fetching %s', url)
        response = urlfetch.fetch(url, headers=headers)
        if response.status_code == 404:
            logging.warning('repo was deleted?')
            # Returning no open PRs will make us fake a close event for each of
            # them, which is appropriate.
            return []
        if response.status_code != 200:
            raise urlfetch.Error('status code %s' % response.status_code)
        prs += json.loads(response.content)
        m = re.search(r'<([^>]+)>; rel="next"', response.headers.get('Link', ''))
        if m:
            url = m.group(1)
        else:
            break
    logging.info('pr count: %d, github tokens left: %s',
                 len(prs), response.headers.get('x-ratelimit-remaining'))
    return prs


def inject_event_and_reclassify(repo, number, action, body):
    # this follows similar code as handlers.GithubHandler
    parent = models.GithubResource.make_key(repo, number)
    hook = models.GithubWebhookRaw(
        parent=parent, repo=repo, number=number, event='pull_request',
        body=json.dumps({'action': action, 'pull_request': body}, sort_keys=True))
    hook.put()
    deferred.defer(handlers.update_issue_digest, repo, number)


def sync_repo(token, repo, write_html=None):
    if write_html is None:
        write_html = lambda x: None

    logging.info('syncing repo %s', repo)
    write_html('<h1>%s</h1>' % repo)

    # There is a race condition here:
    # We can't atomically get a list of PRs from the database and GitHub,
    # so a PR might falsely be in stale_open_prs if it is opened after
    # we scan GitHub, or falsely be in missing_prs if a PR is made after we
    # got the list from GitHub, and before we get the list from the database.
    #
    # These cases will both be fixed the next time this code runs, so we don't
    # try to prevent it here.
    prs_gh = get_prs_from_github(token, repo)
    prs_gh_by_number = {pr['number']: pr for pr in prs_gh}

    prs_db = list(models.GHIssueDigest.find_open_prs_for_repo(repo))
    prs_db_by_number = {pr.number: pr for pr in prs_db}

    numbers_datastore = set(prs_db_by_number)
    numbers_github = set(prs_gh_by_number)

    stale_open_prs = sorted(numbers_datastore - numbers_github)
    missing_prs = sorted(numbers_github - numbers_datastore)

    if not stale_open_prs and not missing_prs:
        write_html('matched, no further work needed')
        logging.info('matched, no further work needed')
        return

    logging.info('PRs to close: %s', stale_open_prs)
    logging.info('PRs to open: %s', missing_prs)

    write_html('<br>')
    write_html('PRs that should be closed: %s<br>' % stale_open_prs)

    for number in stale_open_prs:
        pr = prs_db_by_number[number]
        write_html('<b>%d</b><br>%s<br>' % (number, pr))
        inject_event_and_reclassify(repo, number, 'gh-sync-close',
            {'state': 'closed',
             # These other 3 keys are injected because the classifier expects them.
             # This simplifies the testing code, and means we don't have to inject
             # fake webhooks.
             'user': {'login': pr.payload['author']},
             'assignees': [{'login': u} for u in pr.payload['assignees']],
             'title': pr.payload['title']})

    write_html('PRs that should be opened: %s<br>' % missing_prs)

    for number in missing_prs:
        pr = models.shrink(prs_gh_by_number[number])
        write_html('<br>%d</br><pre>%s</pre><br>' %
            (number, json.dumps(pr, indent=4, sort_keys=True)))
        inject_event_and_reclassify(repo, number, 'gh-sync-open', pr)


class PRSync(webapp2.RequestHandler):
    def get(self):
        # This is called automatically by the periodic cron scheduler.
        # For debugging, visit something like /sync?repo=kubernetes/test-infra
        token = secrets.get('github_token', per_host=False)
        if not token:
            logging.warning('no github token, skipping sync')
            self.abort(200)

        # first, determine which repositories we need to sync
        open_prs = list(
            models.GHIssueDigest.find_open_prs().fetch(keys_only=True))
        open_repos = sorted({models.GHIssueDigest(key=pr).repo for pr in open_prs})

        self.response.write('open repos:')
        self.response.write(', '.join(open_repos))

        repo = self.request.get('repo')
        if repo:
            # debugging case
            sync_repo(token, repo, self.response.write)
        else:
            for repo in open_repos:
                deferred.defer(sync_repo, token, repo)


app = webapp2.WSGIApplication([
    (r'/sync', PRSync),
], debug=True)
