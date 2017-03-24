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

"""Test for kubernetes_e2e.py"""

import json
import re
import shutil
import string
import urllib
import unittest

import kubernetes_e2e

FAKE_WORKSPACE_STATUS = 'STABLE_BUILD_GIT_COMMIT 599539dc0b99976fda0f326f4ce47e93ec07217c\n' \
'STABLE_BUILD_SCM_STATUS clean\n' \
'STABLE_BUILD_SCM_REVISION v1.7.0-alpha.0.1320+599539dc0b9997\n' \
'STABLE_BUILD_MAJOR_VERSION 1\n' \
'STABLE_BUILD_MINOR_VERSION 7+\n' \
'STABLE_gitCommit 599539dc0b99976fda0f326f4ce47e93ec07217c\n' \
'STABLE_gitTreeState clean\n' \
'STABLE_gitVersion v1.7.0-alpha.0.1320+599539dc0b9997\n' \
'STABLE_gitMajor 1\n' \
'STABLE_gitMinor 7+\n'

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


class ScenarioTest(unittest.TestCase):
    """Test for e2e scenario."""
    callstack = []
    envs = {}

    def setUp(self):
        self.parser = kubernetes_e2e.create_parser()
        self.boiler = [
            Stub(kubernetes_e2e, 'check', self.fake_check),
            Stub(shutil, 'copy', fake_pass),
        ]

    def tearDown(self):
        for stub in self.boiler:
            with stub:  # Leaving with restores things
                pass
        self.callstack[:] = []
        self.envs.clear()

    def fake_check(self, *cmd):
        """Log the command."""
        self.callstack.append(string.join(cmd))

    def fake_check_env(self, env, *cmd):
        """Log the command with a specific env."""
        self.envs.update(env)
        self.callstack.append(string.join(cmd))

    def fake_output_work_status(self, *cmd):
        """fake a workstatus bolb."""
        self.callstack.append(string.join(cmd))
        return FAKE_WORKSPACE_STATUS


class LocalTest(ScenarioTest):
    """Class for testing e2e scenario in local mode."""
    def test_local(self):
        """Make sure local mode is fine overall."""
        args = self.parser.parse_args(['--mode=local'])
        self.assertEqual(args.mode, 'local')
        with Stub(kubernetes_e2e, 'check_env', self.fake_check_env):
            kubernetes_e2e.main(args)

        self.assertNotEqual(self.envs, {})
        for call in self.callstack:
            self.assertFalse(call.startswith('docker'))

    def test_kubeadm(self):
        """Make sure kubeadm mode is fine overall."""
        args = self.parser.parse_args(['--mode=local', '--kubeadm'])
        self.assertEqual(args.mode, 'local')
        self.assertEqual(args.kubeadm, True)
        with Stub(kubernetes_e2e, 'check_env', self.fake_check_env):
            with Stub(kubernetes_e2e, 'check_output', self.fake_output_work_status):
                kubernetes_e2e.main(args)

        self.assertIn('E2E_OPT', self.envs)
        self.assertIn('v1.7.0-alpha.0.1320+599539dc0b9997', self.envs['E2E_OPT'])
        called = False
        for call in self.callstack:
            self.assertFalse(call.startswith('docker'))
            if call == 'hack/print-workspace-status.sh':
                called = True
        self.assertTrue(called)

class DockerTest(ScenarioTest):
    """Class for testing e2e scenario in docker mode."""
    def test_docker(self):
        """Make sure docker mode is fine overall."""
        args = self.parser.parse_args()
        self.assertEqual(args.mode, 'docker')
        with Stub(kubernetes_e2e, 'check_env', fake_bomb):
            kubernetes_e2e.main(args)

        self.assertEqual(self.envs, {})
        for call in self.callstack:
            self.assertTrue(call.startswith('docker'))

    def test_default_tag(self):
        """Ensure the default tag exists on gcr.io."""
        args = self.parser.parse_args()
        match = re.match('gcr.io/([^:]+):(.+)', kubernetes_e2e.kubekins(args.tag))
        self.assertIsNotNone(match)
        url = 'https://gcr.io/v2/%s/manifests/%s' % (match.group(1),
                                                     match.group(2))
        data = json.loads(urllib.urlopen(url).read())
        self.assertNotIn('errors', data)
        self.assertIn('name', data)

if __name__ == '__main__':
    unittest.main()
