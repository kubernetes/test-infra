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

import unittest

import classifier


class MergedTest(unittest.TestCase):
    def test_merged(self):
        self.assertEqual(classifier.get_merged(zip('abcd', [
            {'issue': {'n': 1, 'a': 2}},
            {'pull_request': {'n': 2, 'b': 3}},
            {'c': 4},
            {'issue': {'n': 3, 'd': 4},
             'pull_request': {'n': 4, 'e': 5}}
        ], [0] * 4)), {'n': 4, 'a': 2, 'b': 3, 'd': 4, 'e': 5})


def diffs_to_events(*diffs):
    events = []
    for diff in diffs:
        label = {'name': diff[1:], 'color': '#fff'}
        if diff[0] == '+':
            action = 'labeled'
        elif diff[0] == '-':
            action = 'unlabeled'
        events.append(('pull_request',
                       {'action': action,
                        'label': label}, 0))
    return events


class LabelsTest(unittest.TestCase):
    def expect_labels(self, events, names, extra_events=None):
        labels = classifier.get_labels(events)
        if extra_events:
            labels = classifier.get_labels(extra_events, labels)
        self.assertEqual(sorted(labels.keys()), sorted(names))

    def test_empty(self):
        self.expect_labels([('comment', {'body': 'no labels here'}, 0)], [])

    def test_colors(self):
        self.assertEqual(classifier.get_labels(
                [('c', {'issue':
                        {'labels': [{'name': 'foo', 'color': '#abc'}]}
                        }, 0)]),
            {'foo': '#abc'})

    def test_labeled_action(self):
        self.expect_labels(diffs_to_events('+a'), ['a'])
        self.expect_labels(diffs_to_events('+a', '+a'), ['a'])
        self.expect_labels(diffs_to_events('+a', '-a'), [])
        self.expect_labels(diffs_to_events('+a', '+b', '-c', '-b'), ['a'])
        self.expect_labels(diffs_to_events('+a', '+b', '-c'), ['a'],
                           extra_events=diffs_to_events('-b'))

    def test_issue_overrides_action(self):
        labels = [{'name': 'x', 'color': 'y'}]
        self.expect_labels(diffs_to_events('+a') +
            [('other_event', {'issue': {'labels': labels}}, 0)], ['x'])

    def test_labeled_action_missing_label(self):
        self.expect_labels([('pull_request', {'action': 'labeled'}, 0)], [])


def make_comment_event(num, name, msg='', event='issue_comment',
                       action='created', ts=None):
    return event, {
        'action': action,
        'sender': {'login': name},
        'comment': {
            'id': num,
            'user': {'login': name},
            'body': msg,
            'created_at': ts,
        }
    }, ts


