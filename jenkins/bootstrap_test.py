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

# pylint: disable=protected-access, attribute-defined-outside-init

import argparse
import json
import os
import select
import signal
import subprocess
import tempfile
import time
import unittest

import bootstrap


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
        self.file_data = []
        self.output = {}

    def __call__(self, cmd, *a, **kw):
        self.calls.append((cmd, a, kw))
        for arg in cmd:
            if arg.startswith('/') and os.path.exists(arg):
                self.file_data.append(open(arg).read())
        if kw.get('output') and self.output.get(cmd[0]):
            return self.output[cmd[0]].pop(0)


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


class ConfigureSshKeyTest(unittest.TestCase):
    """Tests for configure_ssh_key()."""
    def test_empty(self):
        """Do not change environ if no ssh key."""
        fake_env = {}
        with Stub(os, 'environ', fake_env):
            with bootstrap.configure_ssh_key(''):
                self.assertFalse(fake_env)

    def test_full(self):
        fake_env = {}
        with Stub(os, 'environ', fake_env):
            with bootstrap.configure_ssh_key('hello there'):
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
            with bootstrap.configure_ssh_key('hello there'):
                self.assertNotEqual(old_env, fake_env)
            self.assertEquals(old_env, fake_env)


class CheckoutTest(unittest.TestCase):
    """Tests for checkout()."""

    def test_clean(self):
        """checkout cleans and resets if asked to."""
        fake = FakeSubprocess()
        with Stub(os, 'chdir', Pass):
            bootstrap.checkout(fake, REPO, REPO, None, PULL, clean=True)

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
                bootstrap.checkout(third_time_charm, REPO, REPO, None, PULL)
        self.assertEquals(expected_attempts, self.tries)

    def test_pull_ref(self):
        """checkout fetches the right ref for a pull."""
        fake = FakeSubprocess()
        with Stub(os, 'chdir', Pass):
            bootstrap.checkout(fake, REPO, REPO, None, PULL)

        expected_ref = bootstrap.pull_ref(PULL)[0][0]
        self.assertTrue(any(
            expected_ref in cmd for cmd, _, _ in fake.calls if 'fetch' in cmd))

    def test_branch(self):
        """checkout fetches the right ref for a branch."""
        fake = FakeSubprocess()
        with Stub(os, 'chdir', Pass):
            bootstrap.checkout(fake, REPO, REPO, BRANCH, None)

        expected_ref = BRANCH
        self.assertTrue(any(
            expected_ref in cmd for cmd, _, _ in fake.calls if 'fetch' in cmd))

    def test_repo(self):
        """checkout initializes and fetches the right repo."""
        fake = FakeSubprocess()
        with Stub(os, 'chdir', Pass):
            bootstrap.checkout(fake, REPO, REPO, BRANCH, None)

        expected_uri = 'https://%s' % REPO
        self.assertTrue(any(
            expected_uri in cmd for cmd, _, _ in fake.calls if 'fetch' in cmd))

    def test_branch_xor_pull(self):
        """Either branch or pull specified, not both."""
        with Stub(os, 'chdir', Bomb):
            with self.assertRaises(ValueError):
                bootstrap.checkout(Bomb, REPO, REPO, None, None)
            with self.assertRaises(ValueError):
                bootstrap.checkout(Bomb, REPO, REPO, BRANCH, PULL)

    def test_happy(self):
        """checkout sanity check."""
        fake = FakeSubprocess()
        with Stub(os, 'chdir', Pass):
            bootstrap.checkout(fake, REPO, REPO, BRANCH, None)

        self.assertTrue(any(
            '--tags' in cmd for cmd, _, _ in fake.calls if 'fetch' in cmd))
        self.assertTrue(any(
            'FETCH_HEAD' in cmd for cmd, _, _ in fake.calls
            if 'checkout' in cmd))

    def test_repo_path(self):
        """checkout repo to different local path."""
        fake = FakeSubprocess()
        repo_path = "foo/bar"
        with Stub(os, 'chdir', Pass):
            bootstrap.checkout(fake, REPO, repo_path, BRANCH, None)

        expected_uri = 'https://%s' % REPO
        self.assertTrue(any(
            expected_uri in cmd for cmd, _, _ in fake.calls if 'fetch' in cmd))

        self.assertTrue(any(
            repo_path in cmd for cmd, _, _ in fake.calls if 'init' in cmd))

