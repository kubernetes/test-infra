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

# pylint: disable=too-many-public-methods, too-many-branches, too-many-locals
# pylint: disable=protected-access, attribute-defined-outside-init, too-many-statements

import argparse
import collections
import json
import os
import re
import select
import signal
import subprocess
import tempfile
import time
import unittest

import bootstrap

import yaml


BRANCH = 'random_branch'
BUILD = 'random_build'
FAIL = ['/bin/bash', '-c', 'exit 1']
JOB = 'random_job'
PASS = ['/bin/bash', '-c', 'exit 0']
PULL = 12345
REPO = 'github.com/random_org/random_repo'
ROBOT = 'fake-service-account.json'
ROOT = '/random/root'
UPLOAD = 'fake-gs://fake-bucket'


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


# pylint: disable=invalid-name
def Pass(*_a, **_kw):
    """Do nothing."""
    pass


def Truth(*_a, **_kw):
    """Always true."""
    return True


def Bomb(*a, **kw):
    """Always raise."""
    raise AssertionError('Should not happen', a, kw)
# pylint: enable=invalid-name


class ReadAllTest(unittest.TestCase):
    endless = 0
    ended = time.time() - 50
    number = 0
    end = -1

    def fileno(self):
        return self.end

    def readline(self):
        line = 'line %d\n' % self.number
        self.number += 1
        return line

    def test_read_more(self):
        """Read lines until we clear the buffer, noting there may be more."""
        lines = []
        total = 10
        def more_lines(*_a, **_kw):
            if len(lines) < total:
                return [self], [], []
            return [], [], []
        with Stub(select, 'select', more_lines):
            done = bootstrap.read_all(self.endless, self, lines.append)

        self.assertFalse(done)
        self.assertEquals(total, len(lines))
        expected = ['line %d' % d for d in range(total)]
        self.assertEquals(expected, lines)

    def test_read_expired(self):
        """Read nothing as we are expired, noting there may be more."""
        lines = []
        with Stub(select, 'select', lambda *a, **kw: ([], [], [])):
            done = bootstrap.read_all(self.ended, self, lines.append)

        self.assertFalse(done)
        self.assertFalse(lines)

    def test_read_end(self):
        """Note we reached the end of the stream."""
        lines = []
        with Stub(select, 'select', lambda *a, **kw: ([self], [], [])):
            with Stub(self, 'readline', lambda: ''):
                done = bootstrap.read_all(self.endless, self, lines.append)

        self.assertTrue(done)


class TerminateTest(unittest.TestCase):
    """Tests for termiante()."""
    pid = 1234
    pgid = 5555
    terminated = False
    killed = False

    def terminate(self):
        self.terminated = True

    def kill(self):
        self.killed = True

    def getpgid(self, pid):
        self.got = pid
        return self.pgid

    def killpg(self, pgig, sig):
        self.killed_pg = (pgig, sig)

    def test_terminate_later(self):
        """Do nothing if end is in the future."""
        timeout = bootstrap.terminate(time.time() + 50, self, False)
        self.assertFalse(timeout)

    def test_terminate_never(self):
        """Do nothing if end is zero."""
        timeout = bootstrap.terminate(0, self, False)
        self.assertFalse(timeout)

    def test_terminate_terminate(self):
        """Terminate pid if after end and kill is false."""
        timeout = bootstrap.terminate(time.time() - 50, self, False)
        self.assertTrue(timeout)
        self.assertFalse(self.killed)
        self.assertTrue(self.terminated)

    def test_terminate_kill(self):
        """Kill process group if after end and kill is true."""
        with Stub(os, 'getpgid', self.getpgid), Stub(os, 'killpg', self.killpg):
            timeout = bootstrap.terminate(time.time() - 50, self, True)
        self.assertTrue(timeout)
        self.assertFalse(self.terminated)
        self.assertTrue(self.killed)
        self.assertEquals(self.pid, self.got)
        self.assertEquals(self.killed_pg, (self.pgid, signal.SIGKILL))


class SubprocessTest(unittest.TestCase):
    """Tests for call()."""

    def test_stdin(self):
        """Will write to subprocess.stdin."""
        with self.assertRaises(subprocess.CalledProcessError) as cpe:
            bootstrap._call(0, ['/bin/bash'], stdin='exit 92')
        self.assertEquals(92, cpe.exception.returncode)

    def test_check_true(self):
        """Raise on non-zero exit codes if check is set."""
        with self.assertRaises(subprocess.CalledProcessError):
            bootstrap._call(0, FAIL, check=True)

        bootstrap._call(0, PASS, check=True)

    def test_check_default(self):
        """Default to check=True."""
        with self.assertRaises(subprocess.CalledProcessError):
            bootstrap._call(0, FAIL)

        bootstrap._call(0, PASS)

    @staticmethod
    def test_check_false():
        """Never raise when check is not set."""
        bootstrap._call(0, FAIL, check=False)
        bootstrap._call(0, PASS, check=False)

    def test_output(self):
        """Output is returned when requested."""
        cmd = ['/bin/bash', '-c', 'echo hello world']
        self.assertEquals(
            'hello world\n', bootstrap._call(0, cmd, output=True))

    def test_zombie(self):
        with self.assertRaises(subprocess.CalledProcessError):
            # make a zombie
            bootstrap._call(0, ['/bin/bash', '-c', 'A=$BASHPID && ( kill -STOP $A ) & exit 1'])


class PullRefsTest(unittest.TestCase):
    """Tests for pull_ref, branch_ref, ref_has_shas, and pull_numbers."""

    def test_multiple_no_shas(self):
        """Test master,1111,2222."""
        self.assertEqual(
            bootstrap.pull_ref('master,123,456'),
            ([
                'master',
                '+refs/pull/123/head:refs/pr/123',
                '+refs/pull/456/head:refs/pr/456',
            ], [
                'FETCH_HEAD',
                'refs/pr/123',
                'refs/pr/456',
            ]),
        )

    def test_pull_has_shas(self):
        self.assertTrue(bootstrap.ref_has_shas('master:abcd'))
        self.assertFalse(bootstrap.ref_has_shas('123'))
        self.assertFalse(bootstrap.ref_has_shas(123))
        self.assertFalse(bootstrap.ref_has_shas(None))

    def test_pull_numbers(self):
        self.assertListEqual(bootstrap.pull_numbers(123), ['123'])
        self.assertListEqual(bootstrap.pull_numbers('master:abcd'), [])
        self.assertListEqual(
            bootstrap.pull_numbers('master:abcd,123:qwer,124:zxcv'),
            ['123', '124'])

    def test_pull_ref(self):
        self.assertEqual(
            bootstrap.pull_ref('master:abcd,123:effe'),
            (['master', '+refs/pull/123/head:refs/pr/123'], ['abcd', 'effe'])
        )
        self.assertEqual(
            bootstrap.pull_ref('123'),
            (['+refs/pull/123/merge'], ['FETCH_HEAD'])
        )

    def test_branch_ref(self):
        self.assertEqual(
            bootstrap.branch_ref('branch:abcd'),
            (['branch'], ['abcd'])
        )
        self.assertEqual(
            bootstrap.branch_ref('master'),
            (['master'], ['FETCH_HEAD'])
        )


class ChooseSshKeyTest(unittest.TestCase):
    """Tests for choose_ssh_key()."""
    def test_empty(self):
        """Do not change environ if no ssh key."""
        fake_env = {}
        with Stub(os, 'environ', fake_env):
            with bootstrap.choose_ssh_key(''):
                self.assertFalse(fake_env)

    def test_full(self):
        fake_env = {}
        with Stub(os, 'environ', fake_env):
            with bootstrap.choose_ssh_key('hello there'):
                self.assertIn('GIT_SSH', fake_env)
                with open(fake_env['GIT_SSH']) as fp:
                    buf = fp.read()
                self.assertIn('hello there', buf)
                self.assertIn('ssh ', buf)
                self.assertIn(' -i ', buf)
            self.assertFalse(fake_env)  # Resets env

    def test_full_old_value(self):
        fake_env = {'GIT_SSH': 'random-value'}
        old_env = dict(fake_env)
        with Stub(os, 'environ', fake_env):
            with bootstrap.choose_ssh_key('hello there'):
                self.assertNotEqual(old_env, fake_env)
            self.assertEquals(old_env, fake_env)


class CheckoutTest(unittest.TestCase):
    """Tests for checkout()."""

    def test_clean(self):
        """checkout cleans and resets if asked to."""
        fake = FakeSubprocess()
        with Stub(os, 'chdir', Pass):
            bootstrap.checkout(fake, REPO, None, PULL, clean=True)

        self.assertTrue(any(
            'clean' in cmd for cmd, _, _ in fake.calls if 'git' in cmd))
        self.assertTrue(any(
            'reset' in cmd for cmd, _, _ in fake.calls if 'git' in cmd))

    def test_fetch_retries(self):
        self.tries = 0
        expected_attempts = 3
        def third_time_charm(cmd, *_a, **_kw):
            if 'fetch' not in cmd:  # init/checkout are unlikely to fail
                return
            self.tries += 1
            if self.tries != expected_attempts:
                raise subprocess.CalledProcessError(128, cmd, None)
        with Stub(os, 'chdir', Pass):
            with Stub(time, 'sleep', Pass):
                bootstrap.checkout(third_time_charm, REPO, None, PULL)
        self.assertEquals(expected_attempts, self.tries)

    def test_pull_ref(self):
        """checkout fetches the right ref for a pull."""
        fake = FakeSubprocess()
        with Stub(os, 'chdir', Pass):
            bootstrap.checkout(fake, REPO, None, PULL)

        expected_ref = bootstrap.pull_ref(PULL)[0][0]
        self.assertTrue(any(
            expected_ref in cmd for cmd, _, _ in fake.calls if 'fetch' in cmd))

    def test_branch(self):
        """checkout fetches the right ref for a branch."""
        fake = FakeSubprocess()
        with Stub(os, 'chdir', Pass):
            bootstrap.checkout(fake, REPO, BRANCH, None)

        expected_ref = BRANCH
        self.assertTrue(any(
            expected_ref in cmd for cmd, _, _ in fake.calls if 'fetch' in cmd))

    def test_repo(self):
        """checkout initializes and fetches the right repo."""
        fake = FakeSubprocess()
        with Stub(os, 'chdir', Pass):
            bootstrap.checkout(fake, REPO, BRANCH, None)

        expected_uri = 'https://%s' % REPO
        self.assertTrue(any(
            expected_uri in cmd for cmd, _, _ in fake.calls if 'fetch' in cmd))

    def test_branch_xor_pull(self):
        """Either branch or pull specified, not both."""
        with Stub(os, 'chdir', Bomb):
            with self.assertRaises(ValueError):
                bootstrap.checkout(Bomb, REPO, None, None)
            with self.assertRaises(ValueError):
                bootstrap.checkout(Bomb, REPO, BRANCH, PULL)

    def test_happy(self):
        """checkout sanity check."""
        fake = FakeSubprocess()
        with Stub(os, 'chdir', Pass):
            bootstrap.checkout(fake, REPO, BRANCH, None)

        self.assertTrue(any(
            '--tags' in cmd for cmd, _, _ in fake.calls if 'fetch' in cmd))
        self.assertTrue(any(
            'FETCH_HEAD' in cmd for cmd, _, _ in fake.calls
            if 'checkout' in cmd))

