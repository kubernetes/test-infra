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

import view_build

import main_test


class ParseJunitTest(unittest.TestCase):
    @staticmethod
    def parse(xml):
        return list(view_build.parse_junit(xml, "fp"))

    def test_normal(self):
        failures = self.parse(main_test.JUNIT_SUITE)
        stack = '/go/src/k8s.io/kubernetes/test.go:123\nError Goes Here'
        self.assertEqual(failures, [('Third', 96.49, stack, "fp")])

    def test_testsuites(self):
        failures = self.parse('''
            <testsuites>
                <testsuite name="k8s.io/suite">
                    <properties>
                        <property name="go.version" value="go1.6"/>
                    </properties>
                    <testcase name="TestBad" time="0.1">
                        <failure>something bad</failure>
                    </testcase>
                </testsuite>
            </testsuites>''')
        self.assertEqual(failures,
                         [('k8s.io/suite TestBad', 0.1, 'something bad', "fp")])

    def test_bad_xml(self):
        self.assertEqual(self.parse('''<body />'''), [])
