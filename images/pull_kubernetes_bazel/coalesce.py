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


import argparse
import os

import xml.etree.ElementTree as ET


BAZEL_FAILURE_HEADER = '''exec ${PAGER:-/usr/bin/less} "$0" || exit 1
-----------------------------------------------------------------------------
'''


def test_packages(root):
    """Yields test package directories under root."""
    for package, _, files in os.walk(root):
        if 'test.xml' in files and 'test.log' in files:
            yield package


def result(pkg):
    """Given a directory, create a testcase element describing it."""
    el = ET.Element('testcase')
    el.set('classname', 'go_test')
    pkg_parts = pkg.split('/')
    el.set('name', '//%s:%s' % ('/'.join(pkg_parts[1:-1]), pkg_parts[-1]))
    el.set('time', '0')
    suites = ET.parse(pkg + '/test.xml').getroot()
    for suite in suites:
        for case in suite:
            for status in case:
                if status.tag == 'error' or status.tag == 'failure':
                    failure = ET.Element('failure')
                    with open(pkg + '/test.log') as f:
                        failure.text = f.read().decode('utf8', 'ignore')
                        if failure.text.startswith(BAZEL_FAILURE_HEADER):
                            failure.text = failure.text[len(BAZEL_FAILURE_HEADER):]
                    el.append(failure)
    return el


def main():
    root = ET.Element('testsuite')
    root.set('time', '0')
    for package in sorted(test_packages('bazel-testlogs')):
        root.append(result(package))
    try:
        os.mkdir('_artifacts')
    except OSError:
        pass
    with open('_artifacts/junit_bazel.xml', 'w') as f:
        f.write(ET.tostring(root, 'utf8'))


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='Coalesce JUnit results.')
    parser.add_argument('--repo_root', default='.')
    args = parser.parse_args()
    os.chdir(args.repo_root)
    main()
