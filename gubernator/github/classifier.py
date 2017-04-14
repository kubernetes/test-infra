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

import datetime
import logging
import re

import google.appengine.ext.ndb as ndb

import models


XREF_RE = re.compile(r'k8s-gubernator.appspot.com/build(/[^])\s]+/\d+)')
APPROVERS_RE = re.compile(r'<!-- META={"?approvers"?:\[([^]]*)\]} -->')


class Deduper(object):
    ''' A memory-saving string deduplicator for Python datastructures.

    This is somewhat like the built-in intern() function, but without pinning memory
    permanently.

    Tries to reduce memory usage by making equivalent strings point at the same object.
    This reduces memory usage for large, repetitive JSON structures by >2x.
    '''

    def __init__(self):
        self.strings = {}

    def dedup(self, obj):
        if isinstance(obj, basestring):
            return self.strings.setdefault(obj, obj)
        elif isinstance(obj, dict):
            return {self.dedup(k): self.dedup(v) for k, v in obj.iteritems()}
        elif isinstance(obj, tuple):
            return tuple(self.dedup(x) for x in obj)
        elif isinstance(obj, list):
            return [self.dedup(x) for x in obj]
        return obj


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
    logging.debug('finding webhooks for %s %s', repo, number)
    event_keys = list(models.GithubWebhookRaw.query(ancestor=ancestor).fetch(keys_only=True))

    logging.debug('classifying %s %s (%d events)', repo, number, len(event_keys))
    event_tuples = []
    last_event_timestamp = datetime.datetime(2000, 1, 1)


    if len(event_keys) > 800:
        logging.warning('too many events. blackholing.')
        return False, False, [], {'num_events': len(event_keys)}, last_event_timestamp

    deduper = Deduper()

    for x in xrange(0, len(event_keys), 100):
        events = ndb.get_multi(event_keys[x:x+100])
        last_event_timestamp = max(last_event_timestamp, max(e.timestamp for e in events))
        event_tuples.extend([deduper.dedup(event.to_tuple()) for event in events])

    event_tuples.sort(key=lambda x: x[2])  # sort by timestamp

    del deduper  # attempt to save memory
    del events

    merged = get_merged(event_tuples)
    statuses = None
    if 'head' in merged:
        statuses = {}
        for status in models.GHStatus.query_for_sha(repo, merged['head']['sha']):
            last_event_timestamp = max(last_event_timestamp, status.updated_at)
            statuses[status.context] = [
                status.state, status.target_url, status.description]

    return list(classify(event_tuples, statuses)) + [last_event_timestamp]


def get_merged(events):
    '''
    Determine the most up-to-date view of the issue given its inclusion
    in a series of events.

    Note that different events have different levels of detail-- comments
    don't include head SHA information, pull request events don't have label
    information, etc.

    Args:
        events: a list of (event_type str, event_body dict, timestamp).
    Returns:
        body: a dict representing the issue's latest state.
    '''
    merged = {}
    for _event, body, _timestamp in events:
        if 'issue' in body:
            merged.update(body['issue'])
        if 'pull_request' in body:
            merged.update(body['pull_request'])
    return merged


