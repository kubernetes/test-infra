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

"""Tests for bootstrap."""

import json
import os
import subprocess
import unittest

import bootstrap


BRANCH = 'random_branch'
FAIL = ['/bin/bash', '-c', 'exit 1']
JOB = 'random_job'
PASS = ['/bin/bash', '-c', 'exit 0']
PULL = 12345
REPO = 'random_org/random_repo'


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


class FakeSubprocess(object):
    """Keep track of calls."""
    def __init__(self):
        self.calls = []

    def __call__(self, cmd, *a, **kw):
        self.calls.append((cmd, a, kw))


def Pass(*a, **kw):
    """Do nothing."""
    pass


def Truth(*a, **kw):
    """Always true."""
    return True


def Bomb(*a, **kw):
    """Always raise."""
    raise AssertionError('Should not happen', a, kw)


class SubprocessTest(unittest.TestCase):
    """Tests for Subprocess()."""

    def testStdin(self):
        """Will write to subprocess.stdin."""
        with self.assertRaises(subprocess.CalledProcessError) as cpe:
            bootstrap.Subprocess(['/bin/bash'], stdin='exit 92')
        self.assertEquals(92, cpe.exception.returncode)

    def testCheckTrue(self):
        """Raise on non-zero exit codes if check is set."""
        with self.assertRaises(subprocess.CalledProcessError) as cpe:
            bootstrap.Subprocess(FAIL, check=True)

        bootstrap.Subprocess(PASS, check=True)

    def testCheckDefault(self):
        """Default to check=True."""
        with self.assertRaises(subprocess.CalledProcessError) as cpe:
            bootstrap.Subprocess(FAIL)

        bootstrap.Subprocess(PASS)

    def testCheckFalse(self):
        """Never raise when check is not set."""
        bootstrap.Subprocess(FAIL, check=False)
        bootstrap.Subprocess(PASS, check=False)

    def testOutput(self):
        """Output is returned when requested."""
        cmd = ['/bin/bash', '-c', 'echo hello world']
        self.assertEquals(
            'hello world\n', bootstrap.Subprocess(cmd, output=True))

class CheckoutTest(unittest.TestCase):
    """Tests for Checkout()."""

    def testPull(self):
        """Checkout fetches the right ref for a pull."""
        with Stub(bootstrap, 'Subprocess', FakeSubprocess()) as fake:
            with Stub(os, 'chdir', Pass):
                bootstrap.Checkout(REPO, None, PULL)

        expected_ref = bootstrap.PullRef(PULL)
        self.assertTrue(any(
            expected_ref in cmd for cmd, _, _ in fake.calls if 'fetch' in cmd))

    def testBranch(self):
        """Checkout fetches the right ref for a branch."""
        with Stub(bootstrap, 'Subprocess', FakeSubprocess()) as fake:
            with Stub(os, 'chdir', Pass):
                bootstrap.Checkout(REPO, BRANCH, None)

        expected_ref = BRANCH
        self.assertTrue(any(
            expected_ref in cmd for cmd, _, _ in fake.calls if 'fetch' in cmd))

    def testRepo(self):
        """Checkout initializes and fetches the right repo."""
        with Stub(bootstrap, 'Subprocess', FakeSubprocess()) as fake:
            with Stub(os, 'chdir', Pass):
                bootstrap.Checkout(REPO, BRANCH, None)

        expected_uri = 'https://github.com/%s' % REPO
        self.assertTrue(any(
            expected_uri in cmd for cmd, _, _ in fake.calls if 'fetch' in cmd))

    def testBranchXorPull(self):
        """Either branch or pull specified, not both."""
        with Stub(bootstrap, 'Subprocess', Bomb), Stub(os, 'chdir', Bomb):
            with self.assertRaises(ValueError):
              bootstrap.Checkout(REPO, None, None)
            with self.assertRaises(ValueError):
              bootstrap.Checkout(REPO, BRANCH, PULL)

    def testHappy(self):
        """Checkout sanity check."""
        with Stub(bootstrap, 'Subprocess', FakeSubprocess()) as fake:
            with Stub(os, 'chdir', Pass):
                bootstrap.Checkout(REPO, BRANCH, None)

        self.assertTrue(any(
            '--tags' in cmd for cmd, _, _ in fake.calls if 'fetch' in cmd))
        self.assertTrue(any(
            'FETCH_HEAD' in cmd for cmd, _, _ in fake.calls
            if 'checkout' in cmd))


