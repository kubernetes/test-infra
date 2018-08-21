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

import os
import shutil
import string
import tempfile
import urllib2
import unittest
import time

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

FAKE_WORKSPACE_STATUS_V1_6 = 'STABLE_BUILD_GIT_COMMIT 84febd4537dd190518657405b7bdb921dfbe0387\n' \
'STABLE_BUILD_SCM_STATUS clean\n' \
'STABLE_BUILD_SCM_REVISION v1.6.4-beta.0.18+84febd4537dd19\n' \
'STABLE_BUILD_MAJOR_VERSION 1\n' \
'STABLE_BUILD_MINOR_VERSION 6+\n' \
'STABLE_gitCommit 84febd4537dd190518657405b7bdb921dfbe0387\n' \
'STABLE_gitTreeState clean\n' \
'STABLE_gitVersion v1.6.4-beta.0.18+84febd4537dd19\n' \
'STABLE_gitMajor 1\n' \
'STABLE_gitMinor 6+\n'

FAKE_DESCRIBE_FROM_FAMILY_RESPONSE = """
archiveSizeBytes: '1581831882'
creationTimestamp: '2017-06-16T10:37:57.681-07:00'
description: 'Google, Container-Optimized OS, 59-9460.64.0 stable, Kernel: ChromiumOS-4.4.52
  Kubernetes: 1.6.4 Docker: 1.11.2'
diskSizeGb: '10'
family: cos-stable
id: '2388425242502080922'
kind: compute#image
labelFingerprint: 42WmSpB8rSM=
licenses:
- https://www.googleapis.com/compute/v1/projects/cos-cloud/global/licenses/cos
name: cos-stable-59-9460-64-0
rawDisk:
  containerType: TAR
  source: ''
selfLink: https://www.googleapis.com/compute/v1/projects/cos-cloud/global/images/cos-stable-59-9460-64-0
sourceType: RAW
status: READY
"""

def fake_pass(*_unused, **_unused2):
    """Do nothing."""
    pass

def fake_bomb(*a, **kw):
    """Always raise."""
    raise AssertionError('Should not happen', a, kw)

def raise_urllib2_error(*_unused, **_unused2):
    """Always raise a urllib2.URLError"""
    raise urllib2.URLError("test failure")

def always_kubernetes(*_unused, **_unused2):
    """Always return 'kubernetes'"""
    return 'kubernetes'

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


class ClusterNameTest(unittest.TestCase):
    def test_name_filled(self):
        """Return the cluster name if set."""
        name = 'foo'
        build = '1984'
        os.environ['BUILD_ID'] = build
        actual = kubernetes_e2e.cluster_name(name)
        self.assertTrue(actual)
        self.assertIn(name, actual)
        self.assertNotIn(build, actual)

    def test_name_empty_short_build(self):
        """Return the build number if name is empty."""
        name = ''
        build = '1984'
        os.environ['BUILD_ID'] = build
        actual = kubernetes_e2e.cluster_name(name)
        self.assertTrue(actual)
        self.assertIn(build, actual)

    def test_name_empty_long_build(self):
        """Return a short hash of a long build number if name is empty."""
        name = ''
        build = '0' * 63
        os.environ['BUILD_ID'] = build
        actual = kubernetes_e2e.cluster_name(name)
        self.assertTrue(actual)
        self.assertNotIn(build, actual)
        if len(actual) > 32:  # Some firewall names consume half the quota
            self.fail('Name should be short: %s' % actual)

    def test_name_presubmit(self):
        """Return the build number if name is empty."""
        name = ''
        build = '1984'
        pr = '12345'
        os.environ['BUILD_ID'] = build
        os.environ['JOB_TYPE'] = 'presubmit'
        os.environ['PULL_NUMBER'] = pr
        actual = kubernetes_e2e.cluster_name(name, False)
        self.assertTrue(actual)
        self.assertIn(build, actual)
        self.assertNotIn(pr, actual)

        actual = kubernetes_e2e.cluster_name(name, True)
        self.assertTrue(actual)
        self.assertIn(pr, actual)
        self.assertNotIn(build, actual)


