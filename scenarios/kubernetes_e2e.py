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
# pylint: disable=bad-continuation

"""Runs kubernetes e2e test with specified config"""

import argparse
import hashlib
import os
import random
import re
import shutil
import subprocess
import sys
import urllib2
import time

ORIG_CWD = os.getcwd()  # Checkout changes cwd

# The zones below are the zones available in the CNCF account (in theory, zones vary by account)
# We aim for 3 zones per region to try to maintain even spreading.
# We also remove a few zones where our preferred instance type is not available,
# though really this needs a better fix (likely in kops)
DEFAULT_AWS_ZONES = [
    'ap-northeast-1a',
    'ap-northeast-1c',
    'ap-northeast-1d',
    'ap-northeast-2a',
    #'ap-northeast-2b' - AZ does not exist, so we're breaking the 3 AZs per region target here
    'ap-northeast-2c',
    'ap-south-1a',
    'ap-south-1b',
    'ap-southeast-1a',
    'ap-southeast-1b',
    'ap-southeast-1c',
    'ap-southeast-2a',
    'ap-southeast-2b',
    'ap-southeast-2c',
    'ca-central-1a',
    'ca-central-1b',
    'eu-central-1a',
    'eu-central-1b',
    'eu-central-1c',
    'eu-west-1a',
    'eu-west-1b',
    'eu-west-1c',
    'eu-west-2a',
    'eu-west-2b',
    'eu-west-2c',
    #'eu-west-3a', documented to not support c4 family
    #'eu-west-3b', documented to not support c4 family
    #'eu-west-3c', documented to not support c4 family
    'sa-east-1a',
    #'sa-east-1b', AZ does not exist, so we're breaking the 3 AZs per region target here
    'sa-east-1c',
    #'us-east-1a', # temporarily removing due to lack of quota #10043
    #'us-east-1b', # temporarily removing due to lack of quota #10043
    #'us-east-1c', # temporarily removing due to lack of quota #10043
    #'us-east-1d', # limiting to 3 zones to not overallocate
    #'us-east-1e', # limiting to 3 zones to not overallocate
    #'us-east-1f', # limiting to 3 zones to not overallocate
    #'us-east-2a', InsufficientInstanceCapacity for c4.large 2018-05-30
    #'us-east-2b', InsufficientInstanceCapacity for c4.large 2018-05-30
    #'us-east-2c', InsufficientInstanceCapacity for c4.large 2018-05-30
    'us-west-1a',
    'us-west-1b',
    #'us-west-1c', AZ does not exist, so we're breaking the 3 AZs per region target here
    #'us-west-2a', # temporarily removing due to lack of quota #10043
    #'us-west-2b', # temporarily removing due to lack of quota #10043
    #'us-west-2c', # temporarily removing due to lack of quota #10043
]

def test_infra(*paths):
    """Return path relative to root of test-infra repo."""
    return os.path.join(ORIG_CWD, os.path.dirname(__file__), '..', *paths)


def check(*cmd):
    """Log and run the command, raising on errors."""
    print >>sys.stderr, 'Run:', cmd
    subprocess.check_call(cmd)


def check_output(*cmd):
    """Log and run the command, raising on errors, return output"""
    print >>sys.stderr, 'Run:', cmd
    return subprocess.check_output(cmd)


def check_env(env, *cmd):
    """Log and run the command with a specific env, raising on errors."""
    print >>sys.stderr, 'Environment:'
    for key, value in sorted(env.items()):
        print >>sys.stderr, '%s=%s' % (key, value)
    print >>sys.stderr, 'Run:', cmd
    subprocess.check_call(cmd, env=env)


def kubekins(tag):
    """Return full path to kubekins-e2e:tag."""
    return 'gcr.io/k8s-testimages/kubekins-e2e:%s' % tag

def parse_env(env):
    """Returns (FOO, BAR=MORE) for FOO=BAR=MORE."""
    return env.split('=', 1)

def aws_role_config(profile, arn):
    return (('[profile jenkins-assumed-role]\n' +
             'role_arn = %s\n' +
             'source_profile = %s\n') %
            (arn, profile))