class GSUtilTest(unittest.TestCase):
    """Tests for GSUtil."""
    def testUploadJson(self):
        gsutil = bootstrap.GSUtil()
        with Stub(bootstrap, 'Subprocess', FakeSubprocess()) as fake:
            gsutil.UploadJson('fake_path', {'wee': 'fun'})
        self.assertTrue(any(
            'application/json' in a for a in fake.calls[0][0]))
        self.assertIn('stdin', fake.calls[0][2])  # kwargs

    def testUploadText_Cached(self):
        gsutil = bootstrap.GSUtil()
        with Stub(bootstrap, 'Subprocess', FakeSubprocess()) as fake:
            gsutil.UploadText('fake_path', 'hello world', cached=True)
        self.assertFalse(any(
            'Cache-Control' in a and 'max-age' in a
            for a in fake.calls[0][0]))
        self.assertIn('stdin', fake.calls[0][2])  # kwargs

    def testUploadText_Default(self):
        gsutil = bootstrap.GSUtil()
        with Stub(bootstrap, 'Subprocess', FakeSubprocess()) as fake:
            gsutil.UploadText('fake_path', 'hello world')
        self.assertFalse(any(
            'Cache-Control' in a and 'max-age' in a
            for a in fake.calls[0][0]))
        self.assertIn('stdin', fake.calls[0][2])  # kwargs

    def testUploadText_Uncached(self):
        gsutil = bootstrap.GSUtil()
        with Stub(bootstrap, 'Subprocess', FakeSubprocess()) as fake:
            gsutil.UploadText('fake_path', 'hello world', cached=False)
        self.assertTrue(any(
            'Cache-Control' in a and 'max-age' in a
            for a in fake.calls[0][0]))
        self.assertIn('stdin', fake.calls[0][2])  # kwargs


class FakeGSUtil(object):
    def __init__(self):
        self.texts = []
        self.jsons = []

    def UploadText(self, *args, **kwargs):
        self.texts.append((args, kwargs))

    def UploadJson(self, *args, **kwargs):
        self.jsons.append((args, kwargs))


class AppendBuildTest(unittest.TestCase):
    """Tests for AppendBuild()."""
    def testHandleJunk(self):
        gsutil = FakeGSUtil()
        build = 123
        version = 'v.interesting'
        success = True
        with Stub(bootstrap, 'Subprocess', lambda *a, **kw: '!@!$!@$@!$'):
            bootstrap.AppendBuild(gsutil, 'fake_path', build, version, success)
        cache = gsutil.jsons[0][0][1]
        self.assertEquals(1, len(cache))
        self.assertIn(build, cache[0].values())
        self.assertIn(version, cache[0].values())

    def testPassedIsBool(self):
        build = 123
        version = 'v.interesting'
        def Try(success):
            gsutil = FakeGSUtil()
            with Stub(bootstrap, 'Subprocess', lambda *a, **kw: ''):
                bootstrap.AppendBuild(gsutil, 'fake_path', build, version, success)
            cache = gsutil.jsons[0][0][1]
            self.assertTrue(isinstance(cache[0]['passed'], bool))

        Try(1)
        Try(0)
        Try(None)
        Try('')
        Try('hello')
        Try('true')

    def testTruncate(self):
        old = json.dumps({n: True for n in range(100000)})
        gsutil = FakeGSUtil()
        build = 123
        version = 'v.interesting'
        success = True
        with Stub(bootstrap, 'Subprocess', lambda *a, **kw: old):
            bootstrap.AppendBuild(gsutil, 'fake_path', build, version, success)
        cache = gsutil.jsons[0][0][1]
        self.assertLess(len(cache), len(old))