class ParseReposTest(unittest.TestCase):
    def test_bare(self):
        """--bare works."""
        args = bootstrap.parse_args(['--job=foo', '--bare'])
        self.assertFalse(bootstrap.parse_repos(args))

    def test_pull_branch_none(self):
        """args.pull and args.branch should be None"""
        args = bootstrap.parse_args(['--job=foo', '--bare'])
        self.assertIsNone(args.pull)
        self.assertIsNone(args.branch)

    def test_plain(self):
        """"--repo=foo equals foo=master."""
        args = bootstrap.parse_args(['--job=foo', '--repo=foo'])
        self.assertEquals(
            {'foo': ('master', '')},
            bootstrap.parse_repos(args))

    def test_branch(self):
        """--repo=foo=branch."""
        args = bootstrap.parse_args(['--job=foo', '--repo=foo=this'])
        self.assertEquals(
            {'foo': ('this', '')},
            bootstrap.parse_repos(args))

    def test_branch_commit(self):
        """--repo=foo=branch:commit works."""
        args = bootstrap.parse_args(['--job=foo', '--repo=foo=this:abcd'])
        self.assertEquals(
            {'foo': ('this:abcd', '')},
            bootstrap.parse_repos(args))

    def test_parse_repos(self):
        """--repo=foo=111,222 works"""
        args = bootstrap.parse_args(['--job=foo', '--repo=foo=111,222'])
        self.assertEquals(
            {'foo': ('', '111,222')},
            bootstrap.parse_repos(args))

    def test_pull_branch(self):
        """--repo=foo=master,111,222 works"""
        args = bootstrap.parse_args(['--job=foo', '--repo=foo=master,111,222'])
        self.assertEquals(
            {'foo': ('', 'master,111,222')},
            bootstrap.parse_repos(args))

    def test_pull_release_branch(self):
        """--repo=foo=release-3.14,&a-fancy%_branch+:abcd,222 works"""
        args = bootstrap.parse_args(['--job=foo',
                                     '--repo=foo=release-3.14,&a-fancy%_branch+:abcd,222'])
        self.assertEquals(
            {'foo': ('', 'release-3.14,&a-fancy%_branch+:abcd,222')},
            bootstrap.parse_repos(args))

    def test_pull_branch_commit(self):
        """--repo=foo=master,111,222 works"""
        args = bootstrap.parse_args(['--job=foo',
                                     '--repo=foo=master:aaa,111:bbb,222:ccc'])
        self.assertEquals(
            {'foo': ('', 'master:aaa,111:bbb,222:ccc')},
            bootstrap.parse_repos(args))

    def test_multi_repo(self):
        """--repo=foo=master,111,222 bar works"""
        args = bootstrap.parse_args(['--job=foo',
                                     '--repo=foo=master:aaa,111:bbb,222:ccc',
                                     '--repo=bar'])
        self.assertEquals(
            {
                'foo': ('', 'master:aaa,111:bbb,222:ccc'),
                'bar': ('master', '')},
            bootstrap.parse_repos(args))


class GSUtilTest(unittest.TestCase):
    """Tests for GSUtil."""
    def test_upload_json(self):
        fake = FakeSubprocess()
        gsutil = bootstrap.GSUtil(fake)
        gsutil.upload_json('fake_path', {'wee': 'fun'})
        self.assertTrue(any(
            'application/json' in a for a in fake.calls[0][0]))
        self.assertEqual(fake.file_data, ['{\n  "wee": "fun"\n}'])

    def test_upload_text_cached(self):
        fake = FakeSubprocess()
        gsutil = bootstrap.GSUtil(fake)
        gsutil.upload_text('fake_path', 'hello world', cached=True)
        self.assertFalse(any(
            'Cache-Control' in a and 'max-age' in a
            for a in fake.calls[0][0]))
        self.assertEqual(fake.file_data, ['hello world'])

    def test_upload_text_default(self):
        fake = FakeSubprocess()
        gsutil = bootstrap.GSUtil(fake)
        gsutil.upload_text('fake_path', 'hello world')
        self.assertFalse(any(
            'Cache-Control' in a and 'max-age' in a
            for a in fake.calls[0][0]))
        self.assertEqual(fake.file_data, ['hello world'])

    def test_upload_text_uncached(self):
        fake = FakeSubprocess()
        gsutil = bootstrap.GSUtil(fake)
        gsutil.upload_text('fake_path', 'hello world', cached=False)
        self.assertTrue(any(
            'Cache-Control' in a and 'max-age' in a
            for a in fake.calls[0][0]))
        self.assertEqual(fake.file_data, ['hello world'])

    def test_upload_text_metalink(self):
        fake = FakeSubprocess()
        gsutil = bootstrap.GSUtil(fake)
        gsutil.upload_text('txt', 'path', additional_headers=['foo: bar'])
        self.assertTrue(any('foo: bar' in a for a in fake.calls[0][0]))
        self.assertEqual(fake.file_data, ['path'])

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


