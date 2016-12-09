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

# Need to figure out why this only fails on travis
# pylint: disable=bad-continuation

"""Bootstraps starting a test job.

The following should already be done:
  git checkout http://k8s.io/test-infra
  cd $WORKSPACE
  test-infra/jenkins/bootstrap.py <--repo=R || --bare> <--job=J> <--pull=P || --branch=B>

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
import pipes
import random
import re
import select
import socket
import subprocess
import sys
import time

ORIG_CWD = os.getcwd()  # Checkout changes cwd


def call(cmd, stdin=None, check=True, output=None):
    """Start a subprocess."""
    logging.info('Call:  %s', ' '.join(pipes.quote(c) for c in cmd))
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
        for fdesc in ret[0]:
            if fdesc == proc.stdout.fileno():
                line = proc.stdout.readline()
                if not line:
                    reads.remove(fdesc)
                    continue
                if output:
                    out.append(line)
                else:
                    logging.info(line[:-1])
            if fdesc == proc.stderr.fileno():
                line = proc.stderr.readline()
                if not line:
                    reads.remove(fdesc)
                    continue
                logging.warning(line[:-1])
        if not ret[0]:
            if proc.poll() is not None:
                break  # process exited without closing pipes (timeout?)

    code = proc.wait()
    logging.info('process %d exited with code %d', proc.pid, code)
    lines = output and '\n'.join(out)
    if check and code:
        raise subprocess.CalledProcessError(code, cmd, lines)
    return lines


def pull_has_shas(pull):
    """Determine if a pull reference specifies shas (contains ':')

    >>> pull_has_shas('master:abcd')
    True
    >>> pull_has_shas('123')
    False
    >>> pull_has_shas(123)
    False
    >>> pull_has_shas(None)
    False
    """
    return isinstance(pull, basestring) and ':' in pull


def pull_numbers(pull):
    """Turn a pull reference list into a list of PR numbers to merge.

    >>> pull_numbers(123)
    ['123']
    >>> pull_numbers('master:abcd')
    []
    >>> pull_numbers('master:abcd,123:qwer,124:zxcv')
    ['123', '124']
    """
    if pull_has_shas(pull):
        return [r.split(':')[0] for r in pull.split(',')][1:]
    else:
        return [str(pull)]


def pull_ref(pull):
    """Turn a PR number of list of refs into specific refs to fetch and check out.

    >>> pull_ref('123')
    (['+refs/pull/123/merge'], ['FETCH_HEAD'])
    >>> pull_ref('master:abcd,123:effe')
    (['master', '+refs/pull/123/head:refs/pr/123'], ['abcd', 'effe'])
    """
    if pull_has_shas(pull):
        refs = []
        checkouts = []
        for name, sha in (x.split(':') for x in pull.split(',')):
            if len(refs) == 0:
                # first entry is the branch spec ("master")
                refs.append(name)
            else:
                num = int(name)
                refs.append('+refs/pull/%d/head:refs/pr/%d' % (num, num))
            checkouts.append(sha)
        return refs, checkouts
    else:
        return ['+refs/pull/%d/merge' % int(pull)], ['FETCH_HEAD']


def repository(repo):
    """Return the url associated with the repo."""
    if repo.startswith('k8s.io/'):
        return 'https://github.com/kubernetes/%s' % (repo[len('k8s.io/'):])
    return 'https://%s' % repo


def random_sleep(attempt):
    """Sleep 2**attempt seconds with a random fractional offset."""
    time.sleep(random.random() + attempt ** 2)


def checkout(repo, branch, pull):
    """Fetch and checkout the repository at the specified branch/pull."""
    if bool(branch) == bool(pull):
        raise ValueError('Must specify one of --branch or --pull')

    if pull:
        refs, checkouts = pull_ref(pull)
    else:
        refs, checkouts = [branch], ['FETCH_HEAD']

    git = 'git'
    call([git, 'init', repo])
    os.chdir(repo)

    # To make a merge commit, a user needs to be set. It's okay to use a dummy
    # user here, since we're not exporting the history.
    call([git, 'config', '--local', 'user.name', 'K8S Bootstrap'])
    call([git, 'config', '--local', 'user.email', 'k8s_bootstrap@localhost'])
    retries = 3
    for attempt in range(retries):
        try:
            call([git, 'fetch', '--tags', repository(repo)] + refs)
            break
        except subprocess.CalledProcessError as cpe:
            if attempt >= retries - 1:
                raise
            if cpe.returncode != 128:
                raise
            logging.warning('git fetch failed')
            random_sleep(attempt)
    call([git, 'checkout', '-B', 'test', checkouts[0]])
    for ref, head in zip(refs, checkouts)[1:]:
        call(['git', 'merge', '--no-ff', '-m', 'Merge %s' % ref, head])


def start(gsutil, paths, stamp, node_name, version, pull):
    """Construct and upload started.json."""
    data = {
        'timestamp': int(stamp),
        'jenkins-node': node_name,
        'node': node_name,
    }
    if version:
        data['repo-version'] = version
        data['version'] = version  # TODO(fejta): retire
    if pull_has_shas(pull):
        data['pull'] = pull
    gsutil.upload_json(paths.started, data)
    # Upload a link to the build path in the directory
    if paths.pr_build_link:
        gsutil.upload_text(paths.pr_build_link, paths.pr_path)


class GSUtil(object):
    """A helper class for making gsutil commands."""
    gsutil = 'gsutil'

    def stat(self, path):
        """Return metadata about the object, such as generation."""
        cmd = [self.gsutil, 'stat', path]
        return call(cmd, output=True)

    def upload_json(self, path, jdict, generation=None):
        """Upload the dictionary object to path."""
        if generation is not None:  # generation==0 means object does not exist
            gen = ['-h', 'x-goog-if-generation-match:%s' % generation]
        else:
            gen = []
        cmd = [
            self.gsutil, '-q',
            '-h', 'Content-Type:application/json'] + gen + [
            'cp', '-', path]
        call(cmd, stdin=json.dumps(jdict, indent=2))

    def copy_file(self, dest, orig):
        """Copy the file to the specified path using compressed encoding."""
        cmd = [self.gsutil, '-q', 'cp', '-Z', orig, dest]
        call(cmd)

    def upload_text(self, path, txt, cached=True):
        """Copy the text to path, optionally disabling caching."""
        headers = ['-h', 'Content-Type:text/plain']
        if not cached:
            headers += ['-h', 'Cache-Control:private, max-age=0, no-transform']
        cmd = [self.gsutil, '-q'] + headers + ['cp', '-', path]
        call(cmd, stdin=txt)

    def cat(self, path, generation):
        """Return contents of path#generation"""
        cmd = [self.gsutil, '-q', 'cat', '%s#%s' % (path, generation)]
        return call(cmd, output=True)


    def upload_artifacts(self, path, artifacts):
        """Upload artifacts to the specified path."""
        # Upload artifacts
        if os.path.isdir(artifacts):
            cmd = [
                self.gsutil, '-m', '-q',
                '-o', 'GSUtil:use_magicfile=True',
                'cp', '-r', '-c', '-z', 'log,txt,xml',
                artifacts, path,
            ]
            call(cmd)


