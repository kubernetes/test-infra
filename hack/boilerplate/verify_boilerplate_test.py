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

import io
import os
import sys
import unittest

import verify_boilerplate

class TestBoilerplate(unittest.TestCase):

  def setUp(self):
    self.old_cwd = os.getcwd()
    if os.getenv('TEST_WORKSPACE'): # Running in bazel
      os.chdir('verify/boilerplate')
    os.chdir('test/')
    self.old_out = sys.stdout
    sys.stdout = io.StringIO()

  def tearDown(self):
    sys.stdout = self.old_out
    os.chdir(self.old_cwd)

  def test_boilerplate(self):

    class Args(object):
      def __init__(self):
        self.filenames = []
        self.rootdir = '.'
        self.boilerplate_dir = '../'
        self.skip = []
        self.verbose = True

    verify_boilerplate.ARGS = Args()
    with self.assertRaises(SystemExit):
        verify_boilerplate.main()

    output = sys.stdout.getvalue()
    expected = '\n'.join(verify_boilerplate.nonconforming_lines([
        './fail.go',
        './fail.py',
    ])) + '\n' # add trailing newline

    self.assertEqual(output, expected)


if __name__ == '__main__':
    unittest.main()
