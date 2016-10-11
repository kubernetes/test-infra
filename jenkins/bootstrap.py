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
import logging
import os
import select
import socket
import subprocess
import sys
import time


ORIG_CWD = os.getcwd()  # Checkout changes cwd


def Subprocess(cmd, stdin=None, check=True, output=None):
    logging.info('Call subprocess:\n  %s', ' '.join(cmd))
    proc = subprocess.Popen(
        cmd,
        stdin=subprocess.PIPE if stdin is not None else None,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if stdin:
        proc.stdin.write(stdin)
        proc.stdin.close()
    out = []
    code = None
    reads = [proc.stdout.fileno(), proc.stderr.fileno()]
    while reads:
        ret = select.select(reads, [], [], 0.1)
        for fd in ret[0]:
            if fd == proc.stdout.fileno():
                line = proc.stdout.readline()
                if not line:
                    reads.remove(fd)
                    continue
                logging.info(line[:-1])
                if output:
                    out.append(line)
            if fd == proc.stderr.fileno():
                line = proc.stderr.readline()
                if not line:
                    reads.remove(fd)
                    continue
                logging.warning(line[:-1])

    code = proc.wait()
    logging.info('Subprocess %d exited with code %d' % (proc.pid, code))
    lines = output and '\n'.join(out)
    if check and code:
        raise subprocess.CalledProcessError(code, cmd, lines)
    return lines


def PullRef(pull):
    return '+refs/pull/%d/merge' % pull


def Repo(repo):
    return 'https://github.com/%s' % repo


def Checkout(repo, branch, pull):
    if bool(branch) == bool(pull):
        raise ValueError('Must specify one of --branch or --pull')
    if pull:
        ref = PullRef(pull)
    else:
        ref = branch

    git = 'git'
    Subprocess([git, 'init', repo])
    os.chdir(repo)
    # TODO(fejta): cache git calls
    Subprocess([
        git, 'fetch', '--tags', Repo(repo), ref,
    ])
    Subprocess([git, 'checkout', 'FETCH_HEAD'])


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
        Subprocess(cmd, stdin=json.dumps(jdict, indent=2))

    def CopyFile(self, dest, orig):
        cmd = [self.gsutil, '-q', 'cp', '-Z', '-a', 'public-read', orig, dest]
        Subprocess(cmd)

    def UploadText(self, path, txt, cached=True):
        cp_args = ['-a', 'public-read']
        headers = ['-h', 'Content-Type:text/plain']
        if not cached:
            headers += ['-h', 'Cache-Control:private, max-age=0, no-transform']
        cmd = [self.gsutil, '-q'] + headers + [
            'cp'] + cp_args + [
            '-', path,
        ]
        Subprocess(cmd, stdin=txt)


def UploadArtifacts(path, artifacts):
    # Upload artifacts
    if os.path.isdir(artifacts):
        cmd = [
            'gsutil', '-m', '-q',
            '-o', 'GSUtil:use_magicfile=True',
            'cp', '-a', 'public-read', '-r', '-c', '-z', 'log,txt,xml',
            artifacts, path,
        ]
        Subprocess(cmd)


def AppendResult(gsutil, path, build, version, passed):
    """Download a json list and append metadata about this build to it."""
    # TODO(fejta): ensure the object has not changed since downloading.
    # TODO(fejta): delete the clone of this logic in upload-to-gcs.sh
    #                  (this is update_job_result_cache)
    cmd = ['gsutil', '-q', 'cat', path]
    try:
        cache = json.loads(Subprocess(cmd, output=True))
        if not isinstance(cache, list):
            raise ValueError(cache)
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


def Metadata(repo, artifacts):
    # TODO(rmmh): update tooling to expect metadata in finished.json
    path = os.path.join(artifacts or '', 'metadata.json')
    metadata = None
    if os.path.isfile(path):
        try:
            with open(path) as fp:
                metadata = json.loads(fp.read())
        except (IOError, ValueError):
            pass

    if not metadata or not isinstance(metadata, dict):
        metadata = {}
    metadata['repo'] = repo
    return metadata


def Finish(gsutil, paths, success, artifacts, build, version, repo):
    """
    Args:
        paths: a Paths instance.
        success: the build passed if true.
        artifacts: a dir containing artifacts to upload.
        build: identifier of this build.
        version: identifies what version of the code the build tested.
        repo: the target repo
    """

    if os.path.isdir(artifacts):
        UploadArtifacts(paths.artifacts, artifacts)

    # Upload the latest build for the job
    for path in {paths.latest, paths.pr_latest}:
        if path:
            gsutil.UploadText(path, str(build), cached=False)

    # Upload a link to the build path in the directory
    if paths.pr_build_link:
        gsutil.UploadText(paths.pr_build_link, paths.pr_path)

    # github.com/kubernetes/release/find_green_build depends on AppendResult()
    # TODO(fejta): reconsider whether this is how we want to solve this problem.
    AppendResult(gsutil, paths.result_cache, build, version, success)
    if paths.pr_result_cache:
        AppendResult(gsutil, paths.pr_result_cache, build, version, success)

    data = {
        'timestamp': time.time(),
        'result': 'SUCCESS' if success else 'FAILURE',
        'passed': bool(success),
        'metadata': Metadata(repo, artifacts),
    }
    gsutil.UploadJson(paths.finished, data)


def TestInfra(*paths):
    """Return path relative to root of test-infra repo."""
    return os.path.join(ORIG_CWD, os.path.dirname(__file__), '..', *paths)


def Node():
    return os.environ[NODE_ENV]


def Version():
    version_file = 'version'
    if os.path.isfile(version_file):
        # e2e tests which download kubernetes use this path:
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
        return Subprocess(cmd, output=True).strip()

    return 'unknown'


class Paths(object):
    """Links to remote gcs-paths for uploading results."""
    def __init__(
        self,
        artifacts,  # artifacts folder (in build)
        build_log,  # build-log.txt (in build)
        pr_path,  # path to build
        finished,  # finished.json (metadata from end of build)
        latest,   # latest-build.txt (in job)
        pr_build_link,  # file containng pr_path (in job directory)
        pr_latest,  # latest-build.txt (in pr job)
        pr_result_cache,  # jobResultsCache.json (in pr job)
        result_cache,  # jobResultsCache.json (cache of latest results in job)
        started,  # started.json  (metadata from start of build)
    ):
        self.artifacts = artifacts
        self.build_log = build_log
        self.pr_path = pr_path
        self.finished = finished
        self.latest = latest
        self.pr_build_link = pr_build_link
        self.pr_latest = pr_latest
        self.pr_result_cache = pr_result_cache
        self.result_cache = result_cache
        self.started = started



def CIPaths(job, build):
    base = 'gs://kubernetes-jenkins/logs'
    latest = os.path.join(base, job, 'latest-build.txt')
    return Paths(
        artifacts=os.path.join(base, job, build, 'artifacts'),
        build_log=os.path.join(base, job, build, 'build-log.txt'),
        pr_path=None,
        finished=os.path.join(base, job, build, 'finished.json'),
        latest=latest,
        pr_build_link=None,
        pr_latest=None,
        pr_result_cache=None,
        result_cache=os.path.join(base, job, 'jobResultsCache.json'),
        started=os.path.join(base, job, build, 'started.json'),
    )



def PRPaths(repo, job, build, pull):
    pull = str(pull)
    # TODO(rmmh): wouldn't this be better as a dir than prefix?
    if repo == 'kubernetes/kubernetes':
        prefix = ''
    elif repo.startswith('kubernetes/'):
        prefix = repo[len('kubernetes/'):]
    else:
        prefix = repo.replace('/', '_')
    base = 'gs://kubernetes-jenkins/pr-logs'
    pull = prefix + pull
    pr_path = os.path.join(base, 'pull', pull, job, build)
    result_cache = os.path.join(
            base, 'directory', job, 'jobResultsCache.json')
    pr_result_cache = os.path.join(
            base, 'pull', pull, job, 'jobResultsCache.json')
    return Paths(
        artifacts=os.path.join(pr_path, 'artifacts'),
        build_log=os.path.join(pr_path, 'build-log.txt'),
        pr_path=pr_path,
        finished=os.path.join(pr_path, 'finished.json'),
        latest=os.path.join(base, 'directory', job, 'latest-build.txt'),
        pr_build_link=os.path.join(base, 'directory', job, '%s.txt' % build),
        pr_latest=os.path.join(base, 'pull', pull, job, 'latest-build.txt'),
        pr_result_cache=pr_result_cache,
        result_cache=result_cache,
        started=os.path.join(pr_path, 'started.json'),
    )



def Uniq():
    """Return a probably unique suffix for the process."""
    return '%x-%d' % (hash(Node()), os.getpid())


BUILD_ENV = 'BUILD_NUMBER'
BOOTSTRAP_ENV = 'BOOTSTRAP_MIGRATION'
CLOUDSDK_ENV = 'CLOUDSDK_CONFIG'
GCE_KEY_ENV = 'JENKINS_GCE_SSH_PRIVATE_KEY_FILE'
HOME_ENV = 'HOME'
JOB_ENV = 'JOB_NAME'
NODE_ENV = 'NODE_NAME'
SERVICE_ACCOUNT_ENV = 'GOOGLE_APPLICATION_CREDENTIALS'
WORKSPACE_ENV = 'WORKSPACE'


def Build(start):
    # TODO(fejta): right now jenkins sets the BUILD_NUMBER and does this
    #              in an environment variable. Consider migrating this to a
    #              bootstrap.py flag
    if BUILD_ENV not in os.environ:
        # Automatically generate a build number if none is set
        uniq = Uniq()
        autogen = time.strftime('%Y%m%d-%H%M%S' + uniq, time.gmtime(start))
        os.environ[BUILD_ENV] = autogen
    return os.environ[BUILD_ENV]


def KeyFlag(path):
    return '--key-file=%s' % path


def SetupCredentials():

    # TODO(fejta): also check aws, and skip gce check when not necessary.
    if not os.path.isfile(os.environ[GCE_KEY_ENV]):
        raise IOError(
            'Cannot find gce ssh key',
            os.environ[GCE_KEY_ENV],
        )

    # TODO(fejta): stop activating inside the image
    # TODO(fejta): allow use of existing gcloud auth
    if not os.path.isfile(os.environ[SERVICE_ACCOUNT_ENV]):
        raise IOError(
            'Cannot find service account credentials',
            os.environ[SERVICE_ACCOUNT_ENV],
            'Create service account and then create key at '
            'https://console.developers.google.com/iam-admin/serviceaccounts/project',
        )

    Subprocess([
        'gcloud',
        'auth',
        'activate-service-account',
        KeyFlag(os.environ[SERVICE_ACCOUNT_ENV]),
    ])


def SetupLogging(path):
    # See https://docs.python.org/2/library/logging.html#logrecord-attributes
    # [IWEF]yymm HH:MM:SS.uuuuuu file:line] msg
    fmt = '%(levelname).1s%(asctime)s.%(msecs)d000 %(filename)s:%(lineno)d] %(message)s'
    datefmt = '%m%d %H:%M:%S'
    logging.basicConfig(
        level=logging.INFO,
        format=fmt,
        datefmt=datefmt,
    )
    build_log = logging.FileHandler(filename=path, mode='w')
    build_log.setLevel(logging.INFO)
    formatter = logging.Formatter(fmt,datefmt=datefmt)
    build_log.setFormatter(formatter)
    logging.getLogger('').addHandler(build_log)
    return build_log


def SetupMagicEnvironment(job):
    """Set magic environment variables scripts currently expect."""
    home = os.environ[HOME_ENV]
    # TODO(fejta): jenkins sets these values. Consider migrating to using
    #              a secret volume instead and passing the path to this volume
    #              into bootstrap.py as a flag.
    os.environ.setdefault(
        GCE_KEY_ENV,
        os.path.join(home, '.ssh/google_compute_engine'),
    )
    os.environ.setdefault(
        'JENKINS_GCE_SSH_PUBLIC_KEY_FILE',
        os.path.join(home, '.ssh/google_compute_engine.pub'),
    )
    os.environ.setdefault(
        'JENKINS_AWS_SSH_PRIVATE_KEY_FILE',
        os.path.join(home, '.ssh/kube_aws_rsa'),
    )
    os.environ.setdefault(
        'JENKINS_AWS_SSH_PUBLIC_KEY_FILE',
        os.path.join(home, '.ssh/kube_aws_rsa.pub'),
    )
    # SERVICE_ACCOUNT_ENV is a magic gcloud/gcp environment variable that
    # controls the location of service credentials. The e2e go test code depends
    # on this variable, and we also read this value when activating a serivce
    # account
    # TODO(fejta): consider allowing people to pass in their user credentials
    #              when running from a workstation.
    os.environ.setdefault(
        SERVICE_ACCOUNT_ENV,
        os.path.join(home, 'service-account.json'),
    )
    cwd = os.getcwd()
    # TODO(fejta): jenkins sets WORKSPACE and pieces of our infra expect this
    #              value. Consider doing something else in the future.
    os.environ[WORKSPACE_ENV] = cwd
    # TODO(fejta): jenkins/dockerized-e2e-runner.sh also sets HOME to WORKSPACE,
    #              probably to minimize leakage between jobs.
    #              Consider accomplishing this another way.
    os.environ[HOME_ENV] = cwd
    # TODO(fejta): jenkins sets JOB_ENV and pieces of our infra expect this
    #              value. Consider making everything below here agnostic to the
    #              job name.
    if JOB_ENV not in os.environ:
        os.environ[JOB_ENV] = job
    elif os.environ[JOB_ENV] != job:
        raise ValueError(JOB_ENV, os.environ[JOB_ENV], job)
    # TODO(fejta): Magic value to tell our test code not do upload started.json
    # TODO(fejta): delete upload-to-gcs.sh and then this value.
    os.environ[BOOTSTRAP_ENV] = 'yes'
    # TODO(fejta): jenkins sets the node name and our infra expect this value.
    # TODO(fejta): Consider doing something different here.
    if NODE_ENV not in os.environ:
        os.environ[NODE_ENV] = ''.join(socket.gethostname().split('.')[:1])
    # This helps prevent reuse of cloudsdk configuration. It also reduces the
    # risk that running a job on a workstation corrupts the user's config.
    os.environ[CLOUDSDK_ENV] = '%s/.config/gcloud' % cwd


def Job(job):
    return TestInfra('jobs/%s.sh' % job)


def Bootstrap(job, repo, branch, pull, root):
    build_log_path = os.path.abspath('build-log.txt')
    build_log = SetupLogging(build_log_path)
    start = time.time()
    logging.info('Bootstrap %s...' % job)
    build = Build(start)
    logging.info(
        'Check out %s at %s...',
        os.path.join(root, repo),
        pull if pull else branch)
    if not os.path.exists(root):
        os.makedirs(root)
    os.chdir(root)
    Checkout(repo, branch, pull)
    logging.info('Configure environment...')
    version = Version()
    SetupMagicEnvironment(job)
    SetupCredentials()
    if pull:
        paths = PRPaths(repo, job, build, pull)
    else:
        paths = CIPaths(job, build)
    gsutil = GSUtil()
    logging.info('Start %s at %s...' % (build, version))
    Start(gsutil, paths, start, Node(), version)
    success = False
    try:
        cmd = [Job(job)]
        Subprocess(cmd)
        logging.info('PASS: %s' % job)
        success = True
    except subprocess.CalledProcessError:
        logging.error('FAIL: %s' % job)
    logging.info('Upload result and artifacts...')
    Finish(gsutil, paths, success, '_artifacts', build, version, repo)
    logging.getLogger('').removeHandler(build_log)
    build_log.close()
    gsutil.CopyFile(paths.build_log, build_log_path)
    if not success:
        # TODO(fejta/spxtr): we should distinguish infra and non-infra problems
        # by exit code and automatically retrigger after an infra-problem.
        sys.exit(1)


if __name__ == '__main__':
  parser = argparse.ArgumentParser(
      'Checks out a github PR/branch to <basedir>/<repo>/')
  parser.add_argument('--root', default='.', help='Root dir to work with')
  parser.add_argument('--pull', type=int, help='PR number')
  parser.add_argument('--branch', help='Checkout the following branch')
  parser.add_argument('--repo', required=True, help='The kubernetes repository to fetch from')
  parser.add_argument('--job', required=True, help='Name of the job to run')
  args = parser.parse_args()
  Bootstrap(args.job, args.repo, args.branch, args.pull, args.root)