class ParseReposTest(unittest.TestCase):
    def test_bare(self):
        """--bare works."""
        self.assertFalse(
            bootstrap.parse_repos(FakeArgs(repo=[], branch=[], pull=[], bare=True)))

    def test_deprecated_branch(self):
        """--repo=foo --branch=bbb works"""
        self.assertEquals(
            {'foo': ('bbb', '')},
            bootstrap.parse_repos(FakeArgs(repo=['foo'], branch='bbb', pull='')))

    def test_deprecated_branch_commit(self):
        """--repo=foo --branch=bbb:1234 works"""
        self.assertEquals(
            {'foo': ('bbb:1234', '')},
            bootstrap.parse_repos(FakeArgs(repo=['foo'], branch='bbb:1234', pull='')))

    def test_depre_branch_repo_commit(self):
        """--repo=foo=master:aaa --branch=bar is not allowed"""
        with self.assertRaises(ValueError):
            bootstrap.parse_repos(FakeArgs(
                repo=['foo=master:aaa'], branch='master'))
        with self.assertRaises(ValueError):
            bootstrap.parse_repos(FakeArgs(
                repo=['foo=master'], branch='bar'))

    def test_deprecated_pull(self):
        """--repo=foo --pull=123 works."""
        self.assertEquals(
            {'foo': ('', '123:abc,333:ddd')},
            bootstrap.parse_repos(FakeArgs(repo=['foo'], branch='', pull='123:abc,333:ddd')))


    def test_depre_pull_repo_commit(self):
        """--repo=foo=master:aaa --pull=123:abc is not allowed"""
        with self.assertRaises(ValueError):
            bootstrap.parse_repos(FakeArgs(
                repo=['foo=master:aaa'], branch='', pull='123:abc'))
        with self.assertRaises(ValueError):
            bootstrap.parse_repos(FakeArgs(
                repo=['foo=master'], branch='', pull='123:abc'))

    def test_plain(self):
        """"--repo=foo equals foo=master."""
        self.assertEquals(
            {'foo': ('master', '')},
            bootstrap.parse_repos(FakeArgs(repo=['foo'], branch='', pull='')))

    def test_branch(self):
        """--repo=foo=branch."""
        self.assertEquals(
            {'foo': ('this', '')},
            bootstrap.parse_repos(
                FakeArgs(repo=['foo=this'], branch='', pull='')))

    def test_branch_commit(self):
        """--repo=foo=branch:commit works."""
        self.assertEquals(
            {'foo': ('this:abcd', '')},
            bootstrap.parse_repos(
                FakeArgs(repo=['foo=this:abcd'], branch='', pull='')))

    def test_parse_repos(self):
        """--repo=foo=111,222 works"""
        self.assertEquals(
            {'foo': ('', '111,222')},
            bootstrap.parse_repos(FakeArgs(
                repo=['foo=111,222'], branch='', pull='')))

    def test_pull_branch(self):
        """--repo=foo=master,111,222 works"""
        self.assertEquals(
            {'foo': ('', 'master,111,222')},
            bootstrap.parse_repos(
                FakeArgs(repo=['foo=master,111,222'], branch='', pull='')))

    def test_pull_release_branch(self):
        """--repo=foo=release-3.14,&a-fancy%_branch+:abcd,222 works"""
        self.assertEquals(
            {'foo': ('', 'release-3.14,&a-fancy%_branch+:abcd,222')},
            bootstrap.parse_repos(
                FakeArgs(repo=['foo=release-3.14,&a-fancy%_branch+:abcd,222'],
                         branch='', pull='')))

    def test_pull_branch_commit(self):
        """--repo=foo=master,111,222 works"""
        self.assertEquals(
            {'foo': ('', 'master:aaa,111:bbb,222:ccc')},
            bootstrap.parse_repos(FakeArgs(
                repo=['foo=master:aaa,111:bbb,222:ccc'], branch='', pull='')))

    def test_multi_repo(self):
        """--repo=foo=master,111,222 bar works"""
        self.assertEquals(
            {
                'foo': ('', 'master:aaa,111:bbb,222:ccc'),
                'bar': ('master', '')},
            bootstrap.parse_repos(
                FakeArgs(branch='', pull='', repo=[
                    'foo=master:aaa,111:bbb,222:ccc',
                    'bar',
                ])))


class GSUtilTest(unittest.TestCase):
    """Tests for GSUtil."""
    def test_upload_json(self):
        fake = FakeSubprocess()
        gsutil = bootstrap.GSUtil(fake)
        gsutil.upload_json('fake_path', {'wee': 'fun'})
        self.assertTrue(any(
            'application/json' in a for a in fake.calls[0][0]))
        self.assertIn('stdin', fake.calls[0][2])  # kwargs

    def test_upload_text_cached(self):
        fake = FakeSubprocess()
        gsutil = bootstrap.GSUtil(fake)
        gsutil.upload_text('fake_path', 'hello world', cached=True)
        self.assertFalse(any(
            'Cache-Control' in a and 'max-age' in a
            for a in fake.calls[0][0]))
        self.assertIn('stdin', fake.calls[0][2])  # kwargs

    def test_upload_text_default(self):
        fake = FakeSubprocess()
        gsutil = bootstrap.GSUtil(fake)
        gsutil.upload_text('fake_path', 'hello world')
        self.assertFalse(any(
            'Cache-Control' in a and 'max-age' in a
            for a in fake.calls[0][0]))
        self.assertIn('stdin', fake.calls[0][2])  # kwargs

    def test_upload_text_uncached(self):
        fake = FakeSubprocess()
        gsutil = bootstrap.GSUtil(fake)
        gsutil.upload_text('fake_path', 'hello world', cached=False)
        self.assertTrue(any(
            'Cache-Control' in a and 'max-age' in a
            for a in fake.calls[0][0]))
        self.assertIn('stdin', fake.calls[0][2])  # kwargs


class FakeGSUtil(object):
    generation = 123

    def __init__(self):
        self.cats = []
        self.jsons = []
        self.stats = []
        self.texts = []

    def cat(self, *a, **kw):
        self.cats.append((a, kw))
        return 'this is not a list'

    def stat(self, *a, **kw):
        self.stats.append((a, kw))
        return 'Generation: %s' % self.generation

    def upload_text(self, *args, **kwargs):
        self.texts.append((args, kwargs))

    def upload_json(self, *args, **kwargs):
        self.jsons.append((args, kwargs))

class GubernatorUriTest(unittest.TestCase):
    def create_path(self, uri):
        self.fake_path = FakePath()
        self.fake_path.build_log = uri
        return self.fake_path

    def test_non_gs(self):
        uri = 'hello/world'
        self.assertEquals('hello', bootstrap.gubernator_uri(self.create_path(uri)))

    def test_multiple_gs(self):
        uri = 'gs://hello/gs://there'
        self.assertEquals(
            bootstrap.GUBERNATOR + '/hello/gs:',
            bootstrap.gubernator_uri(self.create_path(uri)))

    def test_gs(self):
        uri = 'gs://blah/blah/blah.txt'
        self.assertEquals(
            bootstrap.GUBERNATOR + '/blah/blah',
            bootstrap.gubernator_uri(self.create_path(uri)))



class AppendResultTest(unittest.TestCase):
    """Tests for append_result()."""

    def test_new_job(self):
        """Stat fails when the job doesn't exist."""
        gsutil = FakeGSUtil()
        build = 123
        version = 'v.interesting'
        success = True
        def fake_stat(*_a, **_kw):
            raise subprocess.CalledProcessError(1, ['gsutil'], None)
        gsutil.stat = fake_stat
        bootstrap.append_result(gsutil, 'fake_path', build, version, success)
        cache = gsutil.jsons[0][0][1]
        self.assertEquals(1, len(cache))

    def test_collision_cat(self):
        """cat fails if the cache has been updated."""
        gsutil = FakeGSUtil()
        build = 42
        version = 'v1'
        success = False
        generations = ['555', '444']
        orig_stat = gsutil.stat
        def fake_stat(*a, **kw):
            gsutil.generation = generations.pop()
            return orig_stat(*a, **kw)
        def fake_cat(_, gen):
            if gen == '555':  # Which version is requested?
                return '[{"hello": 111}]'
            raise subprocess.CalledProcessError(1, ['gsutil'], None)
        with Stub(bootstrap, 'random_sleep', Pass):
            with Stub(gsutil, 'stat', fake_stat):
                with Stub(gsutil, 'cat', fake_cat):
                    bootstrap.append_result(
                        gsutil, 'fake_path', build, version, success)
        self.assertIn('generation', gsutil.jsons[-1][1], gsutil.jsons)
        self.assertEquals('555', gsutil.jsons[-1][1]['generation'], gsutil.jsons)

    def test_collision_upload(self):
        """Test when upload_json tries to update an old version."""
        gsutil = FakeGSUtil()
        build = 42
        version = 'v1'
        success = False
        generations = [555, 444]
        orig = gsutil.upload_json
        def fake_upload(path, cache, generation):
            if generation == '555':
                return orig(path, cache, generation=generation)
            raise subprocess.CalledProcessError(128, ['gsutil'], None)
        orig_stat = gsutil.stat
        def fake_stat(*a, **kw):
            gsutil.generation = generations.pop()
            return orig_stat(*a, **kw)
        def fake_cat(*_a, **_kw):
            return '[{"hello": 111}]'
        gsutil.stat = fake_stat
        gsutil.upload_json = fake_upload
        gsutil.cat = fake_cat
        with Stub(bootstrap, 'random_sleep', Pass):
            bootstrap.append_result(
                gsutil, 'fake_path', build, version, success)
        self.assertIn('generation', gsutil.jsons[-1][1], gsutil.jsons)
        self.assertEquals('555', gsutil.jsons[-1][1]['generation'], gsutil.jsons)

    def test_handle_junk(self):
        gsutil = FakeGSUtil()
        gsutil.cat = lambda *a, **kw: '!@!$!@$@!$'
        build = 123
        version = 'v.interesting'
        success = True
        bootstrap.append_result(gsutil, 'fake_path', build, version, success)
        cache = gsutil.jsons[0][0][1]
        self.assertEquals(1, len(cache))
        self.assertIn(build, cache[0].values())
        self.assertIn(version, cache[0].values())

    def test_passed_is_bool(self):
        build = 123
        version = 'v.interesting'
        def try_run(success):
            gsutil = FakeGSUtil()
            bootstrap.append_result(gsutil, 'fake_path', build, version, success)
            cache = gsutil.jsons[0][0][1]
            self.assertTrue(isinstance(cache[0]['passed'], bool))

        try_run(1)
        try_run(0)
        try_run(None)
        try_run('')
        try_run('hello')
        try_run('true')

    def test_truncate(self):
        old = json.dumps({n: True for n in range(100000)})
        gsutil = FakeGSUtil()
        build = 123
        version = 'v.interesting'
        success = True
        bootstrap.append_result(gsutil, 'fake_path', build, version, success)
        cache = gsutil.jsons[0][0][1]
        self.assertLess(len(cache), len(old))


