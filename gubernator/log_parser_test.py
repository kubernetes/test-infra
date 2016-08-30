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

import re
import unittest

import log_parser
import regex

def digest(data, strip=True, filters=None,
           error_re=regex.error_re):
    if filters is None:
        filters = {"UID":"", "pod":"", "Namespace":"", "ContainerID":""}

    digested = log_parser.digest(data.replace(' ', '\n'), error_re=error_re,
                                 skip_fmt=lambda l: 's%d' % l, filters=filters)
    print digested
    if strip:
        digested = re.sub(r'<span class="skipped"[^<]*>([^<]*)</span>', r'(\1)',
            digested, flags=re.MULTILINE)
        digested = re.sub(r'<[^>]*>', '', digested)
    return digested.replace('\n', ' ')

class LogParserTest(unittest.TestCase):
    def setUp(self):
        log_parser.CONTEXT_DEFAULT = 4

    def expect(self, data, expected):
        self.assertEqual(digest(data), expected)

    def test_empty(self):
        self.expect('', '')
        self.expect('no problems here!', 's3')

    def test_escaping(self):
        self.expect('error &c',
            'error &amp;c')

    def test_context(self):
        self.expect('0 1 2 3 4 5 error 6 7 8 9 10', 's2 2 3 4 5 error 6 7 8 9 10')

    def test_multi_context(self):
        self.expect('0 1 2 3 4 error-1 6 error-2 8 9 10 11 12 13',
            '0 1 2 3 4 error-1 6 error-2 8 9 10 11 s2')

    def test_skip_count(self):
        self.expect('error 1 2 3 4 5 6 7 8 9 A error-2',
            'error 1 2 3 4 s2 7 8 9 A error-2')

    def test_skip_at_least_two(self):
        self.expect('error 1 2 3 4 5 6 7 8 error-2', 'error 1 2 3 4 5 6 7 8 error-2')

    def test_html(self):
        self.assertEqual(digest('error-blah', strip=False), ''
                         '<span class="hilight"><span class="keyword">'
                         'error</span>-blah</span>')

    def test_html_range(self):
        self.assertEqual(digest('error 1 2 3 4 5 6 7 8', strip=False),
            '<span class="hilight"><span class="keyword">error</span></span>'
            ' 1 2 3 4 <span class="skip" data-range="5-9">s4</span>')

    def test_pod(self):
        self.assertEqual(digest(
            'pod-blah', error_re=regex.wordRE("pod"), strip=False),
            '<span class="hilight">pod-blah</span>')
        self.assertEqual(digest('0 1 2 3 4 5 pod 6 7 8 9 10',
            error_re=regex.wordRE("pod"),
            filters={"pod": "pod", "UID": "", "Namespace": "", "ContainerID":""}),
            's2 2 3 4 5 pod 6 7 8 9 10')


if __name__ == '__main__':
    unittest.main()