def kubeadm_version(mode, shared_build_gcs_path):
    """Return string to use for kubeadm version, given the job's mode (ci/pull/periodic)."""
    version = ''
    if mode in ['ci', 'periodic']:
        # This job only runs against the kubernetes repo, and bootstrap.py leaves the
        # current working directory at the repository root. Grab the SCM_REVISION so we
        # can use the .debs built during the bazel-build job that should have already
        # succeeded.
        status = re.search(
            r'STABLE_BUILD_SCM_REVISION ([^\n]+)',
            check_output('hack/print-workspace-status.sh')
        )
        if not status:
            raise ValueError('STABLE_BUILD_SCM_REVISION not found')
        version = status.group(1)

        # The path given here should match ci-kubernetes-bazel-build
        return 'gs://kubernetes-release-dev/ci/%s-bazel/bin/linux/amd64/' % version

    elif mode == 'pull':
        # The format of shared_build_gcs_path looks like:
        # gs://kubernetes-release-dev/bazel/<git-describe-output>
        # Add bin/linux/amd64 yet to that path so it points to the dir with the debs
        return '%s/bin/linux/amd64/' % shared_build_gcs_path

    elif mode == 'stable':
        # This job need not run against the kubernetes repo and uses the stable version
        # of kubeadm packages. This mode may be desired when kubeadm itself is not the
        # SUT (System Under Test).
        return 'stable'

    else:
        raise ValueError("Unknown kubeadm mode given: %s" % mode)

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

    def add_aws_cred(self, priv, pub, cred):
        """Sets aws keys and credentials."""
        ssh_dir = os.path.join(self.workspace, '.ssh')
        if not os.path.isdir(ssh_dir):
            os.makedirs(ssh_dir)

        cred_dir = os.path.join(self.workspace, '.aws')
        if not os.path.isdir(cred_dir):
            os.makedirs(cred_dir)

        aws_ssh = os.path.join(ssh_dir, 'kube_aws_rsa')
        aws_pub = os.path.join(ssh_dir, 'kube_aws_rsa.pub')
        aws_cred = os.path.join(cred_dir, 'credentials')
        shutil.copy(priv, aws_ssh)
        shutil.copy(pub, aws_pub)
        shutil.copy(cred, aws_cred)

        self.add_environment(
            'JENKINS_AWS_SSH_PRIVATE_KEY_FILE=%s' % priv,
            'JENKINS_AWS_SSH_PUBLIC_KEY_FILE=%s' % pub,
            'JENKINS_AWS_CREDENTIALS_FILE=%s' % cred,
        )

    def add_aws_role(self, profile, arn):
        with open(os.path.join(self.workspace, '.aws', 'config'), 'w') as cfg:
            cfg.write(aws_role_config(profile, arn))
        self.add_environment('AWS_SDK_LOAD_CONFIG=true')
        return 'jenkins-assumed-role'

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

    def add_aws_runner(self):
        """Start with kops-e2e-runner.sh"""
        # TODO(Krzyzacy):retire kops-e2e-runner.sh
        self.command = os.path.join(self.workspace, 'kops-e2e-runner.sh')

    def start(self, args):
        """Starts kubetest."""
        print >>sys.stderr, 'starts with local mode'
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
        suffix = hashlib.md5(suffix).hexdigest()[:10]
    job_hash = hashlib.md5(os.getenv('JOB_NAME', '')).hexdigest()[:5]
    return 'e2e-%s-%s' % (suffix, job_hash)


# TODO(krzyzacy): Move this into kubetest
def build_kops(kops, mode):
    """Build kops, set kops related envs."""
    if not os.path.basename(kops) == 'kops':
        raise ValueError(kops)
    version = 'pull-' + check_output('git', 'describe', '--always').strip()
    job = os.getenv('JOB_NAME', 'pull-kops-e2e-kubernetes-aws')
    gcs = 'gs://kops-ci/pulls/%s' % job
    gapi = 'https://storage.googleapis.com/kops-ci/pulls/%s' % job
    mode.add_environment(
        'KOPS_BASE_URL=%s/%s' % (gapi, version),
        'GCS_LOCATION=%s' % gcs
        )
    check('make', 'gcs-publish-ci', 'VERSION=%s' % version, 'GCS_LOCATION=%s' % gcs)