class FinishTest(unittest.TestCase):
    """Tests for finish()."""
    def setUp(self):
        self.stubs = [
            Stub(bootstrap.GSUtil, 'upload_artifacts', Pass),
            Stub(bootstrap, 'append_result', Pass),
            Stub(os.path, 'isfile', Pass),
            Stub(os.path, 'isdir', Pass),
        ]

    def tearDown(self):
        for stub in self.stubs:
            with stub:
                pass

    def test_no_version(self):
        gsutil = FakeGSUtil()
        paths = FakePath()
        success = True
        artifacts = 'not-a-dir'
        no_version = ''
        version = 'should not have found it'
        repos = repo({REPO: ('master', '')})
        with Stub(bootstrap, 'metadata', lambda *a: {'random-meta': version}):
            bootstrap.finish(gsutil, paths, success, artifacts,
                             BUILD, no_version, repos, FakeCall())
        bootstrap.finish(gsutil, paths, success, artifacts, BUILD, no_version, repos, FakeCall())
        calls = gsutil.jsons[-1]
        # json data is second positional argument
        self.assertNotIn('job-version', calls[0][1])
        self.assertNotIn('version', calls[0][1])
        self.assertTrue(calls[0][1].get('metadata'))

    def test_metadata_version(self):
        """Test that we will extract version info from metadata."""
        self.check_metadata_version('job-version')
        self.check_metadata_version('version')

    def check_metadata_version(self, key):
        gsutil = FakeGSUtil()
        paths = FakePath()
        success = True
        artifacts = 'not-a-dir'
        no_version = ''
        version = 'found it'
        with Stub(bootstrap, 'metadata', lambda *a: {key: version}):
            bootstrap.finish(gsutil, paths, success, artifacts, BUILD, no_version, REPO, FakeCall())
        calls = gsutil.jsons[-1]
        # Meta is second positional argument
        self.assertEquals(version, calls[0][1].get('job-version'))
        self.assertEquals(version, calls[0][1].get('version'))

    def test_ignore_err_up_artifacts(self):
        paths = FakePath()
        gsutil = FakeGSUtil()
        local_artifacts = None
        build = 123
        version = 'v1.terrible'
        success = True
        calls = []
        with Stub(os.path, 'isdir', lambda _: True):
            with Stub(os, 'walk', lambda d: [(True, True, True)]):
                def fake_upload(*a, **kw):
                    calls.append((a, kw))
                    raise subprocess.CalledProcessError(1, ['fakecmd'], None)
                gsutil.upload_artifacts = fake_upload
                repos = repo({REPO: ('master', '')})
                bootstrap.finish(
                    gsutil, paths, success, local_artifacts,
                    build, version, repos, FakeCall())
                self.assertTrue(calls)


    def test_ignore_error_uploadtext(self):
        paths = FakePath()
        gsutil = FakeGSUtil()
        local_artifacts = None
        build = 123
        version = 'v1.terrible'
        success = True
        calls = []
        with Stub(os.path, 'isdir', lambda _: True):
            with Stub(os, 'walk', lambda d: [(True, True, True)]):
                def fake_upload(*a, **kw):
                    calls.append((a, kw))
                    raise subprocess.CalledProcessError(1, ['fakecmd'], None)
                gsutil.upload_artifacts = Pass
                gsutil.upload_text = fake_upload
                repos = repo({REPO: ('master', '')})
                bootstrap.finish(
                    gsutil, paths, success, local_artifacts,
                    build, version, repos, FakeCall())
                self.assertTrue(calls)
                self.assertGreater(calls, 1)

    def test_skip_upload_artifacts(self):
        """Do not upload artifacts dir if it doesn't exist."""
        paths = FakePath()
        gsutil = FakeGSUtil()
        local_artifacts = None
        build = 123
        version = 'v1.terrible'
        success = True
        calls = []
        with Stub(os.path, 'isdir', lambda _: False):
            with Stub(bootstrap.GSUtil, 'upload_artifacts', Bomb):
                repos = repo({REPO: ('master', '')})
                bootstrap.finish(
                    gsutil, paths, success, local_artifacts,
                    build, version, repos, FakeCall())
                self.assertFalse(calls)


class MetadataTest(unittest.TestCase):

    def test_always_set_metadata(self):
        repos = repo({REPO: ('master', '')})
        meta = bootstrap.metadata(repos, 'missing-artifacts-dir', FakeCall())
        self.assertIn('repo', meta)
        self.assertEquals(REPO, meta['repo'])

    def test_multi_repo(self):
        repos = repo({REPO: ('foo', ''), 'other-repo': ('', '123,456')})
        meta = bootstrap.metadata(repos, 'missing-artifacts-dir', FakeCall())
        self.assertIn('repo', meta)
        self.assertEquals(REPO, meta['repo'])
        self.assertIn(REPO, meta.get('repos'))
        self.assertEquals('foo', meta['repos'][REPO])
        self.assertIn('other-repo', meta.get('repos'))
        self.assertEquals('123,456', meta['repos']['other-repo'])


SECONDS = 10


def fake_environment(set_home=True, set_node=True, set_job=True, **kwargs):
    if set_home:
        kwargs.setdefault(bootstrap.HOME_ENV, '/fake/home-dir')
    if set_node:
        kwargs.setdefault(bootstrap.NODE_ENV, 'fake-node')
    if set_job:
        kwargs.setdefault(bootstrap.JOB_ENV, JOB)
    return kwargs


class BuildNameTest(unittest.TestCase):
    """Tests for build_name()."""

    def test_auto(self):
        """Automatically select a build if not done by user."""
        with Stub(os, 'environ', fake_environment()) as fake:
            bootstrap.build_name(SECONDS)
            self.assertTrue(fake[bootstrap.BUILD_ENV])

    def test_manual(self):
        """Respect user-selected build."""
        with Stub(os, 'environ', fake_environment()) as fake:
            truth = 'erick is awesome'
            fake[bootstrap.BUILD_ENV] = truth
            self.assertEquals(truth, fake[bootstrap.BUILD_ENV])

    def test_unique(self):
        """New build every minute."""
        with Stub(os, 'environ', fake_environment()) as fake:
            bootstrap.build_name(SECONDS)
            first = fake[bootstrap.BUILD_ENV]
            del fake[bootstrap.BUILD_ENV]
            bootstrap.build_name(SECONDS + 60)
            self.assertNotEqual(first, fake[bootstrap.BUILD_ENV])


class SetupCredentialsTest(unittest.TestCase):
    """Tests for setup_credentials()."""

    def setUp(self):
        keys = {
            bootstrap.GCE_KEY_ENV: 'fake-key',
            bootstrap.SERVICE_ACCOUNT_ENV: 'fake-service-account.json',
        }
        self.env = fake_environment(**keys)

    def test_norobot_noupload_noenv(self):
        """Can avoid setting up credentials."""
        del self.env[bootstrap.SERVICE_ACCOUNT_ENV]
        with Stub(os, 'environ', self.env):
            bootstrap.setup_credentials(Bomb, None, None)

    def test_upload_no_robot_raises(self):
        del self.env[bootstrap.SERVICE_ACCOUNT_ENV]
        with Stub(os, 'environ', self.env):
            with self.assertRaises(ValueError):
                bootstrap.setup_credentials(Pass, None, 'gs://fake')


    def test_application_credentials(self):
        """Raise if GOOGLE_APPLICATION_CREDENTIALS does not exist."""
        del self.env[bootstrap.SERVICE_ACCOUNT_ENV]
        with Stub(os, 'environ', self.env) as fake:
            gac = 'FAKE_CREDS.json'
            fake['HOME'] = 'kansas'
            with Stub(os.path, 'isfile', lambda p: p != gac):
                with self.assertRaises(IOError):
                    bootstrap.setup_credentials(Pass, gac, UPLOAD)

            with Stub(os.path, 'isfile', Truth):
                call = lambda *a, **kw: 'robot'
                bootstrap.setup_credentials(call, gac, UPLOAD)
            # setup_creds should set SERVICE_ACCOUNT_ENV
            self.assertEquals(gac, fake.get(bootstrap.SERVICE_ACCOUNT_ENV))
            # now that SERVICE_ACCOUNT_ENV is set, it should try to activate
            # this
            with Stub(os.path, 'isfile', lambda p: p != gac):
                with self.assertRaises(IOError):
                    bootstrap.setup_credentials(Pass, None, UPLOAD)


class SetupMagicEnvironmentTest(unittest.TestCase):
    def test_workspace(self):
        """WORKSPACE exists, equals HOME and is set to cwd."""
        env = fake_environment()
        cwd = '/fake/random-location'
        with Stub(os, 'environ', env):
            with Stub(os, 'getcwd', lambda: cwd):
                bootstrap.setup_magic_environment(JOB)

        self.assertIn(bootstrap.WORKSPACE_ENV, env)
        self.assertEquals(env[bootstrap.HOME_ENV], env[bootstrap.WORKSPACE_ENV])
        self.assertEquals(cwd, env[bootstrap.WORKSPACE_ENV])

    def test_job_env_mismatch(self):
        env = fake_environment()
        with Stub(os, 'environ', env):
            self.assertNotEquals('this-is-a-job', env[bootstrap.JOB_ENV])
            bootstrap.setup_magic_environment('this-is-a-job')
            self.assertEquals('this-is-a-job', env[bootstrap.JOB_ENV])

    def test_expected(self):
        env = fake_environment()
        del env[bootstrap.JOB_ENV]
        del env[bootstrap.NODE_ENV]
        with Stub(os, 'environ', env):
            bootstrap.setup_magic_environment(JOB)

        def check(name):
            self.assertIn(name, env)

        # Some of these are probably silly to check...
        # TODO(fejta): remove as many of these from our infra as possible.
        check(bootstrap.JOB_ENV)
        check(bootstrap.CLOUDSDK_ENV)
        check(bootstrap.BOOTSTRAP_ENV)
        check(bootstrap.WORKSPACE_ENV)
        self.assertNotIn(bootstrap.SERVICE_ACCOUNT_ENV, env)

    def test_node_present(self):
        expected = 'whatever'
        env = {bootstrap.NODE_ENV: expected}
        with Stub(os, 'environ', env):
            self.assertEquals(expected, bootstrap.node())
        self.assertEquals(expected, env[bootstrap.NODE_ENV])

    def test_node_missing(self):
        env = {}
        with Stub(os, 'environ', env):
            expected = bootstrap.node()
            self.assertTrue(expected)
        self.assertEquals(expected, env[bootstrap.NODE_ENV])



    def test_cloud_sdk_config(self):
        cwd = 'now-here'
        env = fake_environment()
        with Stub(os, 'environ', env):
            with Stub(os, 'getcwd', lambda: cwd):
                bootstrap.setup_magic_environment(JOB)


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
    def __call__(self, *arg, **kw):
        self.arg = arg
        self.kw = kw
        return self


class FakeLogging(object):
    close = Pass
    def __call__(self, *_a, **_kw):
        return self


class FakeFinish(object):
    called = False
    result = None
    def __call__(self, unused_a, unused_b, success, *a, **kw):
        self.called = True
        self.result = success

def repo(config):
    repos = bootstrap.Repos()
    for cur_repo, tup in config.items():
        repos[cur_repo] = tup
    return repos

class PRPathsTest(unittest.TestCase):
    def test_kubernetes_kubernetes(self):
        """Test the kubernetes/kubernetes prefix."""
        path = bootstrap.pr_paths(UPLOAD, repo({'kubernetes/kubernetes': ('', PULL)}), JOB, BUILD)
        self.assertTrue(any(
            str(PULL) == p for p in path.build_log.split('/')))

    def test_kubernetes(self):
        """Test the kubernetes/something prefix."""
        path = bootstrap.pr_paths(UPLOAD, repo({'kubernetes/prefix': ('', PULL)}), JOB, BUILD)
        self.assertTrue(any(
            'prefix' in p for p in path.build_log.split('/')), path.build_log)
        self.assertTrue(any(
            str(PULL) in p for p in path.build_log.split('/')), path.build_log)

    def test_other(self):
        """Test the none kubernetes prefixes."""
        path = bootstrap.pr_paths(UPLOAD, repo({'github.com/random/repo': ('', PULL)}), JOB, BUILD)
        self.assertTrue(any(
            'random_repo' in p for p in path.build_log.split('/')), path.build_log)
        self.assertTrue(any(
            str(PULL) in p for p in path.build_log.split('/')), path.build_log)


