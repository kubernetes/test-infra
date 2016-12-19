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

"""Coalesces bazel test results into one file."""


import os
import xml.etree.ElementTree as ET


def test_packages(root):
    """Yields test package directories under root."""
    for package, _, files in os.walk(root):
        if 'test.xml' in files and 'test.log' in files:
            yield package


def result(pkg):
    """Given a directory, create a testcase element describing it."""
    el = ET.Element('testcase')
    el.set('classname', 'go_test')
    el.set('name', '_'.join(pkg.split('/')[1:-1]))
    suites = ET.parse(pkg + '/test.xml').getroot()
    for suite in suites:
        for case in suite:
            for status in case:
                if status.tag == 'error' or status.tag == 'failure':
                    failure = ET.Element('failure')
                    with open(pkg + '/test.log') as f:
                        failure.text = f.read()
                    el.append(failure)
    return el


def main():
    root = ET.Element('testsuite')
    for package in test_packages('bazel-testlogs'):
        root.append(result(package))
    try:
        os.mkdir('_artifacts')
    except OSError:
        pass
    with open('_artifacts/junit_bazel.xml', 'w') as f:
        f.write(ET.tostring(root))


if __name__ == '__main__':
    main()
