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
import re
import subprocess
import tempfile
import unittest

import bootstrap


BRANCH = 'random_branch'
BUILD = 'random_build'
FAIL = ['/bin/bash', '-c', 'exit 1']
JOB = 'random_job'
PASS = ['/bin/bash', '-c', 'exit 0']
PULL = 12345
REPO = 'random_org/random_repo'
ROOT = '/random/root'


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


class FakeCall(object):
    def __init__(self):
        self.calls = []

    def __call__(self, *a, **kw):
        self.calls.append((a, kw))


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


class AppendResultTest(unittest.TestCase):
    """Tests for AppendResult()."""
    def testHandleJunk(self):
        gsutil = FakeGSUtil()
        build = 123
        version = 'v.interesting'
        success = True
        with Stub(bootstrap, 'Subprocess', lambda *a, **kw: '!@!$!@$@!$'):
            bootstrap.AppendResult(gsutil, 'fake_path', build, version, success)
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
                bootstrap.AppendResult(gsutil, 'fake_path', build, version, success)
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
            bootstrap.AppendResult(gsutil, 'fake_path', build, version, success)
        cache = gsutil.jsons[0][0][1]
        self.assertLess(len(cache), len(old))



class FinishTest(unittest.TestCase):
    """Tests for Finish()."""
    def setUp(self):
      self.stubs = [
          Stub(bootstrap, 'UploadArtifacts', Pass),
          Stub(bootstrap, 'AppendResult', Pass),
          Stub(os.path, 'isfile', Pass),
          Stub(os.path, 'isdir', Pass),
      ]

    def tearDown(self):
        for stub in self.stubs:
            with stub:
                pass

    def testSkipUploadArtifacts(self):
        """Do not upload artifacts dir if it doesn't exist."""
        paths = FakePath()
        gsutil = FakeGSUtil()
        local_artifacts = None
        build = 123
        version = 'v1.terrible'
        success = True
        with Stub(os.path, 'isdir', lambda _: False):
            with Stub(bootstrap, 'UploadArtifacts', Bomb):
                bootstrap.Finish(
                    gsutil, paths, success, local_artifacts,
                    build, version, REPO)


class MetadataTest(unittest.TestCase):
    def testAlwaysSetMetadata(self):
        metadata = bootstrap.Metadata(REPO, 'missing-artifacts-dir')
        self.assertIn('repo', metadata)
        self.assertEquals(REPO, metadata['repo'])


SECONDS = 10


def FakeEnviron(
    set_home=True, set_node=True, set_job=True,
    **kwargs
):
    if set_home:
        kwargs.setdefault(bootstrap.HOME_ENV, '/fake/home-dir')
    if set_node:
        kwargs.setdefault(bootstrap.NODE_ENV, 'fake-node')
    if set_job:
        kwargs.setdefault(bootstrap.JOB_ENV, JOB)
    return kwargs


class BuildTest(unittest.TestCase):
    """Tests for Build()."""

    def testAuto(self):
        """Automatically select a build if not done by user."""
        with Stub(os, 'environ', FakeEnviron()) as fake:
            bootstrap.Build(SECONDS)
            self.assertTrue(fake[bootstrap.BUILD_ENV])

    def testManual(self):
        """Respect user-selected build."""
        with Stub(os, 'environ', FakeEnviron()) as fake:
            truth = 'erick is awesome'
            fake[bootstrap.BUILD_ENV] = truth
            self.assertEquals(truth, fake[bootstrap.BUILD_ENV])

    def testUnique(self):
        """New build every minute."""
        with Stub(os, 'environ', FakeEnviron()) as fake:
            bootstrap.Build(SECONDS)
            first = fake[bootstrap.BUILD_ENV]
            del fake[bootstrap.BUILD_ENV]
            bootstrap.Build(SECONDS + 60)
            self.assertNotEqual(first, fake[bootstrap.BUILD_ENV])