class FakeArgs(object):
    bare = False
    clean = False
    git_cache = ''
    job = JOB
    root = ROOT
    service_account = ROBOT
    ssh = False
    timeout = 0
    upload = UPLOAD
    json = False

    def __init__(self, **kw):
        self.branch = BRANCH
        self.pull = PULL
        self.repo = [REPO]
        for key, val in kw.items():
            if not hasattr(self, key):
                raise AttributeError(self, key)
            setattr(self, key, val)


def test_bootstrap(**kw):
    if isinstance(kw.get('repo'), basestring):
        kw['repo'] = [kw['repo']]
    return bootstrap.bootstrap(FakeArgs(**kw))

class BootstrapTest(unittest.TestCase):

    def setUp(self):
        self.boiler = [
            Stub(bootstrap, 'checkout', Pass),
            Stub(bootstrap, 'finish', Pass),
            Stub(bootstrap.GSUtil, 'copy_file', Pass),
            Stub(bootstrap, 'node', lambda: 'fake-node'),
            Stub(bootstrap, 'setup_credentials', Pass),
            Stub(bootstrap, 'setup_logging', FakeLogging()),
            Stub(bootstrap, 'start', Pass),
            Stub(bootstrap, '_call', Pass),
            Stub(os, 'environ', fake_environment()),
            Stub(os, 'chdir', Pass),
            Stub(os, 'makedirs', Pass),
        ]

    def tearDown(self):
        for stub in self.boiler:
            with stub:  # Leaving with restores things
                pass

    def test_setcreds_setroot_fails(self):
        """We should still call setup_credentials even if setup_root blows up."""
        called = set()
        with Stub(bootstrap, 'setup_root', Bomb):
            with Stub(bootstrap, 'setup_credentials',
                      lambda *a, **kw: called.add('setup_credentials')):
                with Stub(bootstrap, 'finish', lambda *a, **kw: called.add('finish')):
                    with self.assertRaises(AssertionError):
                        test_bootstrap()

        for needed in ['setup_credentials', 'finish']:
            self.assertIn(needed, called)

    def test_empty_repo(self):
        repo_name = None
        with Stub(bootstrap, 'checkout', Bomb):
            test_bootstrap(repo=repo_name, branch=None, pull=None, bare=True)
        with self.assertRaises(ValueError):
            test_bootstrap(repo=repo_name, branch=None, pull=PULL)
        with self.assertRaises(ValueError):
            test_bootstrap(repo=repo_name, branch=BRANCH, pull=None)

    def test_root_not_exists(self):
        with Stub(os, 'chdir', FakeCall()) as fake_chdir:
            with Stub(os.path, 'exists', lambda p: False):
                with Stub(os, 'makedirs', FakeCall()) as fake_makedirs:
                    test_bootstrap(branch=None)
        self.assertTrue(any(ROOT in c[0] for c in fake_chdir.calls), fake_chdir.calls)
        self.assertTrue(any(ROOT in c[0] for c in fake_makedirs.calls), fake_makedirs.calls)

    def test_root_exists(self):
        with Stub(os, 'chdir', FakeCall()) as fake_chdir:
            test_bootstrap(branch=None)
        self.assertTrue(any(ROOT in c[0] for c in fake_chdir.calls))

    def test_pr_paths(self):
        """Use a pr_paths when pull is set."""

        with Stub(bootstrap, 'ci_paths', Bomb):
            with Stub(bootstrap, 'pr_paths', FakePath()) as path:
                test_bootstrap(branch=None, pull=PULL)
            self.assertEquals(PULL, path.arg[1][REPO][1], (PULL, path.arg))

    def test_ci_paths(self):
        """Use a ci_paths when branch is set."""

        with Stub(bootstrap, 'pr_paths', Bomb):
            with Stub(bootstrap, 'ci_paths', FakePath()) as path:
                test_bootstrap(pull=None, branch=BRANCH)
            self.assertFalse(any(
                PULL in o for o in (path.arg, path.kw)))

    def test_finish_when_start_fails(self):
        """Finish is called even if start fails."""
        with Stub(bootstrap, 'finish', FakeFinish()) as fake:
            with Stub(bootstrap, 'start', Bomb):
                with self.assertRaises(AssertionError):
                    test_bootstrap(pull=None)
        self.assertTrue(fake.called)
        self.assertTrue(fake.result is False)  # Distinguish from None

    def test_finish_when_build_fails(self):
        """Finish is called even if the build fails."""
        def call_error(*_a, **_kw):
            raise subprocess.CalledProcessError(1, [], '')
        with Stub(bootstrap, 'finish', FakeFinish()) as fake:
            with Stub(bootstrap, '_call', call_error):
                with self.assertRaises(SystemExit):
                    test_bootstrap(pull=None)
        self.assertTrue(fake.called)
        self.assertTrue(fake.result is False)  # Distinguish from None

    def test_happy(self):
        with Stub(bootstrap, 'finish', FakeFinish()) as fake:
            test_bootstrap(pull=None)
        self.assertTrue(fake.called)
        self.assertTrue(fake.result)  # Distinguish from None

    def test_job_env(self):
        """bootstrap sets JOB_NAME."""
        with Stub(os, 'environ', fake_environment()) as env:
            test_bootstrap(pull=None)
        self.assertIn(bootstrap.JOB_ENV, env)


class RepositoryTest(unittest.TestCase):
    def test_kubernetes_kubernetes(self):
        expected = 'https://github.com/kubernetes/kubernetes'
        actual = bootstrap.repository('k8s.io/kubernetes', '')
        self.assertEquals(expected, actual)

    def test_kubernetes_testinfra(self):
        expected = 'https://github.com/kubernetes/test-infra'
        actual = bootstrap.repository('k8s.io/test-infra', '')
        self.assertEquals(expected, actual)

    def test_whatever(self):
        expected = 'https://foo.com/bar'
        actual = bootstrap.repository('foo.com/bar', '')
        self.assertEquals(expected, actual)

    def test_k8s_k8s_ssh(self):
        expected = 'git@github.com:kubernetes/kubernetes'
        actual = bootstrap.repository('k8s.io/kubernetes', 'path')
        self.assertEquals(expected, actual)

    def test_k8s_k8s_ssh_with_colon(self):
        expected = 'git@github.com:kubernetes/kubernetes'
        actual = bootstrap.repository('github.com:kubernetes/kubernetes', 'path')
        self.assertEquals(expected, actual)

    def test_whatever_ssh(self):
        expected = 'git@foo.com:bar'
        actual = bootstrap.repository('foo.com/bar', 'path')
        self.assertEquals(expected, actual)



class IntegrationTest(unittest.TestCase):
    REPO = 'hello/world'
    MASTER = 'fake-master-file'
    BRANCH_FILE = 'fake-branch-file'
    PR_FILE = 'fake-pr-file'
    BRANCH = 'another-branch'
    PR_NUM = 42
    PR_TAG = bootstrap.pull_ref(PR_NUM)[0][0].strip('+')

    def fake_repo(self, fake, _ssh=False):
        return os.path.join(self.root_github, fake)

    def setUp(self):
        self.boiler = [
            Stub(bootstrap, 'finish', Pass),
            Stub(bootstrap.GSUtil, 'copy_file', Pass),
            Stub(bootstrap, 'repository', self.fake_repo),
            Stub(bootstrap, 'setup_credentials', Pass),
            Stub(bootstrap, 'setup_logging', FakeLogging()),
            Stub(bootstrap, 'start', Pass),
            Stub(os, 'environ', fake_environment(set_job=False)),
        ]
        self.root_github = tempfile.mkdtemp()
        self.root_workspace = tempfile.mkdtemp()
        self.root_git_cache = tempfile.mkdtemp()
        self.ocwd = os.getcwd()
        fakerepo = self.fake_repo(self.REPO)
        subprocess.check_call(['git', 'init', fakerepo])
        os.chdir(fakerepo)
        subprocess.check_call(['git', 'config', 'user.name', 'foo'])
        subprocess.check_call(['git', 'config', 'user.email', 'foo@bar.baz'])
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
        subprocess.check_call(['rm', '-rf', self.root_git_cache])

    def test_git_cache(self):
        subprocess.check_call(['git', 'checkout', '-b', self.BRANCH])
        subprocess.check_call(['git', 'rm', self.MASTER])
        subprocess.check_call(['touch', self.BRANCH_FILE])
        subprocess.check_call(['git', 'add', self.BRANCH_FILE])
        subprocess.check_call(['git', 'commit', '-m', 'Create %s' % self.BRANCH])
        test_bootstrap(
            job='fake-branch',
            repo=self.REPO,
            branch=self.BRANCH,
            pull=None,
            root=self.root_workspace,
            git_cache=self.root_git_cache)
        # Verify that the cache was populated by running a simple git command
        # in the git cache directory.
        subprocess.check_call(
            ['git', '--git-dir=%s/%s' % (self.root_git_cache, self.REPO), 'log'])

    def test_pr(self):
        subprocess.check_call(['git', 'checkout', 'master'])
        subprocess.check_call(['git', 'checkout', '-b', 'unknown-pr-branch'])
        subprocess.check_call(['git', 'rm', self.MASTER])
        subprocess.check_call(['touch', self.PR_FILE])
        subprocess.check_call(['git', 'add', self.PR_FILE])
        subprocess.check_call(['git', 'commit', '-m', 'Create branch for PR %d' % self.PR_NUM])
        subprocess.check_call(['git', 'tag', self.PR_TAG])
        os.chdir('/tmp')
        test_bootstrap(
            job='fake-pr',
            repo=self.REPO,
            branch=None,
            pull=self.PR_NUM,
            root=self.root_workspace)

    def test_branch(self):
        subprocess.check_call(['git', 'checkout', '-b', self.BRANCH])
        subprocess.check_call(['git', 'rm', self.MASTER])
        subprocess.check_call(['touch', self.BRANCH_FILE])
        subprocess.check_call(['git', 'add', self.BRANCH_FILE])
        subprocess.check_call(['git', 'commit', '-m', 'Create %s' % self.BRANCH])

        os.chdir('/tmp')
        test_bootstrap(
            job='fake-branch',
            repo=self.REPO,
            branch=self.BRANCH,
            pull=None,
            root=self.root_workspace)

    def test_branch_ref(self):
        """Make sure we check out a specific commit."""
        subprocess.check_call(['git', 'checkout', '-b', self.BRANCH])
        subprocess.check_call(['git', 'rm', self.MASTER])
        subprocess.check_call(['touch', self.BRANCH_FILE])
        subprocess.check_call(['git', 'add', self.BRANCH_FILE])
        subprocess.check_call(['git', 'commit', '-m', 'Create %s' % self.BRANCH])
        sha = subprocess.check_output(['git', 'rev-parse', 'HEAD']).strip()
        subprocess.check_call(['rm', self.BRANCH_FILE])
        subprocess.check_call(['git', 'add', self.BRANCH_FILE])
        subprocess.check_call(['git', 'commit', '-m', 'Delete %s' % self.BRANCH])

        os.chdir('/tmp')
        # Supplying the commit exactly works.
        test_bootstrap(
            job='fake-branch',
            repo=self.REPO,
            branch='%s:%s' % (self.BRANCH, sha),
            pull=None,
            root=self.root_workspace)
        # Supplying the commit through repo works.
        test_bootstrap(
            job='fake-branch',
            repo="%s=%s:%s" % (self.REPO, self.BRANCH, sha),
            branch=None,
            pull=None,
            root=self.root_workspace)
        # Using branch head fails.
        with self.assertRaises(SystemExit):
            test_bootstrap(
                job='fake-branch',
                repo=self.REPO,
                branch=self.BRANCH,
                pull=None,
                root=self.root_workspace)
        with self.assertRaises(SystemExit):
            test_bootstrap(
                job='fake-branch',
                repo="%s=%s" % (self.REPO, self.BRANCH),
                branch=None,
                pull=None,
                root=self.root_workspace)

    def test_batch(self):
        def head_sha():
            # We can't hardcode the SHAs for the test, so we need to determine
            # them after each commit.
            return subprocess.check_output(['git', 'rev-parse', 'HEAD']).strip()
        refs = ['master:%s' % head_sha()]
        for pr in (123, 456):
            subprocess.check_call(['git', 'checkout', '-b', 'refs/pull/%d/head' % pr, 'master'])
            subprocess.check_call(['git', 'rm', self.MASTER])
            subprocess.check_call(['touch', self.PR_FILE])
            subprocess.check_call(['git', 'add', self.PR_FILE])
            open('pr_%d.txt' % pr, 'w').write('some text')
            subprocess.check_call(['git', 'add', 'pr_%d.txt' % pr])
            subprocess.check_call(['git', 'commit', '-m', 'add some stuff (#%d)' % pr])
            refs.append('%d:%s' % (pr, head_sha()))
        os.chdir('/tmp')
        pull = ','.join(refs)
        print '--pull', pull
        test_bootstrap(
            job='fake-pr',
            repo=self.REPO,
            branch=None,
            pull=pull,
            root=self.root_workspace)

    def test_pr_bad(self):
        random_pr = 111
        with Stub(bootstrap, 'start', Bomb):
            with Stub(time, 'sleep', Pass):
                with self.assertRaises(subprocess.CalledProcessError):
                    test_bootstrap(
                        job='fake-pr',
                        repo=self.REPO,
                        branch=None,
                        pull=random_pr,
                        root=self.root_workspace)

    def test_branch_bad(self):
        random_branch = 'something'
        with Stub(bootstrap, 'start', Bomb):
            with Stub(time, 'sleep', Pass):
                with self.assertRaises(subprocess.CalledProcessError):
                    test_bootstrap(
                        job='fake-branch',
                        repo=self.REPO,
                        branch=random_branch,
                        pull=None,
                        root=self.root_workspace)

    def test_job_missing(self):
        with self.assertRaises(OSError):
            test_bootstrap(
                job='this-job-no-exists',
                repo=self.REPO,
                branch='master',
                pull=None,
                root=self.root_workspace)

    def test_job_fails(self):
        with self.assertRaises(SystemExit):
            test_bootstrap(
                job='fake-failure',
                repo=self.REPO,
                branch='master',
                pull=None,
                root=self.root_workspace)

    def test_commit_in_meta(self):
        sha = subprocess.check_output(['git', 'rev-parse', 'HEAD']).strip()
        cwd = os.getcwd()
        os.chdir(bootstrap.test_infra('.'))
        infra_sha = subprocess.check_output(['git', 'rev-parse', 'HEAD']).strip()[:9]
        os.chdir(cwd)

        # Commit SHA should in meta
        call = lambda *a, **kw: bootstrap._call(5, *a, **kw)
        repos = repo({REPO: ('master', ''), 'other-repo': ('other-branch', '')})
        meta = bootstrap.metadata(repos, 'missing-artifacts-dir', call)
        self.assertIn('repo-commit', meta)
        self.assertEquals(sha, meta['repo-commit'])
        self.assertEquals(40, len(meta['repo-commit']))
        self.assertIn('infra-commit', meta)
        self.assertEquals(infra_sha, meta['infra-commit'])
        self.assertEquals(9, len(meta['infra-commit']))
        self.assertIn(REPO, meta.get('repos'))
        self.assertIn('other-repo', meta.get('repos'))
        self.assertEquals(REPO, meta.get('repo'))


