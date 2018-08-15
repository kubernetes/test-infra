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

import kubelet_parser
import regex


lines = ["line 0", "pod 2 3", "abcd podName", "line 3", "failed",
         "Event(api.ObjectReference{Namespace:\"podName\", Name:\"abc\", UID:\"uid\"}", "uid"]
filters = {"UID": "", "pod": "", "Namespace": ""}

class KubeletParserTest(unittest.TestCase):
    def test_parse_error_re(self):
        """Test for build-log.txt filtering by error_re"""
        matched_lines, highlight_words = kubelet_parser.parse(lines,
            ["error", "fatal", "failed", "build timed out"], filters, {})
        self.assertEqual(matched_lines, [4])
        self.assertEqual(highlight_words, ["error", "fatal", "failed", "build timed out"])

    def test_parse_empty_lines(self):
        """Test that it doesn't fail when files are empty"""
        matched_lines, highlight_words = kubelet_parser.parse([],
            ["error", "fatal", "failed", "build timed out"], filters, {})
        self.assertEqual(matched_lines, [])
        self.assertEqual(highlight_words, ["error", "fatal", "failed", "build timed out"])

    def test_parse_pod_RE(self):
        """Test for initial pod filtering"""
        filters["pod"] = "pod"
        matched_lines, highlight_words = kubelet_parser.parse(lines,
            ["pod"], filters, {})
        self.assertEqual(matched_lines, [1])
        self.assertEqual(highlight_words, ["pod"])

    def test_parse_filters(self):
        """Test for filters"""
        filters["pod"] = "pod"
        filters["UID"] = "on"
        filters["Namespace"] = "on"
        matched_lines, highlight_words = kubelet_parser.parse(lines,
            ["pod"], filters, {"UID":"uid", "Namespace":"podName", "ContainerID":""})
        self.assertEqual(matched_lines, [1, 2, 5, 6])
        self.assertEqual(highlight_words, ["pod", "podName", "uid"])

    def test_make_dict(self):
        """Test make_dict works"""
        objref_dict = kubelet_parser.make_dict(lines, regex.wordRE("abc"), {})
        self.assertEqual(objref_dict, ({"UID":"uid", "Namespace":"podName", "Name":"abc"}, True))

    def test_make_dict_fail(self):
        """Test when objref line not in file"""
        objref_dict = kubelet_parser.make_dict(["pod failed"], regex.wordRE("abc"), {})
        self.assertEqual(objref_dict, ({}, False))


if __name__ == '__main__':
    unittest.main()