def append_result(gsutil, path, build, version, passed):
    """Download a json list and append metadata about this build to it."""
    # TODO(fejta): delete the clone of this logic in upload-to-gcs.sh
    #                  (this is update_job_result_cache)
    end = time.time() + 300  # try for up to five minutes
    errors = 0
    while time.time() < end:
        if errors:
            random_sleep(min(errors, 3))
        try:
            out = gsutil.stat(path)
            gen = re.search(r'Generation:\s+(\d+)', out).group(1)
        except subprocess.CalledProcessError:
            gen = 0
        if gen:
            try:
                cache = json.loads(gsutil.cat(path, gen))
                if not isinstance(cache, list):
                    raise ValueError(cache)
            except ValueError:
                cache = []
            except subprocess.CalledProcessError:  # gen doesn't exist
                errors += 1
                continue
        else:
            cache = []
        cache.append({
            'version': version,  # TODO(fejta): retire
            'job-version': version,
            'buildnumber': build,
            'passed': bool(passed),
            'result': 'SUCCESS' if passed else 'FAILURE',
        })
        cache = cache[-300:]
        try:
            gsutil.upload_json(path, cache, generation=gen)
            return
        except subprocess.CalledProcessError:
            logging.warning('Failed to append to %s#%s', path, gen)
        errors += 1


def metadata(repo, artifacts):
    """Return metadata associated for the build, including inside artifacts."""
    # TODO(rmmh): update tooling to expect metadata in finished.json
    path = os.path.join(artifacts or '', 'metadata.json')
    meta = None
    if os.path.isfile(path):
        try:
            with open(path) as fp:
                meta = json.loads(fp.read())
        except (IOError, ValueError):
            pass

    if not meta or not isinstance(meta, dict):
        meta = {}
    meta['repo'] = repo
    return meta