class FinishTest(unittest.TestCase):
    """Tests for Finish()."""
    def setUp(self):
      self.stubs = [
          Stub(bootstrap, 'UploadArtifacts', Pass),
          Stub(bootstrap, 'AppendBuild', Pass),
          Stub(os.path, 'isfile', Pass),
          Stub(os.path, 'isdir', Pass),
      ]

    def tearDown(self):
        for stub in self.stubs:
            with stub:
                pass

    def testUploadArtifacts(self):
        paths = FakePath()
        gsutil = FakeGSUtil()
        local_artifacts = None
        build = 123
        version = 'v1.terrible'
        success = True
        with Stub(os.path, 'isdir', lambda _: False):
            with Stub(bootstrap, 'UploadArtifacts', Bomb):
                bootstrap.Finish(
                    gsutil, paths, success, local_artifacts, build, version)


SECONDS = 10

class BuildTest(unittest.TestCase):
    """Tests for Build()."""

    def testAuto(self):
        """Automatically select a build if not done by user."""
        with Stub(os, 'environ', {}) as fake:
            bootstrap.Build(SECONDS)
            self.assertTrue(fake[bootstrap.BUILD_ENV])

    def testManual(self):
        """Respect user-selected build."""
        with Stub(os, 'environ', {}) as fake:
            truth = 'erick is awesome'
            fake[bootstrap.BUILD_ENV] = truth
            self.assertEquals(truth, fake[bootstrap.BUILD_ENV])

    def testUnique(self):
        """New build every minute."""
        with Stub(os, 'environ', {}) as fake:
            bootstrap.Build(SECONDS)
            first = fake[bootstrap.BUILD_ENV]
            del fake[bootstrap.BUILD_ENV]
            bootstrap.Build(SECONDS + 60)
            self.assertNotEqual(first, fake[bootstrap.BUILD_ENV])



class SetupCredentialsTest(unittest.TestCase):
    """Tests for SetupCredentials()."""

    def testRequireGoogleApplicationCredentials(self):
        """Raise if GOOGLE_APPLICATION_CREDENTIALS does not exist."""
        with Stub(os, 'environ', {}) as fake:
            gac = 'FAKE_CREDS.json'
            fake['HOME'] = 'kansas'
            fake[bootstrap.SERVICE_ACCOUNT_PATH] = gac
            with Stub(os.path, 'isfile', lambda p: p != gac):
                with self.assertRaises(IOError):
                    bootstrap.SetupCredentials()

            with Stub(os.path, 'isfile', Truth):
                with Stub(bootstrap, 'Subprocess', Pass):
                    bootstrap.SetupCredentials()

    def testRequireGCEKey(self):
        """Raise if the private gce does not exist."""
        with Stub(os, 'environ', {}) as fake:
            pkf = 'FAKE_PRIVATE_KEY'
            fake['HOME'] = 'kansas'
            fake[bootstrap.GCE_PRIVATE_KEY] = pkf
            with Stub(os.path, 'isfile', lambda p: p != pkf):
                with self.assertRaises(IOError):
                    bootstrap.SetupCredentials()

            with Stub(os.path, 'isfile', Truth):
                with Stub(bootstrap, 'Subprocess', Pass):
                    bootstrap.SetupCredentials()

    def testHappy(self):
        with Stub(os, 'environ', {}) as env:
            env['HOME'] = 'kansas'
            with Stub(os.path, 'isfile', Truth):
                with Stub(bootstrap, 'Subprocess', FakeSubprocess()) as sp:
                    bootstrap.SetupCredentials()


        self.assertEquals(env['WORKSPACE'], env['HOME'])
        self.assertTrue(env['WORKSPACE'])
        self.assertTrue(env['CLOUDSDK_CONFIG'])
        self.assertTrue(any(
            bootstrap.KeyFlag(env[bootstrap.SERVICE_ACCOUNT_PATH]) in cmd
            for cmd, _, _ in sp.calls if 'activate-service-account' in cmd))