def set_up_kops_gce(workspace, args, mode, cluster, runner_args):
    """Set up kops on GCE envs."""
    for path in [args.gce_ssh, args.gce_pub]:
        if not os.path.isfile(os.path.expandvars(path)):
            raise IOError(path, os.path.expandvars(path))
    mode.add_gce_ssh(args.gce_ssh, args.gce_pub)

    gce_ssh = os.path.join(workspace, '.ssh', 'google_compute_engine')

    zones = args.kops_zones or random.choice([
        'us-central1-a',
        'us-central1-b',
        'us-central1-c',
        'us-central1-f',
    ])

    runner_args.extend([
        '--kops-cluster=%s' % cluster,
        '--kops-zones=%s' % zones,
        '--kops-state=%s' % args.kops_state_gce,
        '--kops-nodes=%s' % args.kops_nodes,
        '--kops-ssh-key=%s' % gce_ssh,
    ])


def set_up_kops_aws(workspace, args, mode, cluster, runner_args):
    """Set up aws related envs for kops.  Will replace set_up_aws."""
    for path in [args.aws_ssh, args.aws_pub, args.aws_cred]:
        if not os.path.isfile(os.path.expandvars(path)):
            raise IOError(path, os.path.expandvars(path))
    mode.add_aws_cred(args.aws_ssh, args.aws_pub, args.aws_cred)

    aws_ssh = os.path.join(workspace, '.ssh', 'kube_aws_rsa')
    profile = args.aws_profile
    if args.aws_role_arn:
        profile = mode.add_aws_role(profile, args.aws_role_arn)

    # kubetest for kops now support select random regions and zones.
    # For initial testing we are not sending in zones when the
    # --kops-multiple-zones flag is set.  If the flag is not set then
    # we use the older functionality of passing in zones.
    if args.kops_multiple_zones:
        runner_args.extend(["--kops-multiple-zones"])
    else:
        # TODO(@chrislovecnm): once we have tested we can remove the zones
        # and region logic from this code and have kubetest handle that
        # logic
        zones = args.kops_zones or random.choice(DEFAULT_AWS_ZONES)
        regions = ','.join([zone[:-1] for zone in zones.split(',')])
        runner_args.extend(['--kops-zones=%s' % zones])
        mode.add_environment(
          'KOPS_REGIONS=%s' % regions,
        )

    mode.add_environment(
      'AWS_PROFILE=%s' % profile,
      'AWS_DEFAULT_PROFILE=%s' % profile,
    )

    if args.aws_cluster_domain:
        cluster = '%s.%s' % (cluster, args.aws_cluster_domain)

    # AWS requires a username (and it varies per-image)
    ssh_user = args.kops_ssh_user or 'admin'

    runner_args.extend([
        '--kops-cluster=%s' % cluster,
        '--kops-state=%s' % args.kops_state,
        '--kops-nodes=%s' % args.kops_nodes,
        '--kops-ssh-key=%s' % aws_ssh,
        '--kops-ssh-user=%s' % ssh_user,
    ])


