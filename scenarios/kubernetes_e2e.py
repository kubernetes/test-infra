#!/usr/bin/env python3

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
# pylint: disable=bad-continuation

"""Runs kubernetes e2e test with specified config"""

import argparse
import hashlib
import os
import shutil
import subprocess
import sys
import urllib.request, urllib.error, urllib.parse
import time

ORIG_CWD = os.getcwd()  # Checkout changes cwd


def test_infra(*paths):
    """Return path relative to root of test-infra repo."""
    return os.path.join(ORIG_CWD, os.path.dirname(__file__), '..', *paths)


def check(*cmd):
    """Log and run the command, raising on errors."""
    print('Run:', cmd, file=sys.stderr)
    subprocess.check_call(cmd)


def check_output(*cmd):
    """Log and run the command, raising on errors, return output"""
    print('Run:', cmd, file=sys.stderr)
    return subprocess.check_output(cmd)


def check_env(env, *cmd):
    """Log and run the command with a specific env, raising on errors."""
    print('Environment:', file=sys.stderr)
    for key, value in sorted(env.items()):
        print('%s=%s' % (key, value), file=sys.stderr)
    print('Run:', cmd, file=sys.stderr)
    subprocess.check_call(cmd, env=env)


def kubekins(tag):
    """Return full path to kubekins-e2e:tag."""
    return 'gcr.io/k8s-staging-test-infra/kubekins-e2e:%s' % tag


def parse_env(env):
    """Returns (FOO, BAR=MORE) for FOO=BAR=MORE."""
    return env.split('=', 1)


class LocalMode(object):
    """Runs e2e tests by calling kubetest."""
    def __init__(self, workspace, artifacts):
        self.command = 'kubetest'
        self.workspace = workspace
        self.artifacts = artifacts
        self.env = []
        self.os_env = []
        self.env_files = []
        self.add_environment(
            'HOME=%s' % workspace,
            'WORKSPACE=%s' % workspace,
            'PATH=%s' % os.getenv('PATH'),
        )

    def add_environment(self, *envs):
        """Adds FOO=BAR to the list of environment overrides."""
        self.env.extend(parse_env(e) for e in envs)

    def add_os_environment(self, *envs):
        """Adds FOO=BAR to the list of os environment overrides."""
        self.os_env.extend(parse_env(e) for e in envs)

    def add_file(self, env_file):
        """Reads all FOO=BAR lines from env_file."""
        with open(env_file) as fp:
            for line in fp:
                line = line.rstrip()
                if not line or line.startswith('#'):
                    continue
                self.env_files.append(parse_env(line))

    def add_env(self, env):
        self.env_files.append(parse_env(env))

    def add_gce_ssh(self, priv, pub):
        """Copies priv, pub keys to $WORKSPACE/.ssh."""
        ssh_dir = os.path.join(self.workspace, '.ssh')
        if not os.path.isdir(ssh_dir):
            os.makedirs(ssh_dir)

        gce_ssh = os.path.join(ssh_dir, 'google_compute_engine')
        gce_pub = os.path.join(ssh_dir, 'google_compute_engine.pub')
        shutil.copy(priv, gce_ssh)
        shutil.copy(pub, gce_pub)
        self.add_environment(
            'JENKINS_GCE_SSH_PRIVATE_KEY_FILE=%s' % gce_ssh,
            'JENKINS_GCE_SSH_PUBLIC_KEY_FILE=%s' % gce_pub,
        )

    @staticmethod
    def add_service_account(path):
        """Returns path."""
        return path

    def add_k8s(self, *a, **kw):
        """Add specified k8s.io repos (noop)."""
        pass

    def start(self, args):
        """Starts kubetest."""
        print('starts with local mode', file=sys.stderr)
        env = {}
        env.update(self.os_env)
        env.update(self.env_files)
        env.update(self.env)
        check_env(env, self.command, *args)


def cluster_name(cluster, tear_down_previous=False):
    """Return or select a cluster name."""
    if cluster:
        return cluster
    # Create a suffix based on the build number and job name.
    # This ensures no conflict across runs of different jobs (see #7592).
    # For PR jobs, we use PR number instead of build number to ensure the
    # name is constant across different runs of the presubmit on the PR.
    # This helps clean potentially leaked resources from earlier run that
    # could've got evicted midway (see #7673).
    job_type = os.getenv('JOB_TYPE')
    if job_type == 'batch':
        suffix = 'batch-%s' % os.getenv('BUILD_ID', 0)
    elif job_type == 'presubmit' and tear_down_previous:
        suffix = '%s' % os.getenv('PULL_NUMBER', 0)
    else:
        suffix = '%s' % os.getenv('BUILD_ID', 0)
    if len(suffix) > 10:
        suffix = hashlib.md5(suffix.encode('utf-8')).hexdigest()[:10]
    job_hash = hashlib.md5(os.getenv('JOB_NAME', '').encode('utf-8')).hexdigest()[:5]
    return 'e2e-%s-%s' % (suffix, job_hash)