class SetupCredentialsTest(unittest.TestCase):
    """Tests for SetupCredentials()."""

    def setUp(self):
        keys = {
            bootstrap.GCE_KEY_ENV: 'fake-key',
            bootstrap.SERVICE_ACCOUNT_ENV: 'fake-service-account.json',
        }
        self.env = FakeEnviron(**keys)


    def testRequireGoogleApplicationCredentials(self):
        """Raise if GOOGLE_APPLICATION_CREDENTIALS does not exist."""
        del self.env[bootstrap.SERVICE_ACCOUNT_ENV]
        with Stub(os, 'environ', self.env) as fake:
            gac = 'FAKE_CREDS.json'
            fake['HOME'] = 'kansas'
            fake[bootstrap.SERVICE_ACCOUNT_ENV] = gac
            with Stub(os.path, 'isfile', lambda p: p != gac):
                with self.assertRaises(IOError):
                    bootstrap.SetupCredentials()

            with Stub(os.path, 'isfile', Truth):
                with Stub(bootstrap, 'Subprocess', Pass):
                    bootstrap.SetupCredentials()

    def testRequireGCEKey(self):
        """Raise if the private gce does not exist."""
        del self.env[bootstrap.GCE_KEY_ENV]
        with Stub(os, 'environ', self.env) as fake:
            pkf = 'FAKE_PRIVATE_KEY'
            fake['HOME'] = 'kansas'
            fake[bootstrap.GCE_KEY_ENV] = pkf
            with Stub(os.path, 'isfile', lambda p: p != pkf):
                with self.assertRaises(IOError):
                    bootstrap.SetupCredentials()

            with Stub(os.path, 'isfile', Truth):
                with Stub(bootstrap, 'Subprocess', Pass):
                    bootstrap.SetupCredentials()

class SetupMagicEnvironmentTest(unittest.TestCase):
    def testWorkspace(self):
        """WORKSPACE exists, equals HOME and is set to cwd."""
        env = FakeEnviron()
        cwd = '/fake/random-location'
        with Stub(os, 'environ', env):
            with Stub(os, 'getcwd', lambda: cwd):
                bootstrap.SetupMagicEnvironment(JOB)

        self.assertIn(bootstrap.WORKSPACE_ENV, env)
        self.assertEquals(env[bootstrap.HOME_ENV], env[bootstrap.WORKSPACE_ENV])
        self.assertEquals(cwd, env[bootstrap.WORKSPACE_ENV])

    def testJobEnvMismatch(self):
        env = FakeEnviron()
        with Stub(os, 'environ', env):
            with self.assertRaises(ValueError):
                bootstrap.SetupMagicEnvironment('this-is-a-job')

    def testExpected(self):
        env = FakeEnviron()
        del env[bootstrap.JOB_ENV]
        del env[bootstrap.NODE_ENV]
        with Stub(os, 'environ', env):
            bootstrap.SetupMagicEnvironment(JOB)

        def Check(name):
            self.assertIn(name, env)

        # Some of these are probably silly to check...
        # TODO(fejta): remove as many of these from our infra as possible.
        Check(bootstrap.NODE_ENV)
        Check(bootstrap.JOB_ENV)
        Check(bootstrap.CLOUDSDK_ENV)
        Check(bootstrap.BOOTSTRAP_ENV)
        Check(bootstrap.WORKSPACE_ENV)
        Check(bootstrap.SERVICE_ACCOUNT_ENV)

    def testCloudSdkConfig(self):
        cwd = 'now-here'
        env = FakeEnviron()
        with Stub(os, 'environ', env):
            with Stub(os, 'getcwd', lambda: cwd):
                bootstrap.SetupMagicEnvironment(JOB)


        self.assertTrue(env[bootstrap.CLOUDSDK_ENV].startswith(cwd))


class FakePath(object):
    artifacts = 'fake_artifacts'
    pr_latest = 'fake_pr_latest'
    pr_build_link = 'fake_pr_link'
    build_log = 'fake_log_path'
    pr_path = 'fake_pr_path'
    pr_result_cache = 'fake_pr_result_cache'
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

class PRPathsTest(unittest.TestCase):
    def testKubernetesKubernetes(self):
        """Test the kubernetes/kubernetes prefix."""
        path = bootstrap.PRPaths('kubernetes/kubernetes', JOB, BUILD, PULL)
        self.assertTrue(any(
            str(PULL) == p for p in path.build_log.split('/')))

    def testKubernetes(self):
        """Test the kubernetes/something prefix."""
        path = bootstrap.PRPaths('kubernetes/prefix', JOB, BUILD, PULL)
        self.assertTrue(any(
            'prefix%s' % PULL == p for p in path.build_log.split('/')))

    def testOther(self):
        """Test the none kubernetes prefixes."""
        path = bootstrap.PRPaths('random/repo', JOB, BUILD, PULL)
        self.assertTrue(any(
            'random_repo%s' % PULL == p for p in path.build_log.split('/')))