class UploadPodspecTest(unittest.TestCase):
    """ Tests for maybe_upload_podspec() """

    def test_missing_env(self):
        """ Missing env vars return without doing anything. """
        # pylint: disable=no-self-use
        bootstrap.maybe_upload_podspec(None, '', None, {}.get)
        bootstrap.maybe_upload_podspec(None, '', None, {bootstrap.K8S_ENV: 'foo'}.get)
        bootstrap.maybe_upload_podspec(None, '', None, {'HOSTNAME': 'blah'}.get)

    def test_upload(self):
        gsutil = FakeGSUtil()
        call = FakeSubprocess()

        output = 'type: gamma/example\n'
        call.output['kubectl'] = [output]
        artifacts = 'gs://bucket/logs/123/artifacts'
        bootstrap.maybe_upload_podspec(
            call, artifacts, gsutil,
            {bootstrap.K8S_ENV: 'exists', 'HOSTNAME': 'abcd'}.get)

        self.assertEqual(
            call.calls,
            [(['kubectl', 'get', '-oyaml', 'pods/abcd'], (), {'output': True})])
        self.assertEqual(
            gsutil.texts,
            [(('%s/prow_podspec.yaml' % artifacts, output), {})])


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


def fake_environment(set_home=True, set_node=True, set_job=True,
                     set_jenkins_home=True, set_workspace=True,
                     set_artifacts=True, **kwargs):
    if set_home:
        kwargs.setdefault(bootstrap.HOME_ENV, '/fake/home-dir')
    if set_node:
        kwargs.setdefault(bootstrap.NODE_ENV, 'fake-node')
    if set_job:
        kwargs.setdefault(bootstrap.JOB_ENV, JOB)
    if set_jenkins_home:
        kwargs.setdefault(bootstrap.JENKINS_HOME_ENV, '/fake/home-dir')
    if set_workspace:
        kwargs.setdefault(bootstrap.WORKSPACE_ENV, '/fake/workspace')
    if set_artifacts:
        kwargs.setdefault(bootstrap.JOB_ARTIFACTS_ENV, '/fake/workspace/_artifacts')
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
    def test_home_workspace_on_jenkins(self):
        """WORKSPACE/HOME are set correctly for the Jenkins environment."""
        env = fake_environment(set_jenkins_home=True, set_workspace=True)
        cwd = '/fake/random-location'
        old_home = env[bootstrap.HOME_ENV]
        old_workspace = env[bootstrap.WORKSPACE_ENV]
        with Stub(os, 'environ', env):
            with Stub(os, 'getcwd', lambda: cwd):
                bootstrap.setup_magic_environment(JOB, FakeCall())

        self.assertIn(bootstrap.WORKSPACE_ENV, env)
        self.assertNotEquals(env[bootstrap.HOME_ENV],
                             env[bootstrap.WORKSPACE_ENV])
        self.assertNotEquals(old_home, env[bootstrap.HOME_ENV])
        self.assertEquals(cwd, env[bootstrap.HOME_ENV])
        self.assertEquals(old_workspace, env[bootstrap.WORKSPACE_ENV])
        self.assertNotEquals(cwd, env[bootstrap.WORKSPACE_ENV])

    def test_home_workspace_in_k8s(self):
        """WORKSPACE/HOME are set correctly for the kubernetes environment."""
        env = fake_environment(set_jenkins_home=False, set_workspace=True)
        cwd = '/fake/random-location'
        old_home = env[bootstrap.HOME_ENV]
        old_workspace = env[bootstrap.WORKSPACE_ENV]
        with Stub(os, 'environ', env):
            with Stub(os, 'getcwd', lambda: cwd):
                bootstrap.setup_magic_environment(JOB, FakeCall())

        self.assertIn(bootstrap.WORKSPACE_ENV, env)
        self.assertNotEquals(env[bootstrap.HOME_ENV],
                             env[bootstrap.WORKSPACE_ENV])
        self.assertEquals(old_home, env[bootstrap.HOME_ENV])
        self.assertNotEquals(cwd, env[bootstrap.HOME_ENV])
        self.assertEquals(old_workspace, env[bootstrap.WORKSPACE_ENV])
        self.assertNotEquals(cwd, env[bootstrap.WORKSPACE_ENV])

    def test_workspace_always_set(self):
        """WORKSPACE is set to cwd when unset in initial environment."""
        env = fake_environment(set_workspace=False)
        cwd = '/fake/random-location'
        with Stub(os, 'environ', env):
            with Stub(os, 'getcwd', lambda: cwd):
                bootstrap.setup_magic_environment(JOB, FakeCall())

        self.assertIn(bootstrap.WORKSPACE_ENV, env)
        self.assertEquals(cwd, env[bootstrap.HOME_ENV])
        self.assertEquals(cwd, env[bootstrap.WORKSPACE_ENV])

    def test_job_env_mismatch(self):
        env = fake_environment()
        with Stub(os, 'environ', env):
            self.assertNotEquals('this-is-a-job', env[bootstrap.JOB_ENV])
            bootstrap.setup_magic_environment('this-is-a-job', FakeCall())
            self.assertEquals('this-is-a-job', env[bootstrap.JOB_ENV])

    def test_expected(self):
        env = fake_environment()
        del env[bootstrap.JOB_ENV]
        del env[bootstrap.NODE_ENV]
        with Stub(os, 'environ', env):
            # call is only used to git show the HEAD commit, so give a fake
            # timestamp in return
            bootstrap.setup_magic_environment(JOB, lambda *a, **kw: '123456\n')

        def check(name):
            self.assertIn(name, env)

        # Some of these are probably silly to check...
        # TODO(fejta): remove as many of these from our infra as possible.
        check(bootstrap.JOB_ENV)
        check(bootstrap.CLOUDSDK_ENV)
        check(bootstrap.BOOTSTRAP_ENV)
        check(bootstrap.WORKSPACE_ENV)
        self.assertNotIn(bootstrap.SERVICE_ACCOUNT_ENV, env)
        self.assertEquals(env[bootstrap.SOURCE_DATE_EPOCH_ENV], '123456')

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
                bootstrap.setup_magic_environment(JOB, FakeCall())


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
    scenario = ''

    def __init__(self, **kw):
        self.branch = BRANCH
        self.pull = PULL
        self.repo = [REPO]
        self.extra_job_args = []
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
                test_bootstrap(branch=None, pull=PULL, root='.')
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

    def test_job_script_expands_vars(self):
        fake = {
            'HELLO': 'awesome',
            'WORLD': 'sauce',
        }
        with Stub(os, 'environ', fake):
            actual = bootstrap.job_args(
                ['$HELLO ${WORLD}', 'happy', '${MISSING}'])
        self.assertEquals(['awesome sauce', 'happy', '${MISSING}'], actual)


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
        subprocess.check_call(['cp', '-r', bootstrap.test_infra('jenkins/fake'), fakerepo])
        subprocess.check_call(['git', 'add', 'fake'])
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
        master_commit_date = int(subprocess.check_output(
            ['git', 'show', '-s', '--format=format:%ct', head_sha()]))
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
        subprocess.check_call(['ls'])
        test_bootstrap(
            job='fake-pr',
            repo=self.REPO,
            branch=None,
            pull=pull,
            root=self.root_workspace)
        head_commit_date = int(subprocess.check_output(
            ['git', 'show', '-s', '--format=format:%ct', 'test']))
        # Since there were 2 PRs merged, we expect the timestamp of the latest
        # commit on the 'test' branch to be 2 more than master.
        self.assertEqual(head_commit_date, master_commit_date + 2)

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
        with self.assertRaises(ValueError):
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


    def test_extra_job_args(self):
        args = bootstrap.parse_args(['--repo=R', '--job=j', '--', '--foo=bar', '--baz=quux'])
        self.assertEquals(args.extra_job_args, ['--foo=bar', '--baz=quux'])


if __name__ == '__main__':
    unittest.main()