def read_gcs_path(gcs_path):
    """reads a gcs path (gs://...) by HTTP GET to storage.googleapis.com"""
    link = gcs_path.replace('gs://', 'https://storage.googleapis.com/')
    loc = urllib.request.urlopen(link).read()
    print("Read GCS Path: %s" % loc, file=sys.stderr)
    return loc


def get_shared_gcs_path(gcs_shared, use_shared_build):
    """return the shared path for this set of jobs using args and $PULL_REFS."""
    build_file = ''
    if use_shared_build:
        build_file += use_shared_build + '-'
    build_file += 'build-location.txt'
    return os.path.join(gcs_shared, os.getenv('PULL_REFS', ''), build_file)


def main(args):
    """Set up env, start kubekins-e2e, handle termination. """
    # pylint: disable=too-many-branches,too-many-statements,too-many-locals

    # Rules for env var priority here in docker:
    # -e FOO=a -e FOO=b -> FOO=b
    # --env-file FOO=a --env-file FOO=b -> FOO=b
    # -e FOO=a --env-file FOO=b -> FOO=a(!!!!)
    # --env-file FOO=a -e FOO=b -> FOO=b
    #
    # So if you overwrite FOO=c for a local run it will take precedence.
    #

    # Set up workspace/artifacts dir
    workspace = os.environ.get('WORKSPACE', os.getcwd())
    artifacts = os.environ.get('ARTIFACTS', os.path.join(workspace, '_artifacts'))
    if not os.path.isdir(artifacts):
        os.makedirs(artifacts)

    mode = LocalMode(workspace, artifacts)

    for env_file in args.env_file:
        mode.add_file(test_infra(env_file))
    for env in args.env:
        mode.add_env(env)

    # TODO(fejta): remove after next image push
    mode.add_environment('KUBETEST_MANUAL_DUMP=y')
    if args.dump_before_and_after:
        before_dir = os.path.join(mode.artifacts, 'before')
        if not os.path.exists(before_dir):
            os.makedirs(before_dir)
        after_dir = os.path.join(mode.artifacts, 'after')
        if not os.path.exists(after_dir):
            os.makedirs(after_dir)

        runner_args = [
            '--dump-pre-test-logs=%s' % before_dir,
            '--dump=%s' % after_dir,
            ]
    else:
        runner_args = [
            '--dump=%s' % mode.artifacts,
        ]

    if args.service_account:
        runner_args.append(
            '--gcp-service-account=%s' % mode.add_service_account(args.service_account))

    if args.use_shared_build is not None:
        # find shared build location from GCS
        gcs_path = get_shared_gcs_path(args.gcs_shared, args.use_shared_build)
        print('Getting shared build location from: '+gcs_path, file=sys.stderr)
        # retry loop for reading the location
        attempts_remaining = 12
        while True:
            attempts_remaining -= 1
            try:
                # tell kubetest to extract from this location
                shared_build_gcs_path = read_gcs_path(gcs_path)
                args.kubetest_args.append('--extract=' + shared_build_gcs_path)
                args.build = None
                break
            except urllib.error.URLError as err:
                print('Failed to get shared build location: %s' % err, file=sys.stderr)
                if attempts_remaining > 0:
                    print('Waiting 5 seconds and retrying...', file=sys.stderr)
                    time.sleep(5)
                else:
                    raise RuntimeError('Failed to get shared build location too many times!')

    elif args.build is not None:
        if args.build == '':
            # Empty string means --build was passed without any arguments;
            # if --build wasn't passed, args.build would be None
            runner_args.append('--build')
        else:
            runner_args.append('--build=%s' % args.build)
        k8s = os.getcwd()
        if not os.path.basename(k8s) == 'kubernetes':
            raise ValueError(k8s)
        mode.add_k8s(os.path.dirname(k8s), 'kubernetes', 'release')

    if args.stage is not None:
        runner_args.append('--stage=%s' % args.stage)

    # TODO(fejta): move these out of this file
    if args.up == 'true':
        runner_args.append('--up')
    if args.down == 'true':
        runner_args.append('--down')
    if args.test == 'true':
        runner_args.append('--test')

    # Passthrough some args to kubetest
    if args.deployment:
        runner_args.append('--deployment=%s' % args.deployment)
    if args.provider:
        runner_args.append('--provider=%s' % args.provider)

    cluster = cluster_name(args.cluster, args.tear_down_previous)
    runner_args.append('--cluster=%s' % cluster)
    runner_args.append('--gcp-network=%s' % cluster)
    runner_args.extend(args.kubetest_args)

    if args.use_logexporter:
        runner_args.append('--logexporter-gcs-path=%s' % args.logexporter_gcs_path)

    if args.deployment != 'kind' and args.gce_ssh:
        mode.add_gce_ssh(args.gce_ssh, args.gce_pub)

    # TODO(fejta): delete this?
    mode.add_os_environment(*(
        '%s=%s' % (k, v) for (k, v) in list(os.environ.items())))

    mode.add_environment(
      # Boilerplate envs
      # Skip gcloud update checking
      'CLOUDSDK_COMPONENT_MANAGER_DISABLE_UPDATE_CHECK=true',
      # Use default component update behavior
      'CLOUDSDK_EXPERIMENTAL_FAST_COMPONENT_UPDATE=false',
      # AWS
      'KUBE_AWS_INSTANCE_PREFIX=%s' % cluster,
      # GCE
      'INSTANCE_PREFIX=%s' % cluster,
      'KUBE_GCE_INSTANCE_PREFIX=%s' % cluster,
    )

    mode.start(runner_args)


