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

import pull_request


class HelperTest(unittest.TestCase):
    def test_pad_numbers(self):
        self.assertEqual(pull_request.pad_numbers('a3b45'),
                         'a' + '0' * 15 + '3b' + '0' * 14 + '45')


def make(number, version, result, start_time=1000):
    started = None if version is None else {
        'timestamp': start_time, 'version': version}
    finished = result and {'result': result}
    return (number, started, finished)


class TableTest(unittest.TestCase):

    def test_builds_to_table(self):
        jobs = {'J1': [make(4, 'v2', 'A', 9), make(3, 'v2', 'B', 10)],
                'J2': [make(5, 'v1', 'C', 7), make(4, 'v1', 'D', 6)]}
        max_builds, headings, rows = pull_request.builds_to_table(jobs)

        self.assertEqual(max_builds, 4)
        self.assertEqual(headings, [('v2', 2, 9), ('v1', 2, 6)])
        self.assertEqual(rows, [('J1', [(4, 'A'), (3, 'B')]),
                                ('J2', [None, None, (5, 'C'), (4, 'D')])])

    def test_builds_to_table_versions(self):
        jobs = {'J1': [make("3", 'v2', 'A', 10), make("5", 'v4', 'B', 13),
                       make("46", 'v4', 'C', 12), make("4", 'v3', 'D', 11)],
                'J2': [make("7", 'v5', 'E', 20)]}
        max_builds, headings, rows = pull_request.builds_to_table(jobs)

        self.assertEqual(max_builds, 5)
        self.assertEqual(headings, [('v5', 1, 20), ('v4', 2, 12),
                                    ('v3', 1, 11), ('v2', 1, 10)])
        self.assertEqual(rows, [('J1', [None,
                                        ("46", 'C'), ("5", 'B'),
                                        ("4", 'D'), ("3", 'A')]),
                                ('J2', [("7", 'E')])])

    def test_builds_to_table_no_header(self):
        jobs = {'J': [make(5, None, 'A', 3), make(4, '', 'B', 2)]}
        self.assertEqual(pull_request.builds_to_table(jobs),
                         (0, [], [('J', [(5, 'A'), (4, 'B')])]))

    def test_pull_ref_commit(self):
        jobs = {'J1': [make(4, 'v2', 'A', 9)]}
        jobs['J1'][0][1]['pull'] = 'master:1234,35:abcd'
        _, headings, _ = pull_request.builds_to_table(jobs)
        self.assertEqual(headings, [('abcd', 1, 9)])