def set_up_aws(workspace, args, mode, cluster, runner_args):
    """Set up aws related envs.  Legacy; will be replaced by set_up_kops_aws."""
    for path in [args.aws_ssh, args.aws_pub, args.aws_cred]:
        if not os.path.isfile(os.path.expandvars(path)):
            raise IOError(path, os.path.expandvars(path))
    mode.add_aws_cred(args.aws_ssh, args.aws_pub, args.aws_cred)

    aws_ssh = os.path.join(workspace, '.ssh', 'kube_aws_rsa')
    profile = args.aws_profile
    if args.aws_role_arn:
        profile = mode.add_aws_role(profile, args.aws_role_arn)

    zones = args.kops_zones or random.choice(DEFAULT_AWS_ZONES)
    regions = ','.join([zone[:-1] for zone in zones.split(',')])

    mode.add_environment(
      'AWS_PROFILE=%s' % profile,
      'AWS_DEFAULT_PROFILE=%s' % profile,
      'KOPS_REGIONS=%s' % regions,
    )

    if args.aws_cluster_domain:
        cluster = '%s.%s' % (cluster, args.aws_cluster_domain)

    # AWS requires a username (and it varies per-image)
    ssh_user = args.kops_ssh_user or 'admin'

    runner_args.extend([
        '--kops-cluster=%s' % cluster,
        '--kops-zones=%s' % zones,
        '--kops-state=%s' % args.kops_state,
        '--kops-nodes=%s' % args.kops_nodes,
        '--kops-ssh-key=%s' % aws_ssh,
        '--kops-ssh-user=%s' % ssh_user,
    ])
    # TODO(krzyzacy):Remove after retire kops-e2e-runner.sh
    mode.add_aws_runner()

