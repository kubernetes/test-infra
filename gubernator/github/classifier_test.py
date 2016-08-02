#!/usr/bin/env python

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
        ])), {'n': 4, 'a': 2, 'b': 3, 'd': 4, 'e': 5})


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
                        'label': label}))
    return events


class LabelsTest(unittest.TestCase):
    def expect_labels(self, events, names):
        labels = classifier.get_labels(events)
        self.assertEqual(sorted(labels.keys()), sorted(names))

    def test_empty(self):
        self.expect_labels([('comment', {'body': 'no labels here'})], [])

    def test_colors(self):
        self.assertEqual(classifier.get_labels(
                [('c', {'issue':
                        {'labels': [{'name': 'foo', 'color': '#abc'}]}
            })]),
            {'foo': '#abc'})

    def test_labeled_action(self):
        self.expect_labels(diffs_to_events('+a'), ['a'])
        self.expect_labels(diffs_to_events('+a', '+a'), ['a'])
        self.expect_labels(diffs_to_events('+a', '-a'), [])
        self.expect_labels(diffs_to_events('+a', '+b', '-c', '-b'), ['a'])

    def test_issue_overrides_action(self):
        labels = [{'name': 'x', 'color': 'y'}]
        self.expect_labels(diffs_to_events('+a') +
            [('other_event', {'issue': {'labels': labels}})], ['x'])

    def test_labeled_action_missing_label(self):
        self.expect_labels([('pull_request', {'action': 'labeled'})], [])


def make_comment_event(num, name, msg, timestamp, event='issue_comment',
                       action='added'):
    return event, {
        'action': action,
        'comment': {
            'id': num,
            'user': {'login': name},
            'body': msg,
            'created_at': timestamp,
        }
    }


class CommentsTest(unittest.TestCase):
    def test_basic(self):
        self.assertEqual(classifier.get_comments([make_comment_event(1, 'aaa', 'msg', 2016)]),
            [{'author': 'aaa', 'comment': 'msg', 'timestamp': 2016}])

    def test_deleted(self):
        self.assertEqual(classifier.get_comments([
            make_comment_event(1, 'aaa', 'msg', 2016),
            make_comment_event(1, None, None, None, action='deleted'),
            make_comment_event(2, '', '', '', action='deleted')]),
            [])


if __name__ == '__main__':
    unittest.main()