class CalculateTest(unittest.TestCase):
    def test_classify(self):
        # A quick integration test to ensure that all the sub-parts are included.
        # If this test fails, a smaller unit test SHOULD fail as well.
        self.assertEqual(classifier.classify([
                ('pull_request', {
                    'pull_request': {
                        'state': 'open',
                        'user': {'login': 'a'},
                        'assignees': [{'login': 'b'}],
                        'title': 'some fix',
                        'head': {'sha': 'abcdef'},
                        'additions': 1,
                        'deletions': 1,
                        'milestone': {'title': 'v1.10'},
                    }
                }, 1),
                make_comment_event(1, 'k8s-bot',
                    'failure in https://gubernator.k8s.io/build/bucket/job/123/', ts=2),
                ('pull_request', {
                    'action': 'labeled',
                    'label': {'name': 'release-note-none', 'color': 'orange'},
                }, 3),
                make_comment_event(2, 'k8s-merge-robot', '<!-- META={"approvers":["o"]} -->', ts=4),
            ], status_fetcher={'abcdef': {'e2e': ['failure', None, 'stuff is broken']}}.get
        ),
        (True, True, ['a', 'b', 'o'],
         {
            'author': 'a',
            'approvers': ['o'],
            'assignees': ['b'],
            'additions': 1,
            'deletions': 1,
            'attn': {'a': 'fix tests', 'b': 'needs review#0#0', 'o': 'needs approval'},
            'title': 'some fix',
            'labels': {'release-note-none': 'orange'},
            'head': 'abcdef',
            'needs_rebase': False,
            'status': {'e2e': ['failure', None, 'stuff is broken']},
            'xrefs': ['/bucket/job/123'],
            'milestone': 'v1.10',
        }))

    def test_distill(self):
        self.assertEqual(classifier.distill_events([
            make_comment_event(1, 'a', ts=1),
            make_comment_event(2, 'b', ts=2),
            make_comment_event(1, 'a', action='deleted', ts=3),
            make_comment_event(3, 'c', event='pull_request_review_comment', ts=4),
            make_comment_event(4, 'k8s-bot', ts=4),
            ('pull_request', {'action': 'synchronize', 'sender': {'login': 'auth'}}, 5),
            ('pull_request', {'action': 'labeled', 'sender': {'login': 'rev'},
                'label': {'name': 'lgtm'}}, 6),
        ]),
        [
            ('comment', 'b', 2),
            ('comment', 'c', 4),
            ('push', 'auth', 5),
            ('label lgtm', 'rev', 6),
        ])

    def test_calculate_attention(self):
        def expect(payload, events, expected_attn):
            self.assertEqual(classifier.calculate_attention(events, payload),
                             expected_attn)

        def make_payload(author, assignees=None, labels=None, **kwargs):
            ret = {'author': author, 'assignees': assignees or [], 'labels': labels or []}
            ret.update(kwargs)
            return ret

        expect(make_payload('alpha', needs_rebase=True), [],
            {'alpha': 'needs rebase'})
        expect(make_payload('beta', labels={'do-not-merge/release-note-label-needed'}), [],
            {'beta': 'needs release-note label'})
        expect(make_payload('gamma', status={'ci': ['failure', '', '']}), [],
            {'gamma': 'fix tests'})
        expect(make_payload('gamma', status={'ci': ['failure', '', '']}),
            [('comment', 'other', 1)],
            {'gamma': 'address comments#1#1'})
        expect(make_payload('delta', ['epsilon']), [],
            {'epsilon': 'needs review#0#0'})

        expect(make_payload('alpha', ['alpha']), [('comment', 'other', 1)],
            {'alpha': 'address comments#1#1'})

        expect(make_payload('alpha', approvers=['owner']), [],
            {'owner': 'needs approval'})

    def test_author_state(self):
        def expect(events, result):
            self.assertEqual(classifier.get_author_state('author', events),
                             result)
        expect([], ('waiting', 0, 0))
        expect([('comment', 'author', 1)], ('waiting', 0, 0))
        expect([('comment', 'other', 1)], ('address comments', 1, 1))
        expect([('comment', 'other', 1), ('push', 'author', 2)], ('waiting', 2, 2))
        expect([('comment', 'other', 1), ('comment', 'author', 2)], ('waiting', 2, 2))
        expect([('comment', 'other', 1), ('comment', 'other', 2)], ('address comments', 1, 2))

    def test_assignee_state(self):
        def expect(events, result):
            self.assertEqual(classifier.get_assignee_state('me', 'author', events),
                             result)
        expect([], ('needs review', 0, 0))
        expect([('comment', 'other', 1)], ('needs review', 0, 0))
        expect([('comment', 'me', 1)], ('waiting', 1, 1))
        expect([('label lgtm', 'other', 1)], ('needs review', 0, 0))
        expect([('label lgtm', 'me', 1)], ('waiting', 1, 1))
        expect([('comment', 'me', 1), ('push', 'author', 2)], ('needs review', 2, 2))
        expect([('comment', 'me', 1), ('comment', 'author', 2)], ('needs review', 2, 2))
        expect([('comment', 'me', 1), ('comment', 'author', 2), ('comment', 'author', 3)],
               ('needs review', 2, 3))

    def test_xrefs(self):
        def expect(body, comments, result):
            self.assertEqual(result, classifier.get_xrefs(
                [{'comment': c} for c in comments], {'body': body}))
        def fail(path):
            return 'foobar https://gubernator.k8s.io/build%s asdf' % path
        expect(None, [], [])
        expect('something', [], [])
        expect(fail('/a/b/34/'), [], ['/a/b/34'])
        expect(None, [fail('/a/b/34/')], ['/a/b/34'])
        expect(fail('/a/b/34/'), [fail('/a/b/34]')], ['/a/b/34'])
        expect(fail('/a/b/34/)'), [fail('/a/b/35]')], ['/a/b/34', '/a/b/35'])

    def test_reviewers(self):
        def expect(events, result):
            self.assertEqual(result, classifier.get_reviewers(events))

        def mk(*specs):
            out = []
            for event, action, body in specs:
                body = dict(body)  # copy
                body['action'] = action
                out.append((event, body, 0))
            return out

        expect([], set())

        user_a = {'requested_reviewer': {'login': 'a'}}
        expect(mk(('pull_request', 'review_requested', user_a)), {'a'})
        expect(mk(('pull_request', 'review_request_removed', user_a)), set())
        expect(mk(('pull_request', 'review_requested', user_a),
                  ('pull_request', 'review_request_removed', user_a)), set())
        expect(mk(('pull_request_review', 'submitted', {'sender': {'login': 'a'}})), {'a'})

    def test_approvers(self):
        def expect(comment, result):
            self.assertEqual(result, classifier.get_approvers([{
                'author': 'k8s-merge-robot', 'comment': comment}]))

        expect('nothing', [])
        expect('before\n<!-- META={approvers:[someone]} -->', ['someone'])
        expect('<!-- META={approvers:[someone,else]} -->', ['someone', 'else'])
        expect('<!-- META={approvers:[someone,else]} -->', ['someone', 'else'])

        # The META format is *supposed* to be JSON, but a recent change broke it.
        # Support both formats so it can be fixed in the future.
        expect('<!-- META={"approvers":["username"]} -->\n', ['username'])


class CommentsTest(unittest.TestCase):
    def test_basic(self):
        self.assertEqual(classifier.get_comments([make_comment_event(1, 'aaa', 'msg', ts=2016)]),
            [{'id': 1, 'author': 'aaa', 'comment': 'msg', 'timestamp': 2016}])

    def test_deleted(self):
        self.assertEqual(classifier.get_comments([
            make_comment_event(1, 'aaa', 'msg', 2016),
            make_comment_event(1, None, None, None, action='deleted'),
            make_comment_event(2, '', '', '', action='deleted')]),
            [])

    def test_edited(self):
        self.assertEqual(classifier.get_comments([
            make_comment_event(1, 'aaa', 'msg', ts=2016),
            make_comment_event(1, 'aaa', 'redacted', ts=2016.1, action='edited')]),
            [{'id': 1, 'author': 'aaa', 'comment': 'redacted', 'timestamp': 2016.1}])


if __name__ == '__main__':
    unittest.main()