def create_parser():
    """Create argparser."""
    parser = argparse.ArgumentParser()
    parser.add_argument(
        '--env-file', default=[], action="append",
        help='Job specific environment file')
    parser.add_argument(
        '--env', default=[], action="append",
        help='Job specific environment setting ' +
        '(usage: "--env=VAR=SETTING" will set VAR to SETTING).')
    parser.add_argument(
        '--gce-ssh',
        default=os.environ.get('JENKINS_GCE_SSH_PRIVATE_KEY_FILE'),
        help='Path to .ssh/google_compute_engine keys')
    parser.add_argument(
        '--gce-pub',
        default=os.environ.get('JENKINS_GCE_SSH_PUBLIC_KEY_FILE'),
        help='Path to pub gce ssh key')
    parser.add_argument(
        '--service-account',
        default=os.environ.get('GOOGLE_APPLICATION_CREDENTIALS'),
        help='Path to service-account.json')
    parser.add_argument(
        '--build', nargs='?', default=None, const='',
        help='Build kubernetes binaries if set, optionally specifying strategy')
    parser.add_argument(
        '--use-shared-build', nargs='?', default=None, const='',
        help='Use prebuilt kubernetes binaries if set, optionally specifying strategy')
    parser.add_argument(
        '--gcs-shared',
        default='gs://kubernetes-jenkins/shared-results/',
        help='Get shared build from this bucket')
    parser.add_argument(
        '--cluster', default='bootstrap-e2e', help='Name of the cluster')
    parser.add_argument(
        '--stage', default=None, help='Stage release to GCS path provided')
    parser.add_argument(
        '--test', default='true', help='If we need to run any actual test within kubetest')
    parser.add_argument(
        '--down', default='true', help='If we need to tear down the e2e cluster')
    parser.add_argument(
        '--up', default='true', help='If we need to bring up a e2e cluster')
    parser.add_argument(
        '--tear-down-previous', action='store_true',
        help='If we need to tear down previous e2e cluster')
    parser.add_argument(
        '--use-logexporter',
        action='store_true',
        help='If we need to use logexporter tool to upload logs from nodes to GCS directly')
    parser.add_argument(
        '--logexporter-gcs-path',
        default=os.environ.get('GCS_ARTIFACTS_DIR',''),
        help='GCS path where logexporter tool will upload logs if enabled')
    parser.add_argument(
        '--kubetest_args',
        action='append',
        default=[],
        help='Send unrecognized args directly to kubetest')
    parser.add_argument(
        '--dump-before-and-after', action='store_true',
        help='Dump artifacts from both before and after the test run')

    # kubetest flags that also trigger behaviour here
    parser.add_argument(
        '--provider', help='provider flag as used by kubetest')
    parser.add_argument(
        '--deployment', help='deployment flag as used by kubetest')

    return parser


def parse_args(args=None):
    """Return args, adding unrecognized args to kubetest_args."""
    parser = create_parser()
    args, extra = parser.parse_known_args(args)
    args.kubetest_args += extra

    return args


if __name__ == '__main__':
    main(parse_args())