class ScenarioTest(unittest.TestCase):  # pylint: disable=too-many-public-methods
    """Test for e2e scenario."""
    callstack = []
    envs = {}

    def setUp(self):
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
        """fake a workstatus blob."""
        self.callstack.append(string.join(cmd))
        return FAKE_WORKSPACE_STATUS

    def fake_output_work_status_v1_6(self, *cmd):
        """fake a workstatus blob for v1.6."""
        self.callstack.append(string.join(cmd))
        return FAKE_WORKSPACE_STATUS_V1_6

    def fake_output_get_latest_image(self, *cmd):
        """fake a `gcloud compute images describe-from-family` response."""
        self.callstack.append(string.join(cmd))
        return FAKE_DESCRIBE_FROM_FAMILY_RESPONSE

    def test_local(self):
        """Make sure local mode is fine overall."""
        args = kubernetes_e2e.parse_args()
        with Stub(kubernetes_e2e, 'check_env', self.fake_check_env):
            kubernetes_e2e.main(args)

        self.assertNotEqual(self.envs, {})
        for call in self.callstack:
            self.assertFalse(call.startswith('docker'))

    def test_check_leaks(self):
        """Ensure --check-leaked-resources=true sends flag to kubetest."""
        args = kubernetes_e2e.parse_args(['--check-leaked-resources=true'])
        with Stub(kubernetes_e2e, 'check_env', self.fake_check_env):
            kubernetes_e2e.main(args)
            self.assertIn('--check-leaked-resources=true', self.callstack[-1])

    def test_check_leaks_false(self):
        """Ensure --check-leaked-resources=true sends flag to kubetest."""
        args = kubernetes_e2e.parse_args(['--check-leaked-resources=false'])
        with Stub(kubernetes_e2e, 'check_env', self.fake_check_env):
            kubernetes_e2e.main(args)
            self.assertIn('--check-leaked-resources=false', self.callstack[-1])

    def test_check_leaks_default(self):
        """Ensure --check-leaked-resources=true sends flag to kubetest."""
        args = kubernetes_e2e.parse_args(['--check-leaked-resources'])
        with Stub(kubernetes_e2e, 'check_env', self.fake_check_env):
            kubernetes_e2e.main(args)
            self.assertIn('--check-leaked-resources', self.callstack[-1])

    def test_check_leaks_unset(self):
        """Ensure --check-leaked-resources=true sends flag to kubetest."""
        args = kubernetes_e2e.parse_args(['--mode=local'])
        with Stub(kubernetes_e2e, 'check_env', self.fake_check_env):
            kubernetes_e2e.main(args)
            self.assertNotIn('--check-leaked-resources', self.callstack[-1])

    def test_migrated_kubetest_args(self):
        migrated = [
            '--stage-suffix=panda',
            '--random-flag', 'random-value',
            '--multiple-federations',
            'arg1', 'arg2',
            '--federation',
            '--kubemark',
            '--extract=this',
            '--extract=that',
            '--save=somewhere',
            '--skew',
            '--publish=location',
            '--timeout=42m',
            '--upgrade_args=ginkgo',
            '--check-leaked-resources=true',
            '--charts',
        ]
        explicit_passthrough_args = [
            '--deployment=yay',
            '--provider=gce',
        ]
        args = kubernetes_e2e.parse_args(migrated
                                         + explicit_passthrough_args
                                         + ['--test=false'])
        self.assertEquals(migrated, args.kubetest_args)
        with Stub(kubernetes_e2e, 'check_env', self.fake_check_env):
            kubernetes_e2e.main(args)
        lastcall = self.callstack[-1]
        for arg in migrated:
            self.assertIn(arg, lastcall)
        for arg in explicit_passthrough_args:
            self.assertIn(arg, lastcall)

    def test_updown_default(self):
        args = kubernetes_e2e.parse_args([])
        with Stub(kubernetes_e2e, 'check_env', self.fake_check_env):
            kubernetes_e2e.main(args)
        lastcall = self.callstack[-1]
        self.assertIn('--up', lastcall)
        self.assertIn('--down', lastcall)

    def test_updown_set(self):
        args = kubernetes_e2e.parse_args(['--up=false', '--down=true'])
        with Stub(kubernetes_e2e, 'check_env', self.fake_check_env):
            kubernetes_e2e.main(args)
        lastcall = self.callstack[-1]
        self.assertNotIn('--up', lastcall)
        self.assertIn('--down', lastcall)


    def test_kubeadm_ci(self):
        """Make sure kubeadm ci mode is fine overall."""
        args = kubernetes_e2e.parse_args(['--kubeadm=ci'])
        self.assertEqual(args.kubeadm, 'ci')
        with Stub(kubernetes_e2e, 'check_env', self.fake_check_env):
            with Stub(kubernetes_e2e, 'check_output', self.fake_output_work_status):
                kubernetes_e2e.main(args)

        self.assertNotIn('E2E_OPT', self.envs)
        version = 'gs://kubernetes-release-dev/ci/v1.7.0-alpha.0.1320+599539dc0b9997-bazel/bin/linux/amd64/'  # pylint: disable=line-too-long
        self.assertIn('--kubernetes-anywhere-kubeadm-version=%s' % version, self.callstack[-1])
        called = False
        for call in self.callstack:
            self.assertFalse(call.startswith('docker'))
            if call == 'hack/print-workspace-status.sh':
                called = True
        self.assertTrue(called)

    def test_local_env(self):
        """
            Ensure that host variables (such as GOPATH) are included,
            and added envs/env files overwrite os environment.
        """
        mode = kubernetes_e2e.LocalMode('/orig-workspace', '/random-artifacts')
        mode.add_environment(*(
            'FOO=BAR', 'GOPATH=/go/path', 'WORKSPACE=/new/workspace'))
        mode.add_os_environment(*('USER=jenkins', 'FOO=BAZ', 'GOOS=linux'))
        with tempfile.NamedTemporaryFile() as temp:
            temp.write('USER=prow')
            temp.flush()
            mode.add_file(temp.name)
        with Stub(kubernetes_e2e, 'check_env', self.fake_check_env):
            mode.start([])
        self.assertIn(('FOO', 'BAR'), self.envs.viewitems())
        self.assertIn(('WORKSPACE', '/new/workspace'), self.envs.viewitems())
        self.assertIn(('GOPATH', '/go/path'), self.envs.viewitems())
        self.assertIn(('USER', 'prow'), self.envs.viewitems())
        self.assertIn(('GOOS', 'linux'), self.envs.viewitems())
        self.assertNotIn(('USER', 'jenkins'), self.envs.viewitems())
        self.assertNotIn(('FOO', 'BAZ'), self.envs.viewitems())

    def test_kubeadm_periodic(self):
        """Make sure kubeadm periodic mode is fine overall."""
        args = kubernetes_e2e.parse_args(['--kubeadm=periodic'])
        self.assertEqual(args.kubeadm, 'periodic')
        with Stub(kubernetes_e2e, 'check_env', self.fake_check_env):
            with Stub(kubernetes_e2e, 'check_output', self.fake_output_work_status):
                kubernetes_e2e.main(args)

        self.assertNotIn('E2E_OPT', self.envs)
        version = 'gs://kubernetes-release-dev/ci/v1.7.0-alpha.0.1320+599539dc0b9997-bazel/bin/linux/amd64/'  # pylint: disable=line-too-long
        self.assertIn('--kubernetes-anywhere-kubeadm-version=%s' % version, self.callstack[-1])
        called = False
        for call in self.callstack:
            self.assertFalse(call.startswith('docker'))
            if call == 'hack/print-workspace-status.sh':
                called = True
        self.assertTrue(called)

    def test_kubeadm_pull(self):
        """Make sure kubeadm pull mode is fine overall."""
        args = kubernetes_e2e.parse_args([
            '--kubeadm=pull',
            '--use-shared-build=bazel'
        ])
        self.assertEqual(args.kubeadm, 'pull')
        self.assertEqual(args.use_shared_build, 'bazel')

        gcs_bucket = "gs://kubernetes-release-dev/bazel/v1.8.0-beta.1.132+599539dc0b9997"

        def fake_gcs_path(path):
            bazel_default = os.path.join(
                'gs://kubernetes-jenkins/shared-results', 'bazel-build-location.txt')
            self.assertEqual(path, bazel_default)
            return gcs_bucket
        with Stub(kubernetes_e2e, 'check_env', self.fake_check_env):
            with Stub(kubernetes_e2e, 'read_gcs_path', fake_gcs_path):
                kubernetes_e2e.main(args)

        self.assertNotIn('E2E_OPT', self.envs)
        version = '%s/bin/linux/amd64/' % gcs_bucket
        self.assertIn('--kubernetes-anywhere-kubeadm-version=%s' % version, self.callstack[-1])

    def test_kubeadm_invalid(self):
        """Make sure kubeadm invalid mode exits unsuccessfully."""
        with self.assertRaises(SystemExit) as sysexit:
            kubernetes_e2e.parse_args(['--mode=local', '--kubeadm=deploy'])

        self.assertEqual(sysexit.exception.code, 2)

    def test_parse_args_order_agnostic(self):
        args = kubernetes_e2e.parse_args([
            '--some-kubetest-arg=foo',
            '--cluster=test'])
        self.assertEqual(args.kubetest_args, ['--some-kubetest-arg=foo'])
        self.assertEqual(args.cluster, 'test')

    def test_gcp_network(self):
        args = kubernetes_e2e.parse_args(['--mode=local', '--cluster=test'])
        with Stub(kubernetes_e2e, 'check_env', self.fake_check_env):
            kubernetes_e2e.main(args)
        lastcall = self.callstack[-1]
        self.assertIn('--gcp-network=test', lastcall)

    def test_env_local(self):
        env = 'FOO'
        value = 'BLAT'
        args = kubernetes_e2e.parse_args([
            '--mode=local',
            '--env={env}={value}'.format(env=env, value=value),
        ])
        with Stub(kubernetes_e2e, 'check_env', self.fake_check_env):
            kubernetes_e2e.main(args)
        self.assertIn(env, self.envs)
        self.assertEqual(self.envs[env], value)

    def test_aws(self):
        temp = tempfile.NamedTemporaryFile()
        args = kubernetes_e2e.parse_args([
            '--aws',
            '--cluster=foo',
            '--aws-cluster-domain=test-aws.k8s.io',
            '--aws-ssh=%s' % temp.name,
            '--aws-pub=%s' % temp.name,
            '--aws-cred=%s' % temp.name,
            ])
        with Stub(kubernetes_e2e, 'check_env', self.fake_check_env):
            kubernetes_e2e.main(args)

        lastcall = self.callstack[-1]
        self.assertIn('kops-e2e-runner.sh', lastcall)
        self.assertIn('--kops-cluster=foo.test-aws.k8s.io', lastcall)
        self.assertIn('--kops-zones', lastcall)
        self.assertIn('--kops-state=s3://k8s-kops-prow/', lastcall)
        self.assertIn('--kops-nodes=4', lastcall)
        self.assertIn('--kops-ssh-key', lastcall)

        self.assertNotIn('kubetest', lastcall)
        self.assertIn('kops-e2e-runner.sh', lastcall)

        self.assertEqual(
            self.envs['JENKINS_AWS_SSH_PRIVATE_KEY_FILE'], temp.name)
        self.assertEqual(
            self.envs['JENKINS_AWS_SSH_PUBLIC_KEY_FILE'], temp.name)
        self.assertEqual(
            self.envs['JENKINS_AWS_CREDENTIALS_FILE'], temp.name)

    def test_kops_aws(self):
        temp = tempfile.NamedTemporaryFile()
        args = kubernetes_e2e.parse_args([
            '--provider=aws',
            '--deployment=kops',
            '--cluster=foo.example.com',
            '--aws-ssh=%s' % temp.name,
            '--aws-pub=%s' % temp.name,
            '--aws-cred=%s' % temp.name,
            ])
        with Stub(kubernetes_e2e, 'check_env', self.fake_check_env):
            kubernetes_e2e.main(args)

        lastcall = self.callstack[-1]
        self.assertIn('kubetest', lastcall)
        self.assertIn('--provider=aws', lastcall)
        self.assertIn('--deployment=kops', lastcall)
        self.assertIn('--kops-cluster=foo.example.com', lastcall)
        self.assertIn('--kops-zones', lastcall)
        self.assertIn('--kops-state=s3://k8s-kops-prow/', lastcall)
        self.assertIn('--kops-nodes=4', lastcall)
        self.assertIn('--kops-ssh-key', lastcall)
        self.assertIn('kubetest', lastcall)
        self.assertNotIn('kops-e2e-runner.sh', lastcall)

    def test_kops_gce(self):
        temp = tempfile.NamedTemporaryFile()
        args = kubernetes_e2e.parse_args([
            '--provider=gce',
            '--deployment=kops',
            '--cluster=foo.example.com',
            '--gce-ssh=%s' % temp.name,
            '--gce-pub=%s' % temp.name,
            ])
        with Stub(kubernetes_e2e, 'check_env', self.fake_check_env):
            kubernetes_e2e.main(args)

        lastcall = self.callstack[-1]
        self.assertIn('kubetest', lastcall)
        self.assertIn('--provider=gce', lastcall)
        self.assertIn('--deployment=kops', lastcall)
        self.assertIn('--kops-cluster=foo.example.com', lastcall)
        self.assertIn('--kops-zones', lastcall)
        self.assertIn('--kops-state=gs://k8s-kops-gce/', lastcall)
        self.assertIn('--kops-nodes=4', lastcall)
        self.assertIn('--kops-ssh-key', lastcall)

    def test_use_shared_build(self):
        # normal path
        args = kubernetes_e2e.parse_args([
            '--use-shared-build=bazel'
        ])
        def expect_bazel_gcs(path):
            bazel_default = os.path.join(
                'gs://kubernetes-jenkins/shared-results', 'bazel-build-location.txt')
            self.assertEqual(path, bazel_default)
            return always_kubernetes()
        with Stub(kubernetes_e2e, 'check_env', self.fake_check_env):
            with Stub(kubernetes_e2e, 'read_gcs_path', expect_bazel_gcs):
                with Stub(time, 'sleep', fake_pass):
                    kubernetes_e2e.main(args)
        lastcall = self.callstack[-1]
        self.assertIn('--extract=kubernetes', lastcall)
        # normal path, not bazel
        args = kubernetes_e2e.parse_args([
            '--use-shared-build'
        ])
        def expect_normal_gcs(path):
            bazel_default = os.path.join(
                'gs://kubernetes-jenkins/shared-results', 'build-location.txt')
            self.assertEqual(path, bazel_default)
            return always_kubernetes()
        with Stub(kubernetes_e2e, 'check_env', self.fake_check_env):
            with Stub(kubernetes_e2e, 'read_gcs_path', expect_normal_gcs):
                kubernetes_e2e.main(args)
        lastcall = self.callstack[-1]
        self.assertIn('--extract=kubernetes', lastcall)
        # test failure to read shared path from GCS
        with Stub(kubernetes_e2e, 'check_env', self.fake_check_env):
            with Stub(kubernetes_e2e, 'read_gcs_path', raise_urllib2_error):
                with Stub(os, 'getcwd', always_kubernetes):
                    with Stub(time, 'sleep', fake_pass):
                        try:
                            kubernetes_e2e.main(args)
                        except RuntimeError as err:
                            if not err.message.startswith('Failed to get shared build location'):
                                raise err

if __name__ == '__main__':
    unittest.main()
