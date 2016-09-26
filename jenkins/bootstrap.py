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

"""Bootstraps starting a test job.

The following should already be done:
  git checkout http://k8s.io/test-infra
  cd $WORKSPACE
  test-infra/jenkins/bootstrap.py <--repo=R> <--job=J> <--pull=P || --branch=B>

The bootstrapper now does the following:
  # Note start time
  # read test-infra/jenkins/$JOB.json
  # check out repoes defined in $JOB.json
  # note job started
  # call runner defined in $JOB.json
  # upload artifacts (this will change later)
  # upload build-log.txt
  # note job ended

The contract with the runner is as follows:
  * Runner must exit non-zero if job fails for any reason.
"""


import argparse
import json
import os
import socket
import subprocess
import sys
import time


ORIG_CWD = os.getcwd()  # Checkout changes cwd


def Checkout(repo, branch, pull):
    if bool(branch) == bool(pull):
        raise ValueError('Must specify one of --branch or --pull')
    if pull:
        ref = '+refs/pull/%d/merge' % pull
    else:
        ref = branch

    git = 'git'
    subprocess.check_call([git, 'init', repo])
    os.chdir(repo)
    # TODO(fejta): cache git calls
    subprocess.check_call([
        git, 'fetch', '--tags', 'https://github.com/%s' % repo, ref,
    ])
    subprocess.check_call([git, 'checkout', 'FETCH_HEAD'])


def Start(gsutil, paths, stamp, node, version):
    data = {
        'timestamp': stamp,
        'jenkins-node': node,
        'node': node,
        'version': version,
    }
    gsutil.UploadJson(paths.started, data)


class GSUtil(object):
    gsutil = 'gsutil'

    def UploadJson(self, path, jdict):
        cmd = [
            self.gsutil, '-q',
            '-h', 'Content-Type:application/json',
            'cp', '-a', 'public-read',
            '-', path]
        proc = subprocess.Popen(cmd, stdin=subprocess.PIPE)
        proc.communicate(json.dumps(jdict, indent=2))
        if proc.returncode:
            raise subprocess.CalledProcessError(proc.returncode, cmd)

    def UploadText(self, path, txt, cached=True, compressed=False):
        cp_args = ['-a', 'public-read']
        if compressed:
            cp_args.append('-Z')
        headers = ['-h', 'Content-Type:text/plain']
        if not cached:
            headers += ['-h', 'Cache-Control:private, max-age=0, no-transform']
        cmd = [self.gsutil, '-q'] + headers + [
            'cp'] + cp_args + [
            '-', path,
        ]
        proc = subprocess.Popen(cmd, stdin=subprocess.PIPE)
        proc.communicate(txt)
        if proc.returncode:
            raise subprocess.CalledProcessError(proc.returncode, cmd)


def UploadArtifacts(path, artifacts):
    # Upload artifacts
    if os.path.isdir(artifacts):
        cmd = [
            'gsutil', '-m', '-q',
            '-o', 'GSUtil:use_magicfile=True',
            'cp', '-a', 'public-read', '-r', '-c', '-z', 'log,txt,xml',
            artifacts, path,
        ]
        subprocess.check_call(cmd)


def AppendBuild(gsutil, path, build, version, passed):
    cmd = ['gsutil', '-q', 'cat', path]
    try:
        cache = json.loads(subprocess.check_output(cmd))
    except (subprocess.CalledProcessError, ValueError):
        cache = []
    cache.append({
        'version': version,
        'buildnumber': build,
        'passed': bool(passed),
        'result': 'SUCCESS' if passed else 'FAILURE',
    })
    cache = cache[-200:]
    gsutil.UploadJson(path, cache)



def Finish(gsutil, paths, success, artifacts, build, version):
    if os.path.isdir(artifacts):
        UploadArtifacts(paths.artifacts, artifacts)

    # Upload build-log.txt

    # Upload the latest build for the job
    for path in {paths.latest, paths.build_latest}:
        gsutil.UploadText(path, str(build), cached=False)

    # Upload a link to the build path in the directory
    if paths.build_link:
        gsutil.UploadText(paths.build_link, paths.build_path)

    AppendBuild(gsutil, paths.result_cache, build, version, success)
    if paths.build_result_cache:
        AppendBuild(gsutil, paths.build_result_cache, build, version, success)

    # update_job_result_cache ${JENKINS_BUILD_FINISHED}

    data = {
        'timestamp': time.time(),
        'result': 'SUCCESS' if success else 'FAILURE',
        'passed': bool(success),
    }
    # TODO(rmmh): update tooling to expect metadata in finished.json
    metadata = os.path.join(paths.artifacts, 'metadata.json')
    if os.path.isfile(metadata):
        try:
            with open(metadata) as fp:
                val = json.loads(fp.read())
        except (IOError, ValueError):
            val = None
        if val and isinstance(val, dict):
            data['metadata'] = val
    gsutil.UploadJson(paths.finished, data)




def TestInfra(*paths):
    """Return path relative to root of test-infra repo."""
    return os.path.join(ORIG_CWD, os.path.dirname(__file__), '..', *paths)


def Node():
    if 'NODE_NAME' not in os.environ:
        os.environ['NODE_NAME'] = ''.join(socket.gethostname().split('.')[:1])
    return os.environ['NODE_NAME']


def Version():
    version_file = 'version'
    if os.path.isfile(version_file):
        with open(version_file) as fp:
            return fp.read().strip()

    version_script = 'hack/lib/version.sh'
    if os.path.isfile(version_script):
        cmd = [
            'bash', '-c', (
"""
set -o errexit
set -o nounset
export KUBE_ROOT=.
source %s
kube::version::get_version_vars
echo $KUBE_GIT_VERSION
""" % version_script)
        ]
        return subprocess.check_output(cmd).strip()

    return 'unknown'


