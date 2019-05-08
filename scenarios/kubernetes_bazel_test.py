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

import os
import string
import tempfile
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
        self.boiler = {
            'check': Stub(kubernetes_bazel, 'check', self.fake_check),
            'check_output': Stub(kubernetes_bazel, 'check_output', self.fake_check),
            'query': Stub(kubernetes_bazel.Bazel, 'query', self.fake_query),
            'get_version': Stub(kubernetes_bazel, 'get_version', self.fake_version),
            'clean_file_in_dir': Stub(kubernetes_bazel, 'clean_file_in_dir', self.fake_clean),
        }

    def tearDown(self):
        for _, stub in self.boiler.items():
            with stub:  # Leaving with restores things
                pass
        self.callstack[:] = []

    def fake_check(self, *cmd):
        """Log the command."""
        self.callstack.append(string.join(cmd))

    @staticmethod
    def fake_sha(key, default):
        """fake base and pull sha."""
        if key == 'PULL_BASE_SHA':
            return '12345'
        if key == 'PULL_PULL_SHA':
            return '67890'
        return os.getenv(key, default)

    @staticmethod
    def fake_version():
        """return a fake version"""
        return 'v1.0+abcde'

    @staticmethod
    def fake_query(_self, _kind, selected, changed):
        """Simple filter selected by changed."""
        if changed == []:
            return changed
        if not changed:
            return selected
        if not selected:
            return changed

        ret = []
        for pkg in selected:
            if pkg in changed:
                ret.append(pkg)

        return ret

    @staticmethod
    def fake_changed_valid(_base, _pull):
        """Return fake affected targets."""
        return ['//foo', '//bar']

    @staticmethod
    def fake_changed_empty(_base, _pull):
        """Return fake affected targets."""
        return []

    @staticmethod
    def fake_clean(_dirname, _filename):
        """Don't clean"""
        pass

    def test_expand(self):
        """Make sure flags are expanded properly."""
        args = kubernetes_bazel.parse_args([
            '--build=//b/... -//b/bb/... //c/...'
            ])
        kubernetes_bazel.main(args)

        call = self.callstack[-2]
        self.assertIn('//b/...', call)
        self.assertIn('-//b/bb/...', call)
        self.assertIn('//c/...', call)


    def test_query(self):
        """Make sure query is constructed properly."""
        args = kubernetes_bazel.parse_args([
            '--build=//b/... -//b/bb/... //c/...'
            ])
        # temporarily un-stub query
        with Stub(kubernetes_bazel.Bazel, 'query', self.boiler['query'].old):
            def check_query(*cmd):
                self.assertIn(
                    'kind(.*_binary, rdeps(//b/... -//b/bb/... +//c/..., //...))'
                    ' except attr(\'tags\', \'manual\', //...)',
                    cmd
                )
                return '//b/aa/...\n//c/...'
            with Stub(kubernetes_bazel, 'check_output', check_query):
                kubernetes_bazel.main(args)

    def test_expand_arg(self):
        """Make sure flags are expanded properly."""
        args = kubernetes_bazel.parse_args([
            '--test-args=--foo',
            '--test-args=--bar',
            '--test=//b/... //c/...'
            ])
        kubernetes_bazel.main(args)

        call = self.callstack[-2]
        self.assertIn('--foo', call)
        self.assertIn('--bar', call)
        self.assertIn('//b/...', call)
        self.assertIn('//c/...', call)

    def test_all_bazel(self):
        """Make sure all commands starts with bazel except for coarse."""
        args = kubernetes_bazel.parse_args([
            '--build=//a',
            '--test=//b',
            '--release=//c'
            ])
        kubernetes_bazel.main(args)

        for call in self.callstack[:-2]:
            self.assertTrue(call.startswith('bazel'), call)

    def test_install(self):
        """Make sure install is called as 1st scenario call."""
        with tempfile.NamedTemporaryFile(delete=False) as fp:
            install = fp.name
        args = kubernetes_bazel.parse_args([
            '--install=%s' % install,
            ])
        kubernetes_bazel.main(args)

        self.assertIn(install, self.callstack[0])

    def test_install_fail(self):
        """Make sure install fails if path does not exist."""
        args = kubernetes_bazel.parse_args([
            '--install=foo',
            ])
        with self.assertRaises(ValueError):
            kubernetes_bazel.main(args)

    def test_affected(self):
        """--test=affected will work."""
        args = kubernetes_bazel.parse_args([
            '--affected',
            ])
        with self.assertRaises(ValueError):
            kubernetes_bazel.main(args)

        with Stub(kubernetes_bazel, 'get_changed', self.fake_changed_valid):
            with Stub(os, 'getenv', self.fake_sha):
                kubernetes_bazel.main(args)
                test = self.callstack[-2]
                self.assertIn('//foo', test)
                self.assertIn('//bar', test)

                build = self.callstack[-3]
                self.assertIn('//foo', build)
                self.assertIn('//bar', build)

    def test_affected_empty(self):
        """if --affected returns nothing, then nothing should be triggered"""
        args = kubernetes_bazel.parse_args([
            '--affected',
            ])
        with Stub(kubernetes_bazel, 'get_changed', self.fake_changed_empty):
            with Stub(os, 'getenv', self.fake_sha):
                kubernetes_bazel.main(args)
                # trigger empty build
                self.assertIn('bazel build', self.callstack)
                # nothing to test
                for call in self.callstack:
                    self.assertNotIn('bazel test', call)

    def test_affected_filter(self):
        """--test=affected will work."""
        args = kubernetes_bazel.parse_args([
            '--affected',
            '--build=//foo',
            '--test=//foo',
            ])
        with Stub(kubernetes_bazel, 'get_changed', self.fake_changed_valid):
            with Stub(os, 'getenv', self.fake_sha):
                kubernetes_bazel.main(args)
                test = self.callstack[-2]
                self.assertIn('//foo', test)
                self.assertNotIn('//bar', test)

                build = self.callstack[-3]
                self.assertIn('//foo', build)
                self.assertNotIn('//bar', build)


if __name__ == '__main__':
    unittest.main()
