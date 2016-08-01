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

import regex

class RegexTest(unittest.TestCase):

    def test_wordRE(self):
        for text, matches in [
            ('/abcdef/', True),
            ('Pod abcdef failed', True),
            ('abcdef', True),
            ('cdabcdef', False),
            ('abc def', False),
            ('Podname(abcdef)', True),
        ]:
            self.assertEqual(bool(regex.wordRE("abcdef").search(text)), matches,
                'wordRE(abcdef).search(%r) should be %r' % (text, matches))


    def test_error_re(self):
        for text, matches in [
            ('errno blah', False),
            ('ERROR: woops', True),
            ('Build timed out', True),
            ('something timed out', False),
            ('misc. fatality', False),
            ('there was a FaTaL error', True),
            ('we failed to read logs', True),
        ]:
            self.assertEqual(bool(regex.error_re.search(text)), matches,
                'error_re.search(%r) should be %r' % (text, matches))


    def test_objref(self):
        for text, matches in [
            ('Event(api.ObjectReference{Kind:\"Pod\"}) failed', True),
            ('{Pod:\"abc\", Namespace:\"pod abc\"}', False),
            ('Jan 1: Event(api.ObjectReference{Kind:\"Pod\", Podname:\"abc\"}) failed', True),
        ]:
            self.assertEqual(bool(regex.objref(text)), matches,
                'objref(%r) should be %r' % (text, matches))


if __name__ == '__main__':
    unittest.main()