class FakePath(object):
    artifacts = 'fake_artifacts'
    build_latest = 'fake_build_latest'
    build_link = 'fake_build_link'
    build_log = 'fake_build_log_path'
    build_path = 'fake_build_path'
    build_result_cache = 'fake_build_result_cache'
    latest = 'fake_latest'
    result_cache = 'fake_result_cache'
    started = 'fake_started.json'
    finished = 'fake_finished.json'
    def __call__(self, *a, **kw):
        self.a = a
        self.kw = kw
        return self


class FakeLogging(object):
    close = Pass
    def __call__(self, *a, **kw):
        return self


class FakeFinish(object):
    called = False
    result = None
    def __call__(self, unused_a, unused_b, success, *a, **kw):
        self.called = True
        self.result = success

class SetupBootstrap(unittest.TestCase):

    def setUp(self):
        self.boiler = [
            Stub(os, 'environ', {}),
            Stub(bootstrap, 'Checkout', Pass),
            Stub(bootstrap, 'SetupLogging', FakeLogging()),
            Stub(bootstrap, 'SetupCredentials', Pass),
            Stub(bootstrap, 'Start', Pass),
            Stub(bootstrap, 'Subprocess', Pass),
            Stub(bootstrap, 'Finish', Pass),
            Stub(bootstrap.GSUtil, 'CopyFile', Pass),
        ]

    def tearDown(self):
        for stub in self.boiler:
            with stub:  # Leaving with restores things
                pass

    def testPRPath(self):
        """Use a PRPath when pull is set."""

        with Stub(bootstrap, 'CIPath', Bomb):
            with Stub(bootstrap, 'PRPath', FakePath()) as path:
                bootstrap.Bootstrap(JOB, REPO, None, PULL)
            self.assertTrue(any(
                str(PULL) in o for o in (path.a, path.kw)))

    def testCIPath(self):
        """Use a CIPath when branch is set."""

        with Stub(bootstrap, 'PRPath', Bomb):
            with Stub(bootstrap, 'CIPath', FakePath()) as path:
                bootstrap.Bootstrap(JOB, REPO, BRANCH, None)
            self.assertFalse(any(
                str(PULL) in o for o in (path.a, path.kw)))

    def testNoFinishWhenStartFails(self):
        with Stub(bootstrap, 'Finish', FakeFinish()) as fake:
            with Stub(bootstrap, 'Start', Bomb):
                with self.assertRaises(AssertionError):
                    bootstrap.Bootstrap(JOB, REPO, BRANCH, None)
        self.assertFalse(fake.called)


    def testFinishWhenBuildFails(self):
        def CallError(*a, **kw):
            raise subprocess.CalledProcessError(1, [], '')
        with Stub(bootstrap, 'Finish', FakeFinish()) as fake:
            with Stub(bootstrap, 'Subprocess', CallError):
                bootstrap.Bootstrap(JOB, REPO, BRANCH, None)
        self.assertTrue(fake.called)
        self.assertTrue(fake.result is False)  # Distinguish from None

    def testHappy(self):
        with Stub(bootstrap, 'Finish', FakeFinish()) as fake:
            bootstrap.Bootstrap(JOB, REPO, BRANCH, None)
        self.assertTrue(fake.called)
        self.assertTrue(fake.result)  # Distinguish from None

    def testJobEnv(self):
        """Bootstrap sets JOB_NAME."""
        with Stub(os, 'environ', {}) as env:
            bootstrap.Bootstrap(JOB, REPO, BRANCH, None)
        self.assertIn(bootstrap.JOB_ENV, env)



if __name__ == '__main__':
    unittest.main()