class ParseArgsTest(unittest.TestCase):
    def test_json_missing(self):
        args = bootstrap.parse_args(['--bare', '--job=j'])
        self.assertFalse(args.json, args)

    def test_json_onlyflag(self):
        args = bootstrap.parse_args(['--json', '--bare', '--job=j'])
        self.assertTrue(args.json, args)

    def test_json_nonzero(self):
        args = bootstrap.parse_args(['--json=1', '--bare', '--job=j'])
        self.assertTrue(args.json, args)

    def test_json_zero(self):
        args = bootstrap.parse_args(['--json=0', '--bare', '--job=j'])
        self.assertFalse(args.json, args)

    def test_barerepo_both(self):
        with self.assertRaises(argparse.ArgumentTypeError):
            bootstrap.parse_args(['--bare', '--repo=hello', '--job=j'])

    def test_barerepo_neither(self):
        with self.assertRaises(argparse.ArgumentTypeError):
            bootstrap.parse_args(['--job=j'])

    def test_barerepo_bareonly(self):
        args = bootstrap.parse_args(['--bare', '--job=j'])
        self.assertFalse(args.repo, args)
        self.assertTrue(args.bare, args)

    def test_barerepo_repoonly(self):
        args = bootstrap.parse_args(['--repo=R', '--job=j'])
        self.assertFalse(args.bare, args)
        self.assertTrue(args.repo, args)


