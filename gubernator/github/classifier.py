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

import logging
import json

import models


def classify_issue(repo, number):
    '''
    Classify an issue in a repo based on events in Datastore.

    Args:
        repo: string
        number: int
    Returns:
        is_pr: bool
        is_open: bool
        involved: list of strings representing usernames involved
        payload: a dict, see full description for classify below.
        last_event_timestamp: the timestamp of the most recent event.
    '''
    ancestor = models.GithubResource.make_key(repo, number)
    events = list(models.GithubWebhookRaw.query(ancestor=ancestor))
    events.sort(key=lambda e: e.timestamp)
    logging.debug('classifying %s %s (%d events)', repo, number, len(events))
    event_pairs = [(event.event, json.loads(event.body)) for event in events]
    return tuple(list(classify(event_pairs)) + [events[-1].timestamp])


def get_merged(events):
    '''
    Determine the most up-to-date view of the issue given its inclusion
    in a series of events.

    Note that different events have different levels of detail-- comments
    don't include head SHA information, pull request events don't have label
    information, etc.

    Args:
        events: a list of (event_type str, event_body dict) pairs.
    Returns:
        body: a dict representing the issue's latest state.
    '''
    merged = {}
    for _event, body in events:
        if 'issue' in body:
            merged.update(body['issue'])
        if 'pull_request' in body:
            merged.update(body['pull_request'])
    return merged


def get_labels(events):
    '''
    Determine the labels applied to an issue.

    Args:
        events: a list of (event_type str, event_body dict) pairs.
    Returns:
        labels: the currently applied labels as {label_name: label_color}
    '''
    labels = []
    for event, body in events:
        if 'issue' in body:
            # issues come with labels, so we can update here
            labels = body['issue']['labels']
        # pull_requests don't include their full labels :(
        action = body.get('action')
        if event == 'pull_request':
            # Pull request label events don't come with a full label set.
            # Track them explicitly here.
            try:
                if action in ('labeled', 'unlabeled') and 'label' not in body:
                    logging.warning('label event with no labels (multiple changes?)')
                elif action == 'labeled':
                    label = body['label']
                    if label not in labels:
                        labels.append(label)
                elif action == 'unlabeled':
                    label = body['label']
                    if label in labels:
                        labels.remove(label)
            except:
                logging.exception('??? %r', body)
                raise
    return {label['name']: label['color'] for label in labels}


def get_comments(events):
    '''
    Pick comments and pull-request review comments out of a list of events.

    Args:
        events: a list of (event_type str, event_body dict) pairs.
    Returns:
        comments: a list of dict(author=..., comment=..., timestamp=...),
                  ordered with the earliest comment first.
    '''
    comments = {}  # comment_id : comment
    for event, body in events:
        action = body.get('action')
        if event in ('issue_comment', 'pull_request_review_comment'):
            comment_id = body['comment']['id']
            if action == 'deleted':
                comments.pop(comment_id, None)
            else:
                comments[comment_id] = body['comment']
    return [
            {
                'author': c['user']['login'],
                'comment': c['body'],
                'timestamp': c['created_at']
            }
            for c in sorted(comments.values(), key=lambda c: c['created_at'])
    ]


def classify(events):
    '''
    Given an event-stream for an issue and status-getter, process
    the events and determine what action should be taken, if any.

    Args:
        events: a list of (event_type str, event_body dict) pairs.
    Returns:
        is_pr: bool
        is_open: bool
        involved: list of strings representing usernames involved
        payload: a dictionary of additional information, including:
            {
                'author': str author_name,
                'title': str issue title,
                'labels': {label_name: label_color},
                'attn': {user_name: reason},
                'mergeable': bool,
                'comments': [{'user': str name, 'comment': comment, 'timestamp': str iso8601}]
            }
    '''
    merged = get_merged(events)
    labels = get_labels(events)
    comments = get_comments(events)

    commit_change_time = None
    for event, body in events:
        if event == 'pull_request':
            if body.get('action') in ('opened', 'reopened', 'synchronize'):
                commit_change_time = body['pull_request']['updated_at']

    is_pr = 'head' in merged or 'pull_request' in merged
    is_open = merged['state'] != 'closed'
    author = merged['user']['login']
    assignees = sorted(assignee['login'] for assignee in merged['assignees'])
    involved = [author] + assignees

    payload = {
        'author': author,
        'assignees': assignees,
        'title': merged['title'],
        'labels': labels,
    }

    if is_pr:
        if is_open:
            payload['needs_rebase'] = 'needs-rebase' in labels or merged.get('mergeable') == 'false'
        payload['additions'] = merged.get('additions', 0)
        payload['deletions'] = merged.get('deletions', 0)

    payload['attn'] = calculate_attention(payload, comments, commit_change_time)

    return is_pr, is_open, involved, payload


def calculate_attention(payload, comments, commit_change_time):
    '''
    Given information about an issue, determine who should look at it
    '''
    author = payload['author']
    assignees = payload['assignees']

    attn = {}
    def notify(to, reason):
        attn.setdefault(to, set()).add(reason)

    if payload.get('needs_rebase'):
        notify(author, 'needs rebase')

    if 'release-note-label-needed' in payload['labels']:
        notify(author, 'needs release-note label')

    if commit_change_time is not None:
        responded_to_changes = set()
        for comment in comments:
            if comment['timestamp'] > commit_change_time:
                responded_to_changes.add(comment['author'])

        for assignee in set(assignees) - responded_to_changes:
            notify(assignee, 'needs review')

    return {user: ', '.join(sorted(msgs)) for user, msgs in attn.iteritems()}
