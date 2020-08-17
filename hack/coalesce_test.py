#!/usr/bin/env python3

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

import os
import shutil
import tempfile
import unittest
import xml.etree.ElementTree as ET

import coalesce

class TestCoalesce(unittest.TestCase):
    def setUp(self):
        self.tmpdir = tempfile.mkdtemp(prefix='coalesce_test_')

    def tearDown(self):
        shutil.rmtree(self.tmpdir)

    def make_result(self, name, error=''):
        pkg = os.path.join(self.tmpdir, name)
        os.makedirs(pkg)
        if error:
            inner = '<failure>something bad</failure>'
        else:
            inner = ''
        # Pass the encoding parameter to avoid ascii decode error for some
        # platform.
        with open(pkg + '/test.log', 'w', encoding='utf-8') as fp:
            fp.write(error)
        with open(pkg + '/test.xml', 'w', encoding='utf-8') as fp:
            fp.write('''<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite name="{name}" tests="1" failures="0" errors="0">
    <testcase name="{name}" status="run">{inner}</testcase>
  </testsuite>
</testsuites>'''.format(name=name, inner=inner))

        return pkg

    def test_utf8(self):
        uni_string = '\u8a66\u3057'
        pkg = self.make_result(name='coal', error=uni_string)
        result = coalesce.result(pkg)
        self.assertEqual(result.find('failure').text, uni_string)

    def test_header_strip(self):
        failure = '''exec ${PAGER:-/usr/bin/less} "$0" || exit 1
-----------------------------------------------------------------------------
something bad'''
        pkg = self.make_result(name='coal', error=failure)
        result = coalesce.result(pkg)
        self.assertEqual(result.find('failure').text, 'something bad')

    def test_sanitize_bad(self):
        self.assertEqual(coalesce.sanitize('foo\033\x00\x08'), 'foo')

    def test_sanitize_ansi(self):
        self.assertEqual(coalesce.sanitize('foo\033[1mbar\033[1mbaz'),
                         'foobarbaz')

    def test_package_names(self):
        os.chdir(self.tmpdir)
        os.putenv('WORKSPACE', self.tmpdir)
        os.symlink('.', 'bazel-testlogs')

        self.make_result(name='coal/sub_test')
        self.make_result(name='coal/other_test')
        self.make_result(name='some/deep/package/go_test')

        coalesce.main()

        # Pass the encoding parameter to avoid ascii decode error for some
        # platform.
        with open('_artifacts/junit_bazel.xml', encoding='utf-8') as fp:
            data = fp.read()

        root = ET.fromstring(data)
        names = [x.attrib['name'] for x in root.findall('testcase')]
        self.assertEqual(
            names,
            ['//coal:other_test', '//coal:sub_test', '//some/deep/package:go_test']
        )


if __name__ == '__main__':
    unittest.main()