def finish(gsutil, paths, success, artifacts, build, version, repo):
    """
    Args:
        paths: a Paths instance.
        success: the build passed if true.
        artifacts: a dir containing artifacts to upload.
        build: identifier of this build.
        version: identifies what version of the code the build tested.
        repo: the target repo
    """

    if os.path.isdir(artifacts) and any(f for _, _, f in os.walk(artifacts)):
        try:
            gsutil.upload_artifacts(paths.artifacts, artifacts)
        except subprocess.CalledProcessError:
            logging.warning('Failed to upload artifacts')

    meta = metadata(repo, artifacts)
    if not version:
        version = meta.get('job-version')
    if not version:  # TODO(fejta): retire
        version = meta.get('version')
    # github.com/kubernetes/release/find_green_build depends on append_result()
    # TODO(fejta): reconsider whether this is how we want to solve this problem.
    append_result(gsutil, paths.result_cache, build, version, success)
    if paths.pr_result_cache:
        append_result(gsutil, paths.pr_result_cache, build, version, success)

    data = {
        # TODO(fejta): update utils.go in contrib to accept a float
        'timestamp': int(time.time()),
        'result': 'SUCCESS' if success else 'FAILURE',
        'passed': bool(success),
        'metadata': meta,
    }
    if version:
        data['job-version'] = version
        data['version'] = version  # TODO(fejta): retire
    gsutil.upload_json(paths.finished, data)

    # Upload the latest build for the job.
    # Do this last, since other tools expect the rest of the data to be
    # published when this file is created.
    for path in {paths.latest, paths.pr_latest}:
        if path:
            try:
                gsutil.upload_text(path, str(build), cached=False)
            except subprocess.CalledProcessError:
                logging.warning('Failed to update %s', path)


def test_infra(*paths):
    """Return path relative to root of test-infra repo."""
    return os.path.join(ORIG_CWD, os.path.dirname(__file__), '..', *paths)


def node():
    """Return the name of the node running the build."""
    # TODO(fejta): jenkins sets the node name and our infra expect this value.
    # TODO(fejta): Consider doing something different here.
    if NODE_ENV not in os.environ:
        os.environ[NODE_ENV] = ''.join(socket.gethostname().split('.')[:1])
    return os.environ[NODE_ENV]


def find_version():
    """Determine and return the version of the build."""
    # TODO(fejta): once job-version is functional switch this to
    # git rev-parse [--short=N] HEAD^{commit}
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
        return call(cmd, output=True).strip()

    return 'unknown'