class CIPath(object):
    base = 'gs://kubernetes-jenkins/logs'
    build_link = None
    build_result_cache = None

    def __init__(self, job, build):
        self.artifacts = os.path.join(self.base, job, build, 'artifacts')
        self.started = os.path.join(self.base, job, build, 'started.json')
        self.finished = os.path.join(self.base, job, build, 'finished.json')
        self.latest = os.path.join(self.base, job, 'latest-build.txt')
        self.result_cache = os.path.join(
            self.base, job, 'jobResultsCache.json')
        self.build_latest = self.latest


class PRPath(object):
    base = 'gs://kubernetes-jenkins/pr-logs'

    def __init__(self, job, build, pull):
        self.build_path = os.path.join(
            self.base, 'pull', pull, job, build)
        self.artifacts = os.path.join(self.build_path, 'artifacts')
        self.started = os.path.join(self.build_path, 'started.json')
        self.finished = os.path.join(self.build_path, 'finished.json')
        self.latest = os.path.join(
            self.base, 'directory', job, 'latest-build.txt')
        self.result_cache = os.path.join(
            self.base, 'directory', job, 'jobResultsCache.json')
        self.build_latest = os.path.join(
            self.base, 'pull', pull, job, 'latest-build.txt')
        self.build_result_cache = os.path.join(
            self.base, 'pull', pull, job, 'jobResultsCache.json')
        self.build_link = os.path.join(
            self.base, 'directory', job, '%s.txt' % build)


def Build(start):
    if 'BUILD_NUMBER' not in os.environ:
        uniq = '%x-%d' % (hash(Node()), os.getpid())
        autogen = time.strftime('%Y%m%d-%H%M%S-' + uniq, time.gmtime())
        os.environ['BUILD_NUMBER'] = autogen
    return os.environ['BUILD_NUMBER']


def SetupCredentials():
    os.environ.setdefault(
        'JENKINS_GCE_SSH_PRIVATE_KEY_FILE',
        os.path.join(os.environ['HOME'], '.ssh/google_compute_engine'),
    )
    os.environ.setdefault(
        'JENKINS_GCE_SSH_PUBLIC_KEY_FILE',
        os.path.join(os.environ['HOME'], '.ssh/google_compute_engine.pub'),
    )
    os.environ.setdefault(
        'JENKINS_AWS_SSH_PRIVATE_KEY_FILE',
        os.path.join(os.environ['HOME'], '.ssh/kube_aws_rsa'),
    )
    os.environ.setdefault(
        'JENKINS_AWS_SSH_PUBLIC_KEY_FILE',
        os.path.join(os.environ['HOME'], '.ssh/kube_aws_rsa.pub'),
    )
    os.environ.setdefault(
        'GOOGLE_APPLICATION_CREDENTIALS',
        os.path.join(os.environ['HOME'], 'service-account.json'),
    )

    # TODO(fejta): also check aws, and skip gce check when not necessary.
    if not os.path.isfile(os.environ['JENKINS_GCE_SSH_PRIVATE_KEY_FILE']):
        raise IOError(
            'Cannot find gce ssh key',
            os.environ['JENKINS_GCE_SSH_PRIVATE_KEY_FILE'],
        )

    # TODO(fejta): stop activating inside the image
    # TODO(fejta): allow use of existing gcloud auth
    if not os.path.isfile(os.environ['GOOGLE_APPLICATION_CREDENTIALS']):
        raise IOError(
            'Cannot find service account credentials',
            os.environ['GOOGLE_APPLICATION_CREDENTIALS'],
            'Create service account and then create key at '
            'https://console.developers.google.com/iam-admin/serviceaccounts/project',
        )

    cwd = os.getcwd()
    os.environ['WORKSPACE'] = cwd
    os.environ['HOME'] = cwd
    os.environ['CLOUDSDK_CONFIG'] = '%s/.config/gcloud' % cwd
    subprocess.check_call([
        'gcloud',
        'auth',
        'activate-service-account',
        '--key-file=%s' % os.environ['GOOGLE_APPLICATION_CREDENTIALS'],
    ])


def Bootstrap(job, repo, branch, pull):
    # TODO(fejta): track output
    start = time.time()
    build = Build(start)
    Checkout(repo, branch, pull)
    version = Version()
    SetupCredentials()
    if pull:
      paths = PRPath(job, build, str(pull))
    else:
      paths = CIPath(job, build)
    if 'JOB_NAME' not in os.environ:
        os.environ['JOB_NAME'] = job
    gsutil = GSUtil()
    Start(gsutil, paths, start, Node(), version)
    try:
        cmd = [TestInfra('jenkins/%s.sh' % job)]
        subprocess.check_call(cmd)
        success = True
    except subprocess.CalledProcessError:
        success = False
    Finish(gsutil, paths, success, '_artifacts', build, version)


if __name__ == '__main__':
  parser = argparse.ArgumentParser('Checks out a github PR/branch to ./<repo>/')
  parser.add_argument('--pull', type=int, help='PR number')
  parser.add_argument('--branch', help='Checkout the following branch')
  parser.add_argument('--repo', required=True, help='The kubernetes repository to fetch from')
  parser.add_argument('--job', required=True, help='Name of the job to run')
  args = parser.parse_args()
  Bootstrap(args.job, args.repo, args.branch, args.pull)