class JobTest(unittest.TestCase):

    excludes = [
        'BUILD',  # For bazel
        'config.json',  # For --json mode
        'validOwners.json', # Contains a list of current sigs; sigs are allowed to own jobs
        'config_sort.py', # Tool script to sort config.json
        'config_test.py', # Script for testing config.json and Prow config.
        'env_gc.py', # Tool script to garbage collect unused .env files.
        'move_extract.py',
    ]

    yaml_suffix = {
        'job-configs/bootstrap-maintenance.yaml' : 'suffix',
        'job-configs/kubernetes-jenkins/bootstrap-maintenance-ci.yaml' : 'suffix',
        'job-configs/kubernetes-jenkins-pull/bootstrap-maintenance-pull.yaml' : 'suffix',
        'job-configs/kubernetes-jenkins-pull/bootstrap-pull-json.yaml' : 'jsonsuffix',
        'job-configs/kubernetes-jenkins-pull/bootstrap-security-pull.yaml' : 'suffix',
        'job-configs/kubernetes-jenkins/bootstrap-ci.yaml' : 'suffix',
        'job-configs/kubernetes-jenkins/bootstrap-ci-commit.yaml' : 'commit-suffix',
        'job-configs/kubernetes-jenkins/bootstrap-ci-repo.yaml' : 'repo-suffix',
        'job-configs/kubernetes-jenkins/bootstrap-ci-soak.yaml' : 'soak-suffix',
        'job-configs/kubernetes-jenkins/bootstrap-ci-dockerpush.yaml' : 'dockerpush-suffix'
    }

    prow_config = '../prow/config.yaml'

    realjobs = {}
    prowjobs = []

    def test_job_script_expands_vars(self):
        fake = {
            'HELLO': 'awesome',
            'WORLD': 'sauce',
        }
        with Stub(os, 'environ', fake):
            actual = bootstrap.job_args(
                ['$HELLO ${WORLD}', 'happy', '${MISSING}'])
        self.assertEquals(['awesome sauce', 'happy', '${MISSING}'], actual)


    @property
    def jobs(self):
        """[(job, job_path)] sequence"""
        for path, _, filenames in os.walk(
                os.path.dirname(bootstrap.job_script(JOB, False)[0])):
            for job in [f for f in filenames if f not in self.excludes]:
                job_path = os.path.join(path, job)
                yield job, job_path

    def test_bootstrap_maintenance_yaml(self):
        def check(job, name):
            job_name = 'maintenance-%s' % name
            self.assertIn('frequency', job)
            self.assertIn('repo-name', job)
            self.assertIn('.', job['repo-name'])  # Has domain
            self.assertGreater(job['timeout'], 0)
            return job_name

        self.check_bootstrap_yaml('job-configs/bootstrap-maintenance.yaml', check, use_json=True)

    def test_bootstrap_maintenance_ci(self):
        def check(job, name):
            job_name = 'maintenance-ci-%s' % name
            self.assertIn('frequency', job)
            self.assertIn('repo-name', job)
            self.assertIn('.', job['repo-name'])  # Has domain
            self.assertIn('timeout', job)
            self.assertNotIn('json', job)
            self.assertGreater(job['timeout'], 0)
            return job_name

        self.check_bootstrap_yaml('job-configs/kubernetes-jenkins/bootstrap-maintenance-ci.yaml',
                                  check, use_json=True)

    def test_bootstrap_maintenance_pull(self):
        def check(job, name):
            job_name = 'maintenance-pull-%s' % name
            self.assertIn('frequency', job)
            self.assertIn('repo-name', job)
            self.assertIn('.', job['repo-name'])  # Has domain
            self.assertIn('timeout', job)
            self.assertNotIn('json', job)
            self.assertGreater(job['timeout'], 0)
            return job_name

        self.check_bootstrap_yaml(
            'job-configs/kubernetes-jenkins-pull/bootstrap-maintenance-pull.yaml',
            check, use_json=True)

    def test_bootstrap_pull_json_yaml(self):
        def check(job, name):
            job_name = 'pull-%s' % name
            self.assertIn('max-total', job)
            self.assertIn('repo-name', job)
            self.assertIn('.', job['repo-name'])  # Has domain
            self.assertIn('timeout', job)
            self.assertNotIn('json', job)
            self.assertGreater(job['timeout'], 0)
            return job_name

        self.check_bootstrap_yaml(
            'job-configs/kubernetes-jenkins-pull/bootstrap-pull-json.yaml',
            check, use_json=True)

    def test_bootstrap_security_pull(self):
        def check(job, name):
            job_name = 'pull-%s' % name
            self.assertIn('max-total', job)
            self.assertIn('repo-name', job)
            self.assertIn('.', job['repo-name'])  # Has domain
            self.assertIn('timeout', job)
            self.assertNotIn('json', job)
            self.assertGreater(job['timeout'], 0)
            return job_name

        self.check_bootstrap_yaml(
            'job-configs/kubernetes-jenkins-pull/bootstrap-security-pull.yaml',
            check, use_json=True)

    def test_bootstrap_security_match(self):
        json_jobs = self.load_bootstrap_yaml(
            'job-configs/kubernetes-jenkins-pull/bootstrap-pull-json.yaml')

        sec_jobs = self.load_bootstrap_yaml(
            'job-configs/kubernetes-jenkins-pull/bootstrap-security-pull.yaml')
        for name, job in sec_jobs.iteritems():
            self.assertIn(name, json_jobs)
            job2 = json_jobs[name]
            for attr in job:
                if attr == 'repo-name':
                    continue
                self.assertEquals(job[attr], job2[attr])


    def test_bootstrap_ci_yaml(self):
        def check(job, name):
            job_name = 'ci-%s' % name
            self.assertIn('frequency', job)
            self.assertIn('trigger-job', job)
            self.assertNotIn('branch', job)
            self.assertNotIn('json', job)
            self.assertGreater(job['timeout'], 0, job_name)
            self.assertGreaterEqual(job['jenkins-timeout'], job['timeout']+100, job_name)
            return job_name

        self.check_bootstrap_yaml(
            'job-configs/kubernetes-jenkins/bootstrap-ci.yaml',
            check, use_json=True)

    def test_bootstrap_ci_commit_yaml(self):
        def check(job, name):
            job_name = 'ci-%s' % name
            self.assertIn('branch', job)
            self.assertIn('commit-frequency', job)
            self.assertIn('giturl', job)
            self.assertIn('repo-name', job)
            self.assertIn('timeout', job)
            self.assertNotIn('use-logexporter', job)
            self.assertGreater(job['timeout'], 0, job)

            return job_name

        self.check_bootstrap_yaml(
            'job-configs/kubernetes-jenkins/bootstrap-ci-commit.yaml',
            check, use_json=True)

    def test_bootstrap_ci_repo_yaml(self):
        def check(job, name):
            job_name = 'ci-%s' % name
            self.assertIn('branch', job)
            self.assertIn('frequency', job)
            self.assertIn('repo-name', job)
            self.assertIn('timeout', job)
            self.assertNotIn('json', job)
            self.assertNotIn('use-logexporter', job)
            self.assertGreater(job['timeout'], 0, name)
            return job_name

        self.check_bootstrap_yaml(
            'job-configs/kubernetes-jenkins/bootstrap-ci-repo.yaml',
            check, use_json=True)

    def test_bootstrap_ci_soak_yaml(self):
        def check(job, name):
            job_name = 'ci-%s' % name
            self.assertIn('blocker', job)
            self.assertIn('frequency', job)
            self.assertIn('scan', job)
            self.assertNotIn('repo-name', job)
            self.assertNotIn('branch', job)
            self.assertIn('timeout', job)
            self.assertIn('soak-repos', job)
            self.assertNotIn('use-logexporter', job)
            self.assertGreater(job['timeout'], 0, name)

            return job_name

        self.check_bootstrap_yaml(
            'job-configs/kubernetes-jenkins/bootstrap-ci-soak.yaml',
            check, use_json=True)

    def test_bootstrap_ci_dockerpush(self):
        def check(job, name):
            job_name = 'ci-%s' % name
            self.assertIn('branch', job)
            self.assertIn('frequency', job)
            self.assertIn('repo-name', job)
            self.assertIn('timeout', job)
            self.assertNotIn('use-logexporter', job)
            self.assertGreater(job['timeout'], 0, name)
            return job_name

        self.check_bootstrap_yaml(
            'job-configs/kubernetes-jenkins/bootstrap-ci-dockerpush.yaml',
            check, use_json=True)

    def check_job_template(self, tmpl):
        builders = tmpl.get('builders')
        if not isinstance(builders, list):
            self.fail(tmpl)
        self.assertEquals(1, len(builders), builders)
        shell = builders[0]
        if not isinstance(shell, dict):
            self.fail(tmpl)
        self.assertEquals(1, len(shell), tmpl)
        if 'raw' in shell:
            self.assertEquals('maintenance-all-{suffix}', tmpl['name'])
            return
        cmd = shell.get('shell')
        if not isinstance(cmd, basestring):
            self.fail(tmpl)
        self.assertIn('--service-account=', cmd)
        self.assertIn('--upload=', cmd)
        if 'kubernetes-security' in cmd:
            self.assertIn('--upload=\'gs://kubernetes-security-jenkins/pr-logs\'', cmd)
        elif '${{PULL_REFS}}' in cmd:
            self.assertIn('--upload=\'gs://kubernetes-jenkins/pr-logs\'', cmd)
        else:
            self.assertIn('--upload=\'gs://kubernetes-jenkins/logs\'', cmd)

    def add_prow_job(self, job):
        name = job.get('name')
        real_job = {}
        real_job['name'] = name
        if 'spec' in job:
            spec = job.get('spec')
            for container in spec.get('containers'):
                if 'args' in container:
                    for arg in container.get('args'):
                        match = re.match(r'--timeout=(\d+)', arg)
                        if match:
                            real_job['timeout'] = match.group(1)
        if 'pull-' not in name and name in self.realjobs and name not in self.prowjobs:
            self.fail('CI job %s exist in both Jenkins and Prow congfig!' % name)
        if name not in self.realjobs:
            self.realjobs[name] = real_job
            self.prowjobs.append(name)
        if 'run_after_success' in job:
            for sub in job.get('run_after_success'):
                self.add_prow_job(sub)

    def load_prow_yaml(self, path):
        with open(os.path.join(
            os.path.dirname(__file__), path)) as fp:
            doc = yaml.safe_load(fp)

        if 'periodics' not in doc:
            self.fail('No periodics in prow config!')

        if 'presubmits' not in doc:
            self.fail('No presubmits in prow config!')

        for item in doc.get('periodics'):
            self.add_prow_job(item)

        if 'postsubmits' not in doc:
            self.fail('No postsubmits in prow config!')

        presubmits = doc.get('presubmits')
        postsubmits = doc.get('postsubmits')

        for _repo, joblist in presubmits.items() + postsubmits.items():
            for job in joblist:
                self.add_prow_job(job)

    def load_bootstrap_yaml(self, path):
        with open(os.path.join(
            os.path.dirname(__file__), path)) as fp:
            doc = yaml.safe_load(fp)

        project = None
        defined_templates = set()
        for item in doc:
            if not isinstance(item, dict):
                continue
            if isinstance(item.get('job-template'), dict):
                defined_templates.add(item['job-template']['name'])
                self.check_job_template(item['job-template'])
            if not isinstance(item.get('project'), dict):
                continue
            project = item['project']
            self.assertIn('bootstrap-', project.get('name'))
            break
        else:
            self.fail('Could not find bootstrap-pull-jobs project')

        self.assertIn('jobs', project)
        used_templates = {j for j in project['jobs']}
        msg = '\nMissing templates: %s\nUnused templates: %s' % (
            ','.join(used_templates - defined_templates),
            ','.join(defined_templates - used_templates))
        self.assertEquals(defined_templates, used_templates, msg)

        self.assertIn(path, self.yaml_suffix)
        jobs = project.get(self.yaml_suffix[path])
        if not jobs or not isinstance(jobs, list):
            self.fail('Could not find suffix list in %s' % (project))

        real_jobs = {}
        for job in jobs:
            # Things to check on all bootstrap jobs
            if not isinstance(job, dict):
                self.fail('suffix items should be dicts: %s' % jobs)
            self.assertEquals(1, len(job), job)
            name = job.keys()[0]
            real_job = job[name]
            self.assertNotIn(name, real_jobs, 'duplicate job: %s' % name)
            real_jobs[name] = real_job
            real_name = real_job.get('job-name', 'unset-%s' % name)
            if real_name not in self.realjobs:
                self.realjobs[real_name] = real_job
        return real_jobs

    def check_bootstrap_yaml(self, path, check, use_json=False):
        for name, real_job in self.load_bootstrap_yaml(path).iteritems():
            # Things to check on all bootstrap jobs
            if callable(use_json):  # TODO(fejta): gross, but temporary?
                modern = use_json(name)
            else:
                modern = use_json
            cmd = bootstrap.job_script(real_job.get('job-name'), modern)
            path = cmd[0]
            args = cmd[1:]
            self.assertTrue(os.path.isfile(path), (name, path))
            if modern:
                self.assertTrue(all(isinstance(a, basestring) for a in args), args)
                # Ensure the .sh script isn't there
                other = bootstrap.job_script(real_job.get('job-name'), False)
                self.assertFalse(os.path.isfile(other[0]), name)
            else:
                self.assertEquals(1, len(cmd))
                # Ensure the job isn't in the json
                with self.assertRaises(KeyError):
                    bootstrap.job_script(real_job.get('job-name'), True)
                    self.fail(name)
            for key, value in real_job.items():
                if not isinstance(value, (basestring, int)):
                    self.fail('Jobs may not contain child objects %s: %s' % (
                        key, value))
                if '{' in str(value):
                    self.fail('Jobs may not contain {expansions} - %s: %s' % (
                        key, value))  # Use simple strings
            # Things to check on specific flavors.
            job_name = check(real_job, name)
            self.assertTrue(job_name)
            self.assertEquals(job_name, real_job.get('job-name'))

    def get_real_bootstrap_job(self, job):
        key = os.path.splitext(job.strip())[0]
        if not key in self.realjobs:
            for yamlf in self.yaml_suffix:
                self.load_bootstrap_yaml(yamlf)
            self.load_prow_yaml(self.prow_config)
        self.assertIn(key, sorted(self.realjobs))  # sorted for clearer error message
        return self.realjobs.get(key)

    def test_valid_env(self):
        for job, job_path in self.jobs:
            with open(job_path) as fp:
                data = fp.read()
            if 'kops' in job:  # TODO(fejta): update this one too
                continue
            self.assertNotIn(
                'JENKINS_USE_LOCAL_BINARIES=',
                data,
                'Send --extract=local to config.json, not JENKINS_USE_LOCAL_BINARIES in %s' % job)
            self.assertNotIn(
                'JENKINS_USE_EXISTING_BINARIES=',
                data,
                'Send --extract=local to config.json, not JENKINS_USE_EXISTING_BINARIES in %s' % job)  # pylint: disable=line-too-long

    def test_valid_timeout(self):
        """All jobs set a timeout less than 120m or set DOCKER_TIMEOUT."""
        default_timeout = 60
        bad_jobs = set()
        with open(bootstrap.test_infra('jobs/config.json')) as fp:
            config = json.loads(fp.read())

        for job, job_path in self.jobs:
            job_name = job.rsplit('.', 1)[0]
            modern = config.get(job_name, {}).get('scenario') in [
                'kubernetes_e2e',
                'kubernetes_kops_aws',
            ]
            valids = [
                'kubernetes-e2e-',
                'kubernetes-kubemark-',
                'kubernetes-soak-',
                'kubernetes-federation-e2e-',
                'kops-e2e-',
            ]

            if not re.search('|'.join(valids), job):
                continue
            with open(job_path) as fp:
                lines = list(l for l in fp if not l.startswith('#'))
            container_timeout = default_timeout
            kubetest_timeout = None
            for line in lines:  # Validate old pattern no longer used
                if line.startswith('### Reporting'):
                    bad_jobs.add(job)
                if '{rc}' in line:
                    bad_jobs.add(job)
            self.assertFalse(job.endswith('.sh'))
            self.assertTrue(modern, job)

            realjob = self.get_real_bootstrap_job(job)
            self.assertTrue(realjob)
            self.assertIn('timeout', realjob, job)
            container_timeout = int(realjob['timeout'])
            for line in lines:
                if 'DOCKER_TIMEOUT=' in line:
                    self.fail('Set container timeout in prow and/or bootstrap yaml: %s' % job)
                if 'KUBEKINS_TIMEOUT=' in line:
                    self.fail(
                        'Set kubetest --timeout in config.json, not KUBEKINS_TIMEOUT: %s'
                        % job
                    )
            for arg in config[job_name]['args']:
                if arg == '--timeout=None':
                    bad_jobs.add(('Must specify a timeout', job, arg))
                mat = re.match(r'--timeout=(\d+)m', arg)
                if not mat:
                    continue
                kubetest_timeout = int(mat.group(1))
            if kubetest_timeout is None:
                self.fail('Missing timeout: %s' % job)
            if kubetest_timeout > container_timeout:
                bad_jobs.add((job, kubetest_timeout, container_timeout))
            elif kubetest_timeout + 20 > container_timeout:
                bad_jobs.add((
                    'insufficient kubetest leeway',
                    job, kubetest_timeout, container_timeout
                    ))


        if bad_jobs:
            self.fail('\n'.join(str(s) for s in bad_jobs))

    def test_only_jobs(self):
        """Ensure that everything in jobs/ is a valid job name and script."""
        for job, job_path in self.jobs:
            # Jobs should have simple names: letters, numbers, -, .
            self.assertTrue(re.match(r'[.0-9a-z-_]+.(sh|env)', job), job)
            # Jobs should point to a real, executable file
            # Note: it is easy to forget to chmod +x
            self.assertTrue(os.path.isfile(job_path), job_path)
            self.assertFalse(os.path.islink(job_path), job_path)
            if job.endswith('.sh'):
                self.assertTrue(os.access(job_path, os.X_OK|os.R_OK), job_path)
            else:
                self.assertTrue(os.access(job_path, os.R_OK), job_path)

    def test_all_project_are_unique(self):
        # pylint: disable=line-too-long
        allowed_list = {
            # The cos image validation jobs intentionally share projects.
            'ci-kubernetes-e2e-gce-cosdev-k8sdev-default.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosdev-k8sdev-serial.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosdev-k8sdev-slow.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosdev-k8sstable1-default.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosdev-k8sstable1-serial.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosdev-k8sstable1-slow.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosdev-k8sbeta-default.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosdev-k8sbeta-serial.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosdev-k8sbeta-slow.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sdev-default.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sdev-serial.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sdev-slow.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sbeta-default.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sbeta-serial.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sbeta-slow.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sstable1-default.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sstable1-serial.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sstable1-slow.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sstable2-default.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sstable2-serial.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sstable2-slow.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sstable3-default.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sstable3-serial.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sstable3-slow.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sdev-default.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sdev-serial.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sdev-slow.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sbeta-default.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sbeta-serial.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sbeta-slow.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sstable1-default.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sstable1-serial.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sstable1-slow.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sstable2-default.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sstable2-serial.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sstable2-slow.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sstable3-default.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sstable3-serial.env': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sstable3-slow.env': 'ci-kubernetes-e2e-gce-cos*',

            # The ubuntu image validation jobs intentionally share projects.
            'ci-kubernetes-e2e-gce-ubuntudev-k8sdev-default.env': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntudev-k8sdev-serial.env': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntudev-k8sdev-slow.env': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntudev-k8sbeta-default.env': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntudev-k8sbeta-serial.env': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntudev-k8sbeta-slow.env': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntudev-k8sstable1-default.env': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntudev-k8sstable1-serial.env': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntudev-k8sstable1-slow.env': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntustable1-k8sdev-default.env': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntustable1-k8sdev-serial.env': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntustable1-k8sdev-slow.env': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntustable1-k8sstable1-default.env': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntustable1-k8sstable1-serial.env': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntustable1-k8sstable1-slow.env': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntustable1-k8sstable2-default.env': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntustable1-k8sstable2-serial.env': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntustable1-k8sstable2-slow.env': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gke-ubuntustable1-k8sstable1-alphafeatures.env': 'ci-kubernetes-e2e-gke-ubuntu*',
            'ci-kubernetes-e2e-gke-ubuntustable1-k8sstable1-autoscaling.env': 'ci-kubernetes-e2e-gke-ubuntu*',
            'ci-kubernetes-e2e-gke-ubuntustable1-k8sstable1-default.env': 'ci-kubernetes-e2e-gke-ubuntu*',
            'ci-kubernetes-e2e-gke-ubuntustable1-k8sstable1-flaky.env': 'ci-kubernetes-e2e-gke-ubuntu*',
            'ci-kubernetes-e2e-gke-ubuntustable1-k8sstable1-ingress.env': 'ci-kubernetes-e2e-gke-ubuntu*',
            'ci-kubernetes-e2e-gke-ubuntustable1-k8sstable1-reboot.env': 'ci-kubernetes-e2e-gke-ubuntu*',
            'ci-kubernetes-e2e-gke-ubuntustable1-k8sstable1-serial.env': 'ci-kubernetes-e2e-gke-ubuntu*',
            'ci-kubernetes-e2e-gke-ubuntustable1-k8sstable1-slow.env': 'ci-kubernetes-e2e-gke-ubuntu*',
            'ci-kubernetes-e2e-gke-ubuntustable1-k8sstable1-updown.env': 'ci-kubernetes-e2e-gke-ubuntu*',
            # The 1.5 and 1.6 scalability jobs intentionally share projects.
            'ci-kubernetes-e2e-gce-scalability-release-1-7.env': 'ci-kubernetes-e2e-gce-scalability-release-*',
            'ci-kubernetes-e2e-gce-scalability-release-1-6.env': 'ci-kubernetes-e2e-gce-scalability-release-*',
            'ci-kubernetes-e2e-gci-gce-scalability-release-1-7.env': 'ci-kubernetes-e2e-gci-gce-scalability-release-*',
            'ci-kubernetes-e2e-gci-gce-scalability-release-1-6.env': 'ci-kubernetes-e2e-gci-gce-scalability-release-*',
            'ci-kubernetes-e2e-gce-scalability.env': 'ci-kubernetes-e2e-gce-scalability-*',
            'ci-kubernetes-e2e-gce-scalability-canary.env': 'ci-kubernetes-e2e-gce-scalability-*',
            # TODO(fejta): remove these (found while migrating jobs)
            'ci-kubernetes-kubemark-100-gce.env': 'ci-kubernetes-kubemark-*',
            'ci-kubernetes-kubemark-5-gce.env': 'ci-kubernetes-kubemark-*',
            'ci-kubernetes-kubemark-high-density-100-gce.env': 'ci-kubernetes-kubemark-*',
            'ci-kubernetes-kubemark-gce-scale.env': 'ci-kubernetes-scale-*',
            'ci-kubernetes-e2e-gce-enormous-deploy.env': 'ci-kubernetes-scale-*',
            'ci-kubernetes-e2e-gce-enormous-teardown.env': 'ci-kubernetes-scale-*',
            'ci-kubernetes-e2e-gce-large-correctness.env': 'ci-kubernetes-scale-*',
            'ci-kubernetes-e2e-gce-large-performance.env': 'ci-kubernetes-scale-*',
            'ci-kubernetes-e2e-gce-scale-correctness.env': 'ci-kubernetes-scale-*',
            'ci-kubernetes-e2e-gce-scale-performance.env': 'ci-kubernetes-scale-*',
            'ci-kubernetes-e2e-gke-large-correctness.env': 'ci-kubernetes-scale-*',
            'ci-kubernetes-e2e-gke-large-performance.env': 'ci-kubernetes-scale-*',
            'ci-kubernetes-e2e-gke-large-deploy.env': 'ci-kubernetes-scale-*',
            'ci-kubernetes-e2e-gke-large-teardown.env': 'ci-kubernetes-scale-*',
            'ci-kubernetes-e2e-gke-scale-correctness.env': 'ci-kubernetes-scale-*',
            'ci-kubernetes-federation-build.sh': 'ci-kubernetes-federation-*',
            'ci-kubernetes-e2e-gce-federation.env': 'ci-kubernetes-federation-*',
            'pull-kubernetes-federation-e2e-gce.env': 'pull-kubernetes-federation-e2e-gce-*',
            'ci-kubernetes-pull-gce-federation-deploy.env': 'pull-kubernetes-federation-e2e-gce-*',
            'pull-kubernetes-federation-e2e-gce-canary.env': 'pull-kubernetes-federation-e2e-gce-*',
            'ci-kubernetes-pull-gce-federation-deploy-canary.env': 'pull-kubernetes-federation-e2e-gce-*',
            'pull-kubernetes-e2e-gce.env': 'pull-kubernetes-e2e-gce-*',
            'pull-kubernetes-e2e-gce-canary.env': 'pull-kubernetes-e2e-gce-*',
            'ci-kubernetes-e2e-gce.env': 'ci-kubernetes-e2e-gce-*',
            'ci-kubernetes-e2e-gce-canary.env': 'ci-kubernetes-e2e-gce-*',
        }
        for soak_prefix in [
                'ci-kubernetes-soak-gce-1.5',
                'ci-kubernetes-soak-gce-1-7',
                'ci-kubernetes-soak-gce-1.4',
                'ci-kubernetes-soak-gce-1.6',
                'ci-kubernetes-soak-gce-2',
                'ci-kubernetes-soak-gce',
                'ci-kubernetes-soak-gci-gce-1.5',
                'ci-kubernetes-soak-gce-gci',
                'ci-kubernetes-soak-gke-gci',
                'ci-kubernetes-soak-gce-federation',
                'ci-kubernetes-soak-gci-gce-1.4',
                'ci-kubernetes-soak-gci-gce-1.6',
                'ci-kubernetes-soak-gci-gce-1-7',
                'ci-kubernetes-soak-cos-docker-validation',
                'ci-kubernetes-soak-gke',
        ]:
            allowed_list['%s-deploy.env' % soak_prefix] = '%s-*' % soak_prefix
            allowed_list['%s-test.env' % soak_prefix] = '%s-*' % soak_prefix
        # pylint: enable=line-too-long
        projects = collections.defaultdict(set)
        boskos = []
        with open(bootstrap.test_infra('boskos/resources.json')) as fp:
            for rtype in json.loads(fp.read()):
                if rtype['type'] == 'gce-project' or rtype['type'] == 'gke-project':
                    for name in rtype['names']:
                        boskos.append(name)

        with open(bootstrap.test_infra('jobs/config.json')) as fp:
            job_config = json.load(fp)

        for job, job_path in self.jobs:
            with open(job_path) as fp:
                lines = list(fp)
            project = ''
            for line in lines:
                line = line.strip()
                if not line.startswith('PROJECT='):
                    continue
                if '-soak-' in job:  # Soak jobs have deploy/test pairs
                    job = job.replace('-test', '-*').replace('-deploy', '-*')
                if job.startswith('ci-kubernetes-node-'):
                    job = 'ci-kubernetes-node-*'
                if not line.startswith('#') and job.endswith('.sh'):
                    self.assertIn('export', line, line)
                if job.endswith('.sh'):
                    project = re.search(r'PROJECT="([^"]+)"', line).group(1)
                else:
                    project = re.search(r'PROJECT=([^"]+)', line).group(1)
                if project in boskos:
                    self.fail('Project %s cannot be in boskos/resources.json!' % project)
            cfg = job_config.get(job.rsplit('.', 1)[0], {})
            if not project and cfg.get('scenario') == 'kubernetes_e2e':
                for arg in cfg.get('args', []):
                    if not arg.startswith('--gcp-project='):
                        continue
                    project = arg.split('=', 1)[1]
            if project:
                projects[project].add(allowed_list.get(job, job))

        duplicates = [(p, j) for p, j in projects.items() if len(j) > 1]
        if duplicates:
            self.fail('Jobs duplicate projects:\n  %s' % (
                '\n  '.join('%s: %s' % t for t in duplicates)))

    def test_jobs_do_not_source_shell(self):
        for job, job_path in self.jobs:
            if job.startswith('pull-'):
                continue  # No clean way to determine version
            with open(job_path) as fp:
                script = fp.read()
            self.assertFalse(re.search(r'\Wsource ', script), job)
            self.assertNotIn('\n. ', script, job)

    def test_all_bash_jobs_have_errexit(self):
        options = {
            'errexit',
            'nounset',
            'pipefail',
        }
        for job, job_path in self.jobs:
            if not job.endswith('.sh'):
                continue
            with open(job_path) as fp:
                lines = list(fp)
            for option in options:
                expected = 'set -o %s\n' % option
                self.assertIn(
                    expected, lines,
                    '%s not found in %s' % (expected, job_path))

    def _check_env(self, job, setting):
        if not re.match(r'[0-9A-Z_]+=[^\n]*', setting):
            self.fail('[%r]: Env %r: need to follow FOO=BAR pattern' % (job, setting))
        if '#' in setting:
            self.fail('[%r]: Env %r: No inline comments' % (job, setting))
        if '"' in setting or '\'' in setting:
            self.fail('[%r]: Env %r: No quote in env' % (job, setting))
        if '$' in setting:
            self.fail('[%r]: Env %r: Please resolve variables in env' % (job, setting))
        if '{' in setting or '}' in setting:
            self.fail('[%r]: Env %r: { and } are not allowed in env' % (job, setting))
        # also test for https://github.com/kubernetes/test-infra/issues/2829
        # TODO(fejta): sort this list
        black = [
            ('CHARTS_TEST=', '--charts-tests'),
            # TODO(krzyzacy,fejta): This env is still being used in
            # https://github.com/kubernetes/kubernetes/blob/master/cluster/gce/config-test.sh#L95
            # ('CLUSTER_IP_RANGE=', '--test_args=--cluster-ip-range=FOO'),
            ('CLOUDSDK_BUCKET=', '--gcp-cloud-sdk=gs://foo'),
            ('CLUSTER_NAME=', '--cluster=FOO'),
            ('E2E_CLEAN_START=', '--test_args=--clean-start=true'),
            ('E2E_DOWN=', '--down=true|false'),
            ('E2E_MIN_STARTUP_PODS=', '--test_args=--minStartupPods=FOO'),
            ('E2E_NAME=', '--cluster=whatever'),
            ('E2E_PUBLISH_PATH=', '--publish=gs://FOO'),
            ('E2E_REPORT_DIR=', '--test_args=--report-dir=FOO'),
            ('E2E_REPORT_PREFIX=', '--test_args=--report-prefix=FOO'),
            ('E2E_TEST=', '--test=true|false'),
            ('E2E_UPGRADE_TEST=', '--upgrade_args=FOO'),
            ('E2E_UP=', '--up=true|false'),
            ('E2E_OPT=', 'Send kubetest the flags directly'),
            ('FAIL_ON_GCP_RESOURCE_LEAK=', '--check-leaked-resources=true|false'),
            ('FEDERATION_DOWN=', '--down=true|false'),
            ('FEDERATION_UP=', '--up=true|false'),
            ('GINKGO_TEST_ARGS=', '--test_args=FOO'),
            ('GINKGO_UPGRADE_TEST_ARGS=', '--upgrade_args=FOO'),
            ('JENKINS_FEDERATION_PREFIX=', '--stage=gs://FOO'),
            ('JENKINS_GCI_PATCH_K8S=', 'Unused, see --extract docs'),
            ('JENKINS_PUBLISHED_VERSION=', '--extract=V'),
            ('JENKINS_PUBLISHED_SKEW_VERSION=', '--extract=V'),
            ('JENKINS_USE_SKEW_KUBECTL=', 'SKEW_KUBECTL=y'),
            ('JENKINS_USE_SKEW_TESTS=', '--skew'),
            ('JENKINS_SOAK_MODE', '--soak'),
            ('JENKINS_SOAK_PREFIX', '--stage=gs://FOO'),
            ('JENKINS_USE_EXISTING_BINARIES=', '--extract=local'),
            ('JENKINS_USE_LOCAL_BINARIES=', '--extract=none'),
            ('JENKINS_USE_SERVER_VERSION=', '--extract=gke'),
            ('JENKINS_USE_GCI_VERSION=', '--extract=gci/FAMILY'),
            ('JENKINS_USE_GCI_HEAD_IMAGE_FAMILY=', '--extract=gci/FAMILY'),
            ('KUBE_GKE_NETWORK=', '--gcp-network=FOO'),
            ('KUBE_GCE_NETWORK=', '--gcp-network=FOO'),
            ('KUBE_GCE_ZONE=', '--gcp-zone=FOO'),
            ('KUBEKINS_TIMEOUT=', '--timeout=XXm'),
            ('KUBEMARK_TEST_ARGS=', '--test_args=FOO'),
            ('KUBEMARK_TESTS=', '--test_args=--ginkgo.focus=FOO'),
            ('KUBEMARK_MASTER_SIZE=', '--kubemark-master-size=FOO'),
            ('KUBEMARK_NUM_NODES=', '--kubemark-nodes=FOO'),
            ('KUBERNETES_PROVIDER=', '--provider=FOO'),
            ('PERF_TESTS=', '--perf'),
            ('PROJECT=', '--gcp-project=FOO'),
            ('SKEW_KUBECTL=', '--test_args=--kubectl-path=FOO'),
            ('USE_KUBEMARK=', '--kubemark'),
            ('ZONE=', '--gcp-zone=FOO'),
        ]
        for env, fix in black:
            if 'kops' in job and env in [
                    'JENKINS_PUBLISHED_VERSION=',
                    'GINKGO_TEST_ARGS=',
                    'KUBERNETES_PROVIDER=',
            ]:
                continue  # TOOD(fejta): migrate kops jobs
            if setting.startswith(env):
                self.fail('[%s]: Env %s: Convert %s to use %s in jobs/config.json' % (
                    job, setting, env, fix))

    def test_envs_no_export(self):
        for job, job_path in self.jobs:
            if not job.endswith('.env'):
                continue
            with open(job_path) as fp:
                lines = list(fp)
            for line in lines:
                line = line.strip()
                self.assertFalse(line.endswith('\\'))
                if not line:
                    continue
                if line.startswith('#'):
                    continue
                self._check_env(job, line)


    def test_no_bad_vars_in_jobs(self):
        """Searches for jobs that contain ${{VAR}}"""
        for job, job_path in self.jobs:
            with open(job_path) as fp:
                script = fp.read()
            bad_vars = re.findall(r'(\${{.+}})', script)
            if bad_vars:
                self.fail('Job %s contains bad bash variables: %s' % (job, ' '.join(bad_vars)))

    def test_valid_job_envs(self):
        """Validate jobs/config.json."""
        self.load_prow_yaml(self.prow_config)
        config = bootstrap.test_infra('jobs/config.json')
        owners = bootstrap.test_infra('jobs/validOwners.json')
        with open(config) as fp, open(owners) as ownfp:
            config = json.loads(fp.read())
            valid_owners = json.loads(ownfp.read())
            for job in config:
                # onwership assertions
                self.assertIn('sigOwners', config[job], job)
                self.assertIsInstance(config[job]['sigOwners'], list, job)
                self.assertTrue(config[job]['sigOwners'], job) # non-empty
                owners = config[job]['sigOwners']
                for owner in owners:
                    self.assertIsInstance(owner, basestring, job)
                    self.assertIn(owner, valid_owners, job)

                # env assertions
                self.assertTrue('scenario' in config[job], job)
                scenario = bootstrap.test_infra('scenarios/%s.py' % config[job]['scenario'])
                self.assertTrue(os.path.isfile(scenario), job)
                self.assertTrue(os.access(scenario, os.X_OK|os.R_OK), job)
                for arg in config[job].get('args', []):
                    match = re.match(r'--env-file=([^\"]+)\.env', arg)
                    if match:
                        path = bootstrap.test_infra('%s.env' % match.group(1))
                        self.assertTrue(
                            os.path.isfile(path),
                            '%s does not exist for %s' % (path, job))
                    elif 'kops' not in job:
                        match = re.match(r'--cluster=([^\"]+)', arg)
                        if match:
                            cluster = match.group(1)
                            self.assertLessEqual(
                                len(cluster), 20,
                                'Job %r, --cluster should be 20 chars or fewer' % job
                                )
                if config[job]['scenario'] == 'kubernetes_e2e':
                    args = config[job]['args']
                    if job in self.prowjobs:
                        for arg in args:
                            # --mode=local is default now
                            self.assertNotIn('--mode', args, job)
                    else:
                        self.assertIn('--mode=docker', args, job)
                    for arg in args:
                        if "--env=" in arg:
                            self._check_env(job, arg.split("=", 1)[1])
                    if '--provider=gke' in args:
                        self.assertTrue(any('--deployment=gke' in a for a in args),
                                        '%s must use --deployment=gke' % job)
                    if '--deployment=gke' in args:
                        self.assertTrue(any('--gcp-node-image' in a for a in args), job)
                    self.assertNotIn('--charts-tests', args)  # Use --charts
                    if any('--check_version_skew' in a for a in args):
                        self.fail('Use --check-version-skew, not --check_version_skew in %s' % job)
                    if '--check-leaked-resources=true' in args:
                        self.fail('Use --check-leaked-resources (no value) in %s' % job)
                    if '--check-leaked-resources==false' in args:
                        self.fail(
                            'Remove --check-leaked-resources=false (default value) from %s' % job)
                    if (
                            '--env-file=jobs/pull-kubernetes-e2e.env' in args
                            and '--check-leaked-resources' in args):
                        self.fail('PR job %s should not check for resource leaks' % job)
                    # Consider deleting any job with --check-leaked-resources=false
                    if (
                            '--provider=gce' not in args
                            and '--provider=gke' not in args
                            and '--check-leaked-resources' in args
                            and 'generated' not in config[job].get('tags', [])):
                        self.fail('Only GCP jobs can --check-leaked-resources, not %s' % job)
                    if '--mode=local' in args:
                        self.fail('--mode=local is default now, drop that for %s' % job)

                    extracts = [a for a in args if '--extract=' in a]
                    if not extracts:
                        self.fail('e2e job needs --extract flag: %s %s' % (job, args))
                    if any(s in job for s in [
                            'upgrade', 'skew', 'downgrade', 'rollback',
                            'ci-kubernetes-e2e-gce-canary',
                    ]):
                        expected = 2
                    else:
                        expected = 1
                    if len(extracts) != expected:
                        self.fail('Wrong number of --extract args (%d != %d) in %s' % (
                            len(extracts), expected, job))

                    has_image_family = any(
                        [x for x in args if x.startswith('--image-family')])
                    has_image_project = any(
                        [x for x in args if x.startswith('--image-project')])
                    docker_mode = any(
                        [x for x in args if x.startswith('--mode=docker')])
                    if (
                            (has_image_family or has_image_project)
                            and docker_mode):
                        self.fail('--image-family / --image-project is not '
                                  'supported in docker mode: %s' % job)
                    if has_image_family != has_image_project:
                        self.fail('--image-family and --image-project must be'
                                  'both set or unset: %s' % job)

                    if job.startswith('pull-kubernetes-'):
                        self.assertIn('--cluster=', args)
                        if 'gke' in job:
                            stage = 'gs://kubernetes-release-dev/ci'
                            suffix = True
                        elif 'kubeadm' in job:
                            # kubeadm-based jobs use out-of-band .deb artifacts,
                            # not the --stage flag.
                            continue
                        else:
                            stage = 'gs://kubernetes-release-pull/ci/%s' % job
                            suffix = False
                        self.assertIn('--stage=%s' % stage, args)
                        self.assertEquals(
                            suffix,
                            any('--stage-suffix=' in a for a in args),
                            ('--stage-suffix=', suffix, job, args))


if __name__ == '__main__':
    unittest.main()