class Paths(object):  # pylint: disable=too-many-instance-attributes,too-few-public-methods
    """Links to remote gcs-paths for uploading results."""
    def __init__(  # pylint: disable=too-many-arguments
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



def ci_paths(job, build):
    """Return a Paths() instance for a continuous build."""
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



def pr_paths(repo, job, build, pull):
    """Return a Paths() instance for a PR."""
    pull = str(pull)
    if repo in ['k8s.io/kubernetes', 'kubernetes/kubernetes']:
        prefix = ''
    elif repo.startswith('k8s.io/'):
        prefix = repo[len('k8s.io/'):]
    elif repo.startswith('kubernetes/'):
        prefix = repo[len('kubernetes/'):]
    elif repo.startswith('github.com/'):
        prefix = repo[len('github.com/'):].replace('/', '_')
    else:
        prefix = repo.replace('/', '_')
    base = 'gs://kubernetes-jenkins/pr-logs'
    # Batch merges are those with more than one PR specified.
    pr_nums = pull_numbers(pull)
    if len(pr_nums) > 1:
        pull = os.path.join(prefix, 'batch')
    else:
        pull = os.path.join(prefix, pr_nums[0])
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



BUILD_ENV = 'BUILD_NUMBER'
BOOTSTRAP_ENV = 'BOOTSTRAP_MIGRATION'
CLOUDSDK_ENV = 'CLOUDSDK_CONFIG'
GCE_KEY_ENV = 'JENKINS_GCE_SSH_PRIVATE_KEY_FILE'
HOME_ENV = 'HOME'
JOB_ENV = 'JOB_NAME'
NODE_ENV = 'NODE_NAME'
SERVICE_ACCOUNT_ENV = 'GOOGLE_APPLICATION_CREDENTIALS'
WORKSPACE_ENV = 'WORKSPACE'


def build_name(started):
    """Return the unique(ish) string representing this build."""
    # TODO(fejta): right now jenkins sets the BUILD_NUMBER and does this
    #              in an environment variable. Consider migrating this to a
    #              bootstrap.py flag
    if BUILD_ENV not in os.environ:
        # Automatically generate a build number if none is set
        uniq = '%x-%d' % (hash(node()), os.getpid())
        autogen = time.strftime('%Y%m%d-%H%M%S-' + uniq, time.gmtime(started))
        os.environ[BUILD_ENV] = autogen
    return os.environ[BUILD_ENV]


def setup_credentials():
    """Run some sanity checks and activate gcloud."""
    # TODO(fejta): stop activating inside the image
    # TODO(fejta): allow use of existing gcloud auth
    if not os.path.isfile(os.environ[SERVICE_ACCOUNT_ENV]):
        raise IOError(
            'Cannot find service account credentials',
            os.environ[SERVICE_ACCOUNT_ENV],
            'Create service account and then create key at '
            'https://console.developers.google.com/iam-admin/serviceaccounts/project',  # pylint: disable=line-too-long
        )

    call([
        'gcloud',
        'auth',
        'activate-service-account',
        '--key-file=%s' % os.environ[SERVICE_ACCOUNT_ENV],
    ])


def setup_logging(path):
    """Initialize logging to screen and path."""
    # See https://docs.python.org/2/library/logging.html#logrecord-attributes
    # [IWEF]mmdd HH:MM:SS.mmm] msg
    fmt = '%(levelname).1s%(asctime)s.%(msecs)03d] %(message)s'  # pylint: disable=line-too-long
    datefmt = '%m%d %H:%M:%S'
    logging.basicConfig(
        level=logging.INFO,
        format=fmt,
        datefmt=datefmt,
    )
    build_log = logging.FileHandler(filename=path, mode='w')
    build_log.setLevel(logging.INFO)
    formatter = logging.Formatter(fmt, datefmt=datefmt)
    build_log.setFormatter(formatter)
    logging.getLogger('').addHandler(build_log)
    return build_log


def setup_magic_environment(job):
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
    # This helps prevent reuse of cloudsdk configuration. It also reduces the
    # risk that running a job on a workstation corrupts the user's config.
    os.environ[CLOUDSDK_ENV] = '%s/.config/gcloud' % cwd


def job_script(job):
    """Return path to script for job."""
    if "ci-kubernetes-e2e" in job:
        return test_infra('jobs/%s.env' % job)
    return test_infra('jobs/%s.sh' % job)


GUBERNATOR = 'https://k8s-gubernator.appspot.com/build'


def gubernator_uri(paths):
    """Return a gubernator link for this build."""
    job = os.path.dirname(paths.build_log)
    if job.startswith('gs:/'):
        return job.replace('gs:/', GUBERNATOR, 1)
    return job


def bootstrap(job, repo, branch, pull, root):
    """Clone repo at pull/branch into root and run job script."""
    build_log_path = os.path.abspath('build-log.txt')
    build_log = setup_logging(build_log_path)
    started = time.time()
    logging.info('Bootstrap %s...', job)
    build = build_name(started)
    logging.info(
        'Check out %s at %s...',
        os.path.join(root, repo or ''),
        pull if pull else branch)
    if not os.path.exists(root):
        os.makedirs(root)
    os.chdir(root)
    if repo:
        checkout(repo, branch, pull)
    elif branch or pull:
        raise ValueError('--branch and --pull require --repo', branch, pull)
    logging.info('Configure environment...')
    if repo:
        version = find_version()
    else:
        version = ''
    setup_magic_environment(job)
    setup_credentials()
    if pull:
        paths = pr_paths(repo, job, build, pull)
    else:
        paths = ci_paths(job, build)
    logging.info('Gubernator results at %s', gubernator_uri(paths))
    gsutil = GSUtil()
    logging.info('Start %s at %s...', build, version)
    start(gsutil, paths, started, node(), version, pull)
    success = False
    try:
        cmd = [job_script(job)]
        call(cmd)
        logging.info('PASS: %s', job)
        success = True
    except subprocess.CalledProcessError:
        logging.error('FAIL: %s', job)
    logging.info('Upload result and artifacts...')
    logging.info('Gubernator results at %s', gubernator_uri(paths))
    try:
        finish(gsutil, paths, success, '_artifacts', build, version, repo)
    except subprocess.CalledProcessError:  # Still try to upload build log
        success = False
    logging.getLogger('').removeHandler(build_log)
    build_log.close()
    gsutil.copy_file(paths.build_log, build_log_path)
    if not success:
        # TODO(fejta/spxtr): we should distinguish infra and non-infra problems
        # by exit code and automatically retrigger after an infra-problem.
        sys.exit(1)


if __name__ == '__main__':
    PARSER = argparse.ArgumentParser(
        'Checks out a github PR/branch to <basedir>/<repo>/')
    PARSER.add_argument('--root', default='.', help='Root dir to work with')
    PARSER.add_argument('--pull',
        help='PR number, or list of PR:sha pairs like master:abcd,12:ef12,45:ff65')
    PARSER.add_argument('--branch', help='Checkout the following branch')
    PARSER.add_argument('--repo', help='The repository to fetch from')
    PARSER.add_argument('--bare', action='store_true', help='Do not check out a repository')
    PARSER.add_argument('--job', required=True, help='Name of the job to run')
    ARGS = PARSER.parse_args()
    if bool(ARGS.repo) == bool(ARGS.bare):
        raise argparse.ArgumentTypeError(
            'Expected --repo xor --bare:', ARGS.repo, ARGS.bare)
    bootstrap(ARGS.job, ARGS.repo, ARGS.branch, ARGS.pull, ARGS.root)
