#!/usr/bin/env python

# Copyright 2017 The Kubernetes Authors.
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

# Need to figure out why this only fails on travis
# pylint: disable=too-few-public-methods

"""Test for kubernetes_bazel.py"""

import string
import unittest

import kubernetes_bazel


def fake_pass(*_unused, **_unused2):
    """Do nothing."""
    pass

def fake_bomb(*a, **kw):
    """Always raise."""
    raise AssertionError('Should not happen', a, kw)


class Stub(object):
    """Replace thing.param with replacement until exiting with."""
    def __init__(self, thing, param, replacement):
        self.thing = thing
        self.param = param
        self.replacement = replacement
        self.old = getattr(thing, param)
        setattr(thing, param, self.replacement)

    def __enter__(self, *a, **kw):
        return self.replacement

    def __exit__(self, *a, **kw):
        setattr(self.thing, self.param, self.old)


class ScenarioTest(unittest.TestCase):  # pylint: disable=too-many-public-methods
    """Tests for bazel scenario."""
    callstack = []

    def setUp(self):
        self.boiler = [
            Stub(kubernetes_bazel, 'check', self.fake_check),
        ]

    def tearDown(self):
        for stub in self.boiler:
            with stub:  # Leaving with restores things
                pass
        self.callstack[:] = []

    def fake_check(self, *cmd):
        """Log the command."""
        self.callstack.append(string.join(cmd))

    @staticmethod
    def fake_version():
        """return a fake version"""
        return 'v1.0+abcde'

    def test_expand(self):
        """Make sure flags are expanded properly."""
        args = kubernetes_bazel.parse_args([
            '--build=--flag=a //b/... //c/...'
            ])

        kubernetes_bazel.main(args)

        for call in self.callstack:
            if 'build' in call:
                self.assertIn('--flag=a', call)
                self.assertIn('//b/...', call)
                self.assertIn('//c/...', call)


    def test_all_bazel(self):
        """Make sure all commands starts with bazel except for coarse"""
        args = kubernetes_bazel.parse_args([
            '--build=//a',
            '--test=//b',
            '--release=//c'
            ])

        with Stub(kubernetes_bazel, 'get_version', self.fake_version):
            kubernetes_bazel.main(args)

        for call in self.callstack[:-2]:
            self.assertFalse(call.startswith('bazel'))

if __name__ == '__main__':
    unittest.main()