class BootstrapTest(unittest.TestCase):

    def setUp(self):
        self.boiler = [
            Stub(bootstrap, 'Checkout', Pass),
            Stub(bootstrap, 'Finish', Pass),
            Stub(bootstrap.GSUtil, 'CopyFile', Pass),
            Stub(bootstrap, 'Node', lambda: 'fake-node'),
            Stub(bootstrap, 'SetupCredentials', Pass),
            Stub(bootstrap, 'SetupLogging', FakeLogging()),
            Stub(bootstrap, 'Start', Pass),
            Stub(bootstrap, 'Subprocess', Pass),
            Stub(os, 'environ', FakeEnviron()),
            Stub(os, 'chdir', Pass),
            Stub(os, 'makedirs', Pass),
        ]

    def tearDown(self):
        for stub in self.boiler:
            with stub:  # Leaving with restores things
                pass

    def testRoot_NotExists(self):
        with Stub(os, 'chdir', FakeCall()) as fake_chdir:
            with Stub(os.path, 'exists', lambda p: False):
                with Stub(os, 'makedirs', FakeCall()) as fake_makedirs:
                    bootstrap.Bootstrap(JOB, REPO, None, PULL, ROOT)
        self.assertTrue(any(ROOT in c[0] for c in fake_chdir.calls), fake_chdir.calls)
        self.assertTrue(any(ROOT in c[0] for c in fake_makedirs.calls), fake_makedirs.calls)

    def testRoot_Exists(self):
        with Stub(os, 'chdir', FakeCall()) as fake_chdir:
            bootstrap.Bootstrap(JOB, REPO, None, PULL, ROOT)
        self.assertTrue(any(ROOT in c[0] for c in fake_chdir.calls))

    def testPRPaths(self):
        """Use a PRPaths when pull is set."""

        with Stub(bootstrap, 'CIPaths', Bomb):
            with Stub(bootstrap, 'PRPaths', FakePath()) as path:
                bootstrap.Bootstrap(JOB, REPO, None, PULL, ROOT)
            self.assertTrue(PULL in path.a or PULL in path.kw)

    def testCIPaths(self):
        """Use a CIPaths when branch is set."""

        with Stub(bootstrap, 'PRPaths', Bomb):
            with Stub(bootstrap, 'CIPaths', FakePath()) as path:
                bootstrap.Bootstrap(JOB, REPO, BRANCH, None, ROOT)
            self.assertFalse(any(
                PULL in o for o in (path.a, path.kw)))

    def testNoFinishWhenStartFails(self):
        with Stub(bootstrap, 'Finish', FakeFinish()) as fake:
            with Stub(bootstrap, 'Start', Bomb):
                with self.assertRaises(AssertionError):
                    bootstrap.Bootstrap(JOB, REPO, BRANCH, None, ROOT)
        self.assertFalse(fake.called)


    def testFinishWhenBuildFails(self):
        def CallError(*a, **kw):
            raise subprocess.CalledProcessError(1, [], '')
        with Stub(bootstrap, 'Finish', FakeFinish()) as fake:
            with Stub(bootstrap, 'Subprocess', CallError):
                with self.assertRaises(SystemExit):
                    bootstrap.Bootstrap(JOB, REPO, BRANCH, None, ROOT)
        self.assertTrue(fake.called)
        self.assertTrue(fake.result is False)  # Distinguish from None

    def testHappy(self):
        with Stub(bootstrap, 'Finish', FakeFinish()) as fake:
            bootstrap.Bootstrap(JOB, REPO, BRANCH, None, ROOT)
        self.assertTrue(fake.called)
        self.assertTrue(fake.result)  # Distinguish from None

    def testJobEnv(self):
        """Bootstrap sets JOB_NAME."""
        with Stub(os, 'environ', FakeEnviron()) as env:
            bootstrap.Bootstrap(JOB, REPO, BRANCH, None, ROOT)
        self.assertIn(bootstrap.JOB_ENV, env)