def read_gcs_path(gcs_path):
    """reads a gcs path (gs://...) by HTTP GET to storage.googleapis.com"""
    link = gcs_path.replace('gs://', 'https://storage.googleapis.com/')
    loc = urllib2.urlopen(link).read()
    print >>sys.stderr, "Read GCS Path: %s" % loc
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

    shared_build_gcs_path = ""
    if args.use_shared_build is not None:
        # find shared build location from GCS
        gcs_path = get_shared_gcs_path(args.gcs_shared, args.use_shared_build)
        print >>sys.stderr, 'Getting shared build location from: '+gcs_path
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
            except urllib2.URLError as err:
                print >>sys.stderr, 'Failed to get shared build location: %s' % err
                if attempts_remaining > 0:
                    print >>sys.stderr, 'Waiting 5 seconds and retrying...'
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

    if args.build_federation is not None:
        if args.build_federation == '':
            runner_args.append('--build-federation')
        else:
            runner_args.append('--build-federation=%s' % args.build_federation)
        fed = os.getcwd()
        if not os.path.basename(fed) == 'federation':
            raise ValueError(fed)
        mode.add_k8s(os.path.dirname(fed), 'federation', 'release')

    if args.kops_build:
        build_kops(os.getcwd(), mode)

    if args.stage is not None:
        runner_args.append('--stage=%s' % args.stage)
        if args.aws:
            for line in check_output('hack/print-workspace-status.sh').split('\n'):
                if 'gitVersion' in line:
                    _, version = line.strip().split(' ')
                    break
            else:
                raise ValueError('kubernetes version not found in workspace status')
            runner_args.append('--kops-kubernetes-version=%s/%s' % (
                args.stage.replace('gs://', 'https://storage.googleapis.com/'),
                version))

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
        # TODO(fejta): Take the below value through a flag instead of env var.
        runner_args.append('--logexporter-gcs-path=%s' % os.environ.get('GCS_ARTIFACTS_DIR', ''))

    if args.kubeadm:
        version = kubeadm_version(args.kubeadm, shared_build_gcs_path)
        runner_args.extend([
            '--kubernetes-anywhere-path=%s' % os.path.join(workspace, 'k8s.io',
                'kubernetes-anywhere'),
            '--kubernetes-anywhere-phase2-provider=kubeadm',
            '--kubernetes-anywhere-cluster=%s' % cluster,
            '--kubernetes-anywhere-kubeadm-version=%s' % version,
        ])

        if args.kubeadm == "pull":
            # If this is a pull job; the kubelet version should equal
            # the kubeadm version here: we should use debs from the PR build
            runner_args.extend([
                '--kubernetes-anywhere-kubelet-version=%s' % version,
            ])

    if args.aws:
        # Legacy - prefer passing --deployment=kops, --provider=aws,
        # which does not use kops-e2e-runner.sh
        set_up_aws(mode.workspace, args, mode, cluster, runner_args)
    elif args.deployment == 'kops' and args.provider == 'aws':
        set_up_kops_aws(mode.workspace, args, mode, cluster, runner_args)
    elif args.deployment == 'kops' and args.provider == 'gce':
        set_up_kops_gce(mode.workspace, args, mode, cluster, runner_args)
    elif args.gce_ssh:
        mode.add_gce_ssh(args.gce_ssh, args.gce_pub)

    # TODO(fejta): delete this?
    mode.add_os_environment(*(
        '%s=%s' % (k, v) for (k, v) in os.environ.items()))

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
        '--build-federation', nargs='?', default=None, const='',
        help='Build federation binaries if set, optionally specifying strategy')
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
        '--kubeadm', choices=['ci', 'periodic', 'pull', 'stable'])
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
        '--kubetest_args',
        action='append',
        default=[],
        help='Send unrecognized args directly to kubetest')
    parser.add_argument(
        '--dump-before-and-after', action='store_true',
        help='Dump artifacts from both before and after the test run')


    # kops & aws
    # TODO(justinsb): replace with --provider=aws --deployment=kops
    parser.add_argument(
        '--aws', action='store_true', help='E2E job runs in aws')
    parser.add_argument(
        '--aws-profile',
        default=(
            os.environ.get('AWS_PROFILE') or
            os.environ.get('AWS_DEFAULT_PROFILE') or
            'default'
        ),
        help='Profile within --aws-cred to use')
    parser.add_argument(
        '--aws-role-arn',
        default=os.environ.get('KOPS_E2E_ROLE_ARN'),
        help='Use --aws-profile to run as --aws-role-arn if set')
    parser.add_argument(
        '--aws-ssh',
        default=os.environ.get('JENKINS_AWS_SSH_PRIVATE_KEY_FILE'),
        help='Path to private aws ssh keys')
    parser.add_argument(
        '--aws-pub',
        default=os.environ.get('JENKINS_AWS_SSH_PUBLIC_KEY_FILE'),
        help='Path to pub aws ssh key')
    parser.add_argument(
        '--aws-cred',
        default=os.environ.get('JENKINS_AWS_CREDENTIALS_FILE'),
        help='Path to aws credential file')
    parser.add_argument(
        '--aws-cluster-domain', help='Domain of the aws cluster for aws-pr jobs')
    parser.add_argument(
        '--kops-nodes', default=4, type=int, help='Number of nodes to start')
    parser.add_argument(
        '--kops-ssh-user', default='',
        help='Username for ssh connections to instances')
    parser.add_argument(
        '--kops-state', default='s3://k8s-kops-prow/',
        help='Name of the aws state storage')
    parser.add_argument(
        '--kops-state-gce', default='gs://k8s-kops-gce/',
        help='Name of the kops state storage for GCE')
    parser.add_argument(
        '--kops-zones', help='Comma-separated list of zones else random choice')
    parser.add_argument(
        '--kops-build', action='store_true', help='If we need to build kops locally')
    parser.add_argument(
        '--kops-multiple-zones', action='store_true', help='Use multiple zones')


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

    if args.aws or args.provider == 'aws':
        # If aws keys are missing, try to fetch from HOME dir
        if not args.aws_ssh or not args.aws_pub or not args.aws_cred:
            home = os.environ.get('HOME')
            if not home:
                raise ValueError('HOME dir not set!')
            if not args.aws_ssh:
                args.aws_ssh = '%s/.ssh/kube_aws_rsa' % home
                print >>sys.stderr, '-aws-ssh key not set. Defaulting to %s' % args.aws_ssh
            if not args.aws_pub:
                args.aws_pub = '%s/.ssh/kube_aws_rsa.pub' % home
                print >>sys.stderr, '--aws-pub key not set. Defaulting to %s' % args.aws_pub
            if not args.aws_cred:
                args.aws_cred = '%s/.aws/credentials' % home
                print >>sys.stderr, '--aws-cred not set. Defaulting to %s' % args.aws_cred
    return args


if __name__ == '__main__':
    main(parse_args())
