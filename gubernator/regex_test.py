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
            ('FAIL k8s.io/kubernetes/pkg/client/record', True),
            ('undefined: someVariable', True),
            ('\x1b[0;31mFAILED\x1b[0m', True),  # color codes
        ]:
            self.assertEqual(bool(regex.error_re.search(text)), matches,
                'error_re.search(%r) should be %r' % (text, matches))


    def test_objref(self):
        for text, matches in [
            ('api.ObjectReference{Kind:&#34;Pod&#34;} failed', True),
            ('{Pod:&#34;abc&#34;, Namespace:\"pod abc\"}', False),
            ('Jan 1: Event(api.ObjectReference{Kind:&#34;Pod&#34;, Podname:&#34;abc&#34;}) failed'
                , True),
        ]:
            self.assertEqual(bool(regex.objref(text)), matches,
                'objref(%r) should be %r' % (text, matches))


    def test_combine_wordsRE(self):
        for text, matches in [
            ('pod123 failed', True),
            ('Volume mounted to pod', True),
            ('UID: "a123"', True),
        ]:
            self.assertEqual(bool(regex.combine_wordsRE(["pod123", "volume", "a123"])), matches,
                'combine_words(%r) should be %r' % (text, matches))


    def test_log_re(self):
        for text, matches in [
            ('build-log.txt', False),
            ('a/b/c/kublet.log', True),
            ('kube-apiserver.log', True),
            ('abc/kubelet.log/cde', False),
            ('path/to/log', False),
        ]:
            self.assertEqual(bool(regex.log_re.search(text)), matches,
                'log_re(%r) should be %r' % (text, matches))


    def test_containerID(self):
        for text, matches in [
            ('the ContainerID:ab123cd', True),
            ('ContainerID:}]}', False),
            ('ContainerID:', False),
        ]:
            self.assertEqual(bool(regex.containerID(text).group(1)), matches,
                'containerID(%r).group(1) should be %r' % (text, matches))


    def test_timestamp(self):
        for text, matches in [
            ('I0629 17:33:09.813041', True),
            ('2016-07-22T19:01:11.150204523Z', True),
            ('629 17:33:09.813041:', False),
            ('629 17:33:09', False),
        ]:
            self.assertEqual(bool(regex.timestamp(text)), matches,
                'test_timestamp(%r) should be %r' % (text, matches))


    def test_sub_timestamp(self):
        for text, matches in [
            ('0629 17:33:09.813041', '062917:33:09.813041'),
            ('07-22T19:01:11.150204523', '072219:01:11.150204523'),
        ]:
            self.assertEqual(regex.sub_timestamp(text), matches,
                'sub_timetamp(%r) should be %r' % (text, matches))


if __name__ == '__main__':
    unittest.main()
