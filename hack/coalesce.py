#!/usr/bin/env python2

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
import re

import xml.etree.ElementTree as ET


BAZEL_FAILURE_HEADER = '''exec ${PAGER:-/usr/bin/less} "$0" || exit 1
-----------------------------------------------------------------------------
'''

# from https://www.w3.org/TR/xml11/#charsets
# RestrictedChar ::= [#x1-#x8]|[#xB-#xC]|[#xE-#x1F]|[#x7F-#x84]|[#x86-#x9F]
RESTRICTED_XML_CHARS_RE = re.compile(r'[\x00-\x08\x0B\x0C\x0E-\x1F\x7F-\x84\x86-\x9F]')

ANSI_ESCAPE_CODES_RE = re.compile(r'\033\[[\d;]*[@-~]')


def test_packages(root):
    """Yields test package directories under root."""
    for package, _, files in os.walk(root):
        if 'test.xml' in files and 'test.log' in files:
            yield package

def sanitize(text):
    if text.startswith(BAZEL_FAILURE_HEADER):
        text = text[len(BAZEL_FAILURE_HEADER):]
    # ANSI escape sequences should be removed.
    text = ANSI_ESCAPE_CODES_RE.sub('', text)

    # And any other badness that slips through.
    text = RESTRICTED_XML_CHARS_RE.sub('', text)
    return text


def result(pkg):
    """Given a directory, create a testcase element describing it."""
    elem = ET.Element('testcase')
    elem.set('classname', 'go_test')
    pkg_parts = pkg.split('/')
    elem.set('name', '//%s:%s' % ('/'.join(pkg_parts[1:-1]), pkg_parts[-1]))
    elem.set('time', '0')
    suites = ET.parse(pkg + '/test.xml').getroot()
    for suite in suites:
        for case in suite:
            for status in case:
                if status.tag == 'error' or status.tag == 'failure':
                    failure = ET.Element('failure')
                    with open(pkg + '/test.log') as fp:
                        text = fp.read().decode('UTF-8', 'ignore')
                        failure.text = sanitize(text)
                    elem.append(failure)
    return elem


def main():
    root = ET.Element('testsuite')
    root.set('time', '0')
    for package in sorted(test_packages('bazel-testlogs')):
        root.append(result(package))
    artifacts_dir = os.environ.get(
        'ARTIFACTS',
        os.path.join(os.environ.get('WORKSPACE', os.getcwd()), '_artifacts'))
    try:
        os.mkdir(artifacts_dir)
    except OSError:
        pass
    with open(os.path.join(artifacts_dir, 'junit_bazel.xml'), 'w') as fp:
        fp.write(ET.tostring(root, 'UTF-8'))


if __name__ == '__main__':
    PARSER = argparse.ArgumentParser(description='Coalesce JUnit results.')
    PARSER.add_argument('--repo_root', default='.')
    ARGS = PARSER.parse_args()
    os.chdir(ARGS.repo_root)
    main()