def get_labels(events):
    '''
    Determine the labels applied to an issue.

    Args:
        events: a list of (event_type str, event_body dict, timestamp).
    Returns:
        labels: the currently applied labels as {label_name: label_color}
    '''
    labels = []
    for event, body, _timestamp in events:
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
        events: a list of (event_type str, event_body dict, timestamp).
    Returns:
        comment_ids: a set of comment ids that were deleted or made by
            users that should be skiped.
    '''
    if skip_users is None:
        skip_users = []

    skip_comments = set()
    for event, body, _timestamp in events:
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
        events: a list of (event_type str, event_body dict, timestamp).
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
                'comments': [{'user': str name, 'comment': comment, 'timestamp': str iso8601}],
                'xrefs': list of builds referenced (by GCS path),
            }
    '''
    merged = get_merged(events)
    labels = get_labels(events)
    comments = get_comments(events)
    xrefs = get_xrefs(comments, merged)
    approvers = get_approvers(comments)
    reviewers = get_reviewers(events)

    is_pr = 'head' in merged or 'pull_request' in merged
    is_open = merged['state'] != 'closed'
    author = merged['user']['login']
    assignees = sorted({assignee['login'] for assignee in merged['assignees']} | reviewers)
    involved = sorted(set([author] + assignees + approvers))

    payload = {
        'author': author,
        'assignees': assignees,
        'title': merged['title'],
        'labels': labels,
        'xrefs': xrefs,
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

    if approvers:
        payload['approvers'] = approvers

    payload['attn'] = calculate_attention(distill_events(events), payload)

    return is_pr, is_open, involved, payload


def get_xrefs(comments, merged):
    xrefs = set(XREF_RE.findall(merged.get('body') or ''))
    for c in comments:
        xrefs.update(XREF_RE.findall(c['comment']))
    return sorted(xrefs)


def get_comments(events):
    '''
    Pick comments and pull-request review comments out of a list of events.
    Args:
        events: a list of (event_type str, event_body dict, timestamp).
    Returns:
        comments: a list of dict(author=..., comment=..., timestamp=...),
                  ordered with the earliest comment first.
    '''
    comments = {}  # comment_id : comment
    for event, body, _timestamp in events:
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


def get_reviewers(events):
    '''
    Return the set of users that have a code review requested or completed.
    '''
    reviewers = set()
    for event, body, _timestamp in events:
        action = body.get('action')
        if event == 'pull_request':
            if action == 'review_requested':
                if 'requested_reviewer' not in body:
                    logging.warning('no reviewer present -- self-review?')
                    continue
                reviewers.add(body['requested_reviewer']['login'])
            elif action == 'review_request_removed':
                reviewers -= {body['requested_reviewer']['login']}
    return reviewers


def get_approvers(comments):
    '''
    Return approvers requested in comments.

    This MUST be kept in sync with mungegithub's getGubernatorMetadata().
    '''
    approvers = []
    for comment in comments:
        if comment['author'] == 'k8s-merge-robot':
            m = APPROVERS_RE.search(comment['comment'])
            if m:
                approvers = m.group(1).replace('"', '').split(',')
    return approvers


def distill_events(events):
    '''
    Given a sequence of events, return a series of user-action tuples
    relevant to determining user state.
    '''
    bots = [
        'k8s-bot',
        'k8s-ci-robot',
        'k8s-merge-robot',
        'k8s-oncall',
        'k8s-reviewable',
    ]
    skip_comments = get_skip_comments(events, bots)

    output = []
    for event, body, timestamp in events:
        action = body.get('action')
        user = body.get('sender', {}).get('login')
        if event in ('issue_comment', 'pull_request_review_comment'):
            if body['comment']['id'] in skip_comments:
                continue
            if action == 'created':
                output.append(('comment', user, timestamp))
        if event == 'pull_request_review':
            if action == 'submitted':
                # this is morally equivalent to a comment
                output.append(('comment', user, timestamp))
        if event == 'pull_request':
            if action in ('opened', 'reopened', 'synchronize'):
                output.append(('push', user, timestamp))
            if action == 'labeled' and 'label' in body:
                output.append(('label ' + body['label']['name'].lower(), user, timestamp))
    return output


def evaluate_fsm(events, start, transitions):
    '''
    Given a series of event tuples and a start state, execute the list of transitions
    and return the resulting state, the time it entered that state, and the last time
    the state would be entered (self-transitions are allowed).

    transitions is a list of tuples
    (state_before str, state_after str, condition str or callable)

    The transition occurs if condition equals the action (as a str), or if
    condition(action, user) is True.
    '''
    state = start
    state_start = 0 # time that we entered this state
    state_last = 0  # time of last transition into this state
    for action, user, timestamp in events:
        for state_before, state_after, condition in transitions:
            if state_before is None or state_before == state:
                if condition == action or (callable(condition) and condition(action, user)):
                    if state_after != state:
                        state_start = timestamp
                    state = state_after
                    state_last = timestamp
                    break
    return state, state_start, state_last


def get_author_state(author, distilled_events):
    '''
    Determine the state of the author given a series of distilled events.
    '''
    return evaluate_fsm(distilled_events, start='waiting', transitions=[
        # before, after, condition
        (None, 'address comments', lambda a, u: a == 'comment' and u != author),
        ('address comments', 'waiting', 'push'),
        ('address comments', 'waiting', lambda a, u: a == 'comment' and u == author),
    ])


def get_assignee_state(assignee, author, distilled_events):
    '''
    Determine the state of an assignee given a series of distilled events.
    '''
    return evaluate_fsm(distilled_events, start='needs review', transitions=[
        # before, after, condition
        ('needs review', 'waiting', lambda a, u: u == assignee and a in ('comment', 'label lgtm')),
        (None, 'needs review', 'push'),
        (None, 'needs review', lambda a, u: a == 'comment' and u == author),
    ])


def calculate_attention(distilled_events, payload):
    '''
    Given information about an issue, determine who should look at it.

    It can include start and last update time for various states --
    "address comments#123#456" means that something has been in 'address comments' since
    123, and there was some other event that put it in 'address comments' at 456.
    '''
    author = payload['author']
    assignees = payload['assignees']

    attn = {}
    def notify(to, reason):
        attn[to] = reason

    if any(state == 'failure' for state, _url, _desc
           in payload.get('status', {}).values()):
        notify(author, 'fix tests')

    for approver in payload.get('approvers', []):
        notify(approver, 'needs approval')

    for assignee in assignees:
        assignee_state, first, last = get_assignee_state(assignee, author, distilled_events)
        if assignee_state != 'waiting':
            notify(assignee, '%s#%s#%s' % (assignee_state, first, last))

    author_state, first, last = get_author_state(author, distilled_events)
    if author_state != 'waiting':
        notify(author, '%s#%s#%s' % (author_state, first, last))

    if payload.get('needs_rebase'):
        notify(author, 'needs rebase')
    if 'release-note-label-needed' in payload['labels']:
        notify(author, 'needs release-note label')

    return attn
