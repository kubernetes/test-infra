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

"""Tests for graph."""

import datetime
import unittest

import graph


class GraphTest(unittest.TestCase):
    def test_parse_line_normal(self):
        res = graph.parse_line(
            '2017-02-28', '13:50:12.555', 'True', '50', '8', '17', 'False', 7)
        expected = (
            datetime.datetime(2017, 2, 28, 13, 50, 12, 555000),
            True,
            50, 8, 17, False, 7)
        self.assertEquals(res, expected)

    def test_parse_line_fractionless(self):
        res = graph.parse_line(
            '2017-02-28', '13:50:12', 'True', '50', '8', '17', 'False', 7)
        expected = (
            datetime.datetime(2017, 2, 28, 13, 50, 12, 0),
            True,
            50, 8, 17, False, 7)
        self.assertEquals(res, expected)

    def test_parse_line_mergeless(self):
        res = graph.parse_line(
            '2017-02-28', '13:50:12.555', 'True', '50', '8', '17', 'False')
        expected = (
            datetime.datetime(2017, 2, 28, 13, 50, 12, 555000),
            True,
            50, 8, 17, False, 0)
        self.assertEquals(res, expected)



if __name__ == '__main__':
    unittest.main()
