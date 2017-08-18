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

import pb_glance


def tostr(data):
    if isinstance(data, list):
        return ''.join(c if isinstance(c, str) else chr(c) for c in data)
    return data


class PBGlanceTest(unittest.TestCase):
    def expect(self, data, expected, types=None):
        result = pb_glance.parse_protobuf(tostr(data), types)
        self.assertEqual(result, expected)

    def test_basic(self):
        self.expect(
            [0, 1,                  # varint
             0, 0x96, 1,            # multi-byte varint
             (1<<3)|1, 'abcdefgh',  # 64-bit
             (2<<3)|2, 5, 'value',  # length-delimited (string)
             (3<<3)|5, 'abcd',      # 32-bit
            ],
            {
                0: [1, 150],
                1: ['abcdefgh'],
                2: ['value'],
                3: ['abcd'],
            })

    def test_embedded(self):
        self.expect([2, 2, 3<<3, 1], {0: [{3: [1]}]}, {0: {}})

    def test_field_names(self):
        self.expect([2, 2, 'hi'], {'greeting': ['hi']}, {0: 'greeting'})

    def test_embedded_names(self):
        self.expect(
            [2, 4, (3<<3)|2, 2, 'hi'],
            {'msg': [{'greeting': ['hi']}]},
            {0: {'name': 'msg', 3: 'greeting'}})
