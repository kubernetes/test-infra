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

    last_event_timestamp = events[-1].timestamp
    merged = get_merged(event_pairs)
    statuses = None
    if 'head' in merged:
        statuses = {}
        for status in models.GHStatus.query_for_sha(repo, merged['head']['sha']):
            last_event_timestamp = max(last_event_timestamp, status.updated_at)
            statuses[status.context] = [
                status.state, status.target_url, status.description]

    return list(classify(event_pairs, statuses)) + [last_event_timestamp]


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


def get_skip_comments(events, skip_users=None):
    '''
    Determine comment ids that should be ignored, either because of
        deletion or because the user should be skipped.

    Args:
        events: a list of (event_type str, event_body dict) pairs.
    Returns:
        comment_ids: a set of comment ids that were deleted or made by
            users that should be skiped.
    '''
    if skip_users is None:
        skip_users = []

    skip_comments = set()
    for event, body in events:
        action = body.get('action')
        if event in ('issue_comment', 'pull_request_review_comment'):
            comment_id = body['comment']['id']
            if action == 'deleted' or body['sender']['login'] in skip_users:
                skip_comments.add(comment_id)
    return skip_comments


def classify(events, statuses=None):
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
        if 'head' in merged:
            payload['head'] = merged['head']['sha']

    if statuses:
        payload['status'] = statuses

    payload['attn'] = calculate_attention(distill_events(events), payload)

    return is_pr, is_open, involved, payload


def distill_events(events):
    '''
    Given a sequence of events, return a series of user-action tuples
    relevant to determining user state.
    '''
    skip_comments = get_skip_comments(events, ['k8s-bot'])

    output = []
    for event, body in events:
        action = body.get('action')
        user = body.get('sender', {}).get('login')
        if event in ('issue_comment', 'pull_request_review_comment'):
            if body['comment']['id'] in skip_comments:
                continue
            if action == 'created':
                output.append(('comment', user))
        if event == 'pull_request':
            if action in ('opened', 'reopened', 'synchronize'):
                output.append(('push', user))
            if action == 'labeled' and 'label' in body:
                output.append(('label ' + body['label']['name'].lower(), user))
    return output


def get_author_state(author, distilled_events):
    '''
    Determine the state of the author given a series of distilled events.
    '''
    state = 'waiting'
    for action, user in distilled_events:
        if state == 'waiting':
            if action == 'comment' and user != author:
                state = 'address comments'
        elif state == 'address comments':
            if action == 'push':
                state = 'waiting'
    return state


def get_assignee_state(assignee, distilled_events):
    '''
    Determine the state of an assignee given a series of distilled events.
    '''
    state = 'needs review'
    for action, user in distilled_events:
        if state == 'needs review':
            if user == assignee:
                if action == 'comment':
                    state = 'waiting'
                if action == 'label lgtm':
                    state = 'waiting'
        elif state == 'waiting':
            if action == 'push':
                state = 'needs review'
    return state


def calculate_attention(distilled_events, payload):
    '''
    Given information about an issue, determine who should look at it.
    '''
    author = payload['author']
    assignees = payload['assignees']

    attn = {}
    def notify(to, reason):
        attn[to] = reason

    if any(state == 'failure' for state, _url, _desc
           in payload.get('status', {}).values()):
        notify(author, 'fix tests')

    for assignee in assignees:
        assignee_state = get_assignee_state(assignee, distilled_events)
        if assignee_state != 'waiting':
            notify(assignee, assignee_state)

    author_state = get_author_state(author, distilled_events)
    if author_state != 'waiting':
        notify(author, author_state)

    if payload.get('needs_rebase'):
        notify(author, 'needs rebase')
    if 'release-note-label-needed' in payload['labels']:
        notify(author, 'needs release-note label')

    return attn