class IntegrationTest(unittest.TestCase):
    REPO = 'hello/world'
    MASTER = 'fake-master-file'
    BRANCH_FILE = 'fake-branch-file'
    PR_FILE = 'fake-pr-file'
    BRANCH = 'another-branch'
    PR = 42
    PR_TAG = bootstrap.PullRef(PR).strip('+')

    def FakeRepo(self, repo):
        return os.path.join(self.root_github, repo)

    def setUp(self):
        self.boiler = [
            Stub(bootstrap, 'Finish', Pass),
            Stub(bootstrap.GSUtil, 'CopyFile', Pass),
            Stub(bootstrap, 'Repo', self.FakeRepo),
            Stub(bootstrap, 'SetupCredentials', Pass),
            Stub(bootstrap, 'SetupLogging', FakeLogging()),
            Stub(bootstrap, 'Start', Pass),
            Stub(os, 'environ', FakeEnviron(set_job=False)),
        ]
        self.root_github = tempfile.mkdtemp()
        self.root_workspace = tempfile.mkdtemp()
        self.ocwd = os.getcwd()
        repo = self.FakeRepo(self.REPO)
        subprocess.check_call(['git', 'init', repo])
        os.chdir(repo)
        subprocess.check_call(['touch', self.MASTER])
        subprocess.check_call(['git', 'add', self.MASTER])
        subprocess.check_call(['git', 'commit', '-m', 'Initial commit'])
        subprocess.check_call(['git', 'checkout', 'master'])

    def tearDown(self):
        for stub in self.boiler:
            with stub:  # Leaving with restores things
                pass
        os.chdir(self.ocwd)
        subprocess.check_call(['rm', '-rf', self.root_github])
        subprocess.check_call(['rm', '-rf', self.root_workspace])

    def testPr(self):
        subprocess.check_call(['git', 'checkout', 'master'])
        subprocess.check_call(['git', 'checkout', '-b', 'unknown-pr-branch'])
        subprocess.check_call(['git', 'rm', self.MASTER])
        subprocess.check_call(['touch', self.PR_FILE])
        subprocess.check_call(['git', 'add', self.PR_FILE])
        subprocess.check_call(['git', 'commit', '-m', 'Create branch for PR %d' % self.PR])
        subprocess.check_call(['git', 'tag', self.PR_TAG])
        os.chdir('/tmp')
        bootstrap.Bootstrap('fake-pr', self.REPO, None, self.PR, self.root_workspace)

    def testBranch(self):
        subprocess.check_call(['git', 'checkout', '-b', self.BRANCH])
        subprocess.check_call(['git', 'rm', self.MASTER])
        subprocess.check_call(['touch', self.BRANCH_FILE])
        subprocess.check_call(['git', 'add', self.BRANCH_FILE])
        subprocess.check_call(['git', 'commit', '-m', 'Create %s' % self.BRANCH])

        os.chdir('/tmp')
        bootstrap.Bootstrap('fake-branch', self.REPO, self.BRANCH, None, self.root_workspace)

    def testPr_Bad(self):
        random_pr = 111
        with Stub(bootstrap, 'Start', Bomb):
            with self.assertRaises(subprocess.CalledProcessError):
                bootstrap.Bootstrap('fake-pr', self.REPO, None, random_pr, self.root_workspace)

    def testBranch_Bad(self):
        random_branch = 'something'
        with Stub(bootstrap, 'Start', Bomb):
            with self.assertRaises(subprocess.CalledProcessError):
                bootstrap.Bootstrap('fake-branch', self.REPO, random_branch, None, self.root_workspace)

    def testJobMissing(self):
        with self.assertRaises(subprocess.CalledProcessError):
            bootstrap.Bootstrap('this-job-no-exists', self.REPO, None, self.PR, self.root_workspace)

    def testJobFails(self):
        with self.assertRaises(subprocess.CalledProcessError):
            bootstrap.Bootstrap('fake-failure', self.REPO, None, self.PR, self.root_workspace)


class JobTest(unittest.TestCase):
    def testOnlyJobs(self):
        """Ensure that everything in jobs/ is a valid job name and script."""
        for path, _, filenames in os.walk(os.path.dirname(bootstrap.Job(JOB))):
            for job in filenames:
                # Jobs should have simple names
                self.assertTrue(re.match(r'[0-9a-z-]+.sh', job), job)
                job_path = os.path.join(path, job)
                # Jobs should point to a real, executable file
                # Note: it is easy to forget to chmod +x
                self.assertTrue(os.path.isfile(job_path), job_path)
                self.assertFalse(os.path.islink(job_path), job_path)
                self.assertTrue(os.access(job_path, os.X_OK|os.R_OK), job_path)


if __name__ == '__main__':
    unittest.main()
