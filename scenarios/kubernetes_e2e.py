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
import os
import re
import shutil
import signal
import subprocess
import sys

ORIG_CWD = os.getcwd()  # Checkout changes cwd

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
    for key, value in env.items():
        print >>sys.stderr, '%s=%s' % (key, value)
    print >>sys.stderr, 'Run:', cmd
    subprocess.check_call(cmd, env=env)


def kubekins(tag):
    """Return full path to kubekins-e2e:tag."""
    return 'gcr.io/k8s-testimages/kubekins-e2e:%s' % tag


class LocalMode(object):
    """Runs e2e tests by calling e2e-runner.sh."""
    def __init__(self, workspace):
        self.workspace = workspace
        self.env = []
        self.env_files = []
        self.add_environment(
            'HOME=%s' % workspace,
            'WORKSPACE=%s' % workspace,
            'PATH=%s' % os.getenv('PATH'),
        )

    @staticmethod
    def parse_env(env):
        """Returns (FOO, BAR=MORE) for FOO=BAR=MORE."""
        return env.split('=', 1)

    def add_environment(self, *envs):
        """Adds FOO=BAR to the list of environment overrides."""
        self.env.extend(self.parse_env(e) for e in envs)

    def add_files(self, env_files):
        """Reads all FOO=BAR lines from each path in env_files seq."""
        for env_file in env_files:
            with open(test_infra(env_file)) as fp:
                for line in fp:
                    line = line.rstrip()
                    if not line or line.startswith('#'):
                        continue
                    self.env_files.append(self.parse_env(line))

    def add_aws_cred(self, priv, pub, cred):
        """Sets aws keys and credentials."""
        self.add_environment('JENKINS_AWS_SSH_PRIVATE_KEY_FILE=%s' % priv)
        self.add_environment('JENKINS_AWS_SSH_PUBLIC_KEY_FILE=%s' % pub)
        self.add_environment('JENKINS_AWS_CREDENTIALS_FILE=%s' % cred)

    def add_gce_ssh(self, priv, pub):
        """Copies priv, pub keys to $WORKSPACE/.ssh."""
        ssh_dir = '%s/.ssh' % self.workspace
        if not os.path.isdir(ssh_dir):
            os.makedirs(ssh_dir)

        gce_ssh = '%s/google_compute_engine' % ssh_dir
        gce_pub = '%s/google_compute_engine.pub' % ssh_dir
        shutil.copy(priv, gce_ssh)
        shutil.copy(pub, gce_pub)
        self.add_environment(
            'JENKINS_GCE_SSH_PRIVATE_KEY_FILE=%s' % gce_ssh,
            'JENKINS_GCE_SSH_PUBLIC_KEY_FILE=%s' % gce_pub,
        )

    def add_service_account(self, path):
        """Sets GOOGLE_APPLICATION_CREDENTIALS to path."""
        self.add_environment('GOOGLE_APPLICATION_CREDENTIALS=%s' % path)

    @property
    def runner(self):
        """Finds the best version of e2e-runner.sh."""
        options = [
          os.path.join(self.workspace, 'e2e-runner.sh'),
          test_infra('jenkins/e2e-image/e2e-runner.sh')
        ]
        for path in options:
            if os.path.isfile(path):
                return path
        raise ValueError('Cannot find e2e-runner at any of %s' % ', '.join(options))


    def install_prerequisites(self):
        """Copies kubetest if needed."""
        parent = os.path.dirname(self.runner)
        if not os.path.isfile(os.path.join(parent, 'kubetest')):
            print >>sys.stderr, 'Cannot find kubetest in %s, will install from test-infra' % parent
            check('go', 'install', 'k8s.io/test-infra/kubetest')
            shutil.copy(
                os.path.expandvars('${GOPATH}/bin/kubetest'),
                os.path.join(parent, 'kubetest'))

    def add_k8s(self, *a, **kw):
        """Add specified k8s.io repos (noop)."""
        pass

    def start(self, args):
        """Runs e2e-runner.sh after setting env and installing prereqs."""
        print >>sys.stderr, 'starts with local mode'
        env = {}
        env.update(self.env_files)
        env.update(self.env)
        self.install_prerequisites()
        # Do not interfere with the local project
        project = env.get('PROJECT')
        if project:
            try:
                check('gcloud', 'config', 'set', 'project', env['PROJECT'])
            except subprocess.CalledProcessError:
                print >>sys.stderr, 'Fail to set project %r', project
        else:
            print >>sys.stderr, 'PROJECT not set in job, will use local project'
        check_env(env, self.runner, *args)


class DockerMode(object):
    """Runs e2e tests via docker run kubekins-e2e."""
    def __init__(self, container, workspace, sudo, tag, mount_paths):
        self.tag = tag
        try:  # Pull a newer version if one exists
            check('docker', 'pull', kubekins(tag))
        except subprocess.CalledProcessError:
            pass

        print 'Starting %s...' % container

        self.container = container
        self.cmd = [
            'docker', 'run', '--rm',
            '--name=%s' % container,
            '-v', '%s/_artifacts:/workspace/_artifacts' % workspace,
            '-v', '/etc/localtime:/etc/localtime:ro',
        ]
        for path in mount_paths:
            self.cmd.extend(['-v', path])

        if sudo:
            self.cmd.extend(['-v', '/var/run/docker.sock:/var/run/docker.sock'])
        self.add_environment(
            'HOME=/workspace',
            'WORKSPACE=/workspace')

    def add_environment(self, *envs):
        """Adds FOO=BAR to the -e list for docker."""
        for env in envs:
            self.cmd.extend(['-e', env])

    def add_files(self, env_files):
        """Adds each file to the --env-file list."""
        for env_file in env_files:
            self.cmd.extend(['--env-file', test_infra(env_file)])

    def add_k8s(self, k8s, *repos):
        """Add the specified k8s.io repos into container."""
        for repo in repos:
            self.cmd.extend([
                '-v', '%s/%s:/go/src/k8s.io/%s' % (k8s, repo, repo)])
        self.cmd.extend(['-v', '%s/release:/go/src/k8s.io/release' % k8s])

    def add_aws_cred(self, priv, pub, cred):
        """Mounts aws keys/creds inside the container."""
        aws_ssh = '/workspace/.ssh/kube_aws_rsa'
        aws_pub = '%s.pub' % aws_ssh
        aws_cred = '/workspace/.aws/credentials'

        self.cmd.extend([
          '-v', '%s:%s:ro' % (priv, aws_ssh),
          '-v', '%s:%s:ro' % (pub, aws_pub),
          '-v', '%s:%s:ro' % (cred, aws_cred),
        ])

    def add_gce_ssh(self, priv, pub):
        """Mounts priv and pub inside the container."""
        gce_ssh = '/workspace/.ssh/google_compute_engine'
        gce_pub = '%s.pub' % gce_ssh
        self.cmd.extend([
          '-v', '%s:%s:ro' % (priv, gce_ssh),
          '-v', '%s:%s:ro' % (pub, gce_pub),
          '-e', 'JENKINS_GCE_SSH_PRIVATE_KEY_FILE=%s' % gce_ssh,
          '-e', 'JENKINS_GCE_SSH_PUBLIC_KEY_FILE=%s' % gce_pub])

    def add_service_account(self, path):
        """Mounts GOOGLE_APPLICATION_CREDENTIALS inside the container."""
        service = '/service-account.json'
        self.cmd.extend([
            '-v', '%s:%s:ro' % (path, service),
            '-e', 'GOOGLE_APPLICATION_CREDENTIALS=%s' % service])


    def start(self, args):
        """Runs kubekins."""
        print >>sys.stderr, 'starts with docker mode'
        cmd = list(self.cmd)
        cmd.append(kubekins(self.tag))
        cmd.extend(args)
        signal.signal(signal.SIGTERM, self.sig_handler)
        signal.signal(signal.SIGINT, self.sig_handler)
        check(*cmd)

    def sig_handler(self, _signo, _frame):
        """Stops container upon receive signal.SIGTERM and signal.SIGINT."""
        print >>sys.stderr, 'docker stop (signo=%s, frame=%s)' % (_signo, _frame)
        check('docker', 'stop', self.container)


def main(args):
    """Set up env, start kubekins-e2e, handle termination. """
    # pylint: disable=too-many-branches

    # Rules for env var priority here in docker:
    # -e FOO=a -e FOO=b -> FOO=b
    # --env-file FOO=a --env-file FOO=b -> FOO=b
    # -e FOO=a --env-file FOO=b -> FOO=a(!!!!)
    # --env-file FOO=a -e FOO=b -> FOO=b
    #
    # So if you overwrite FOO=c for a local run it will take precedence.
    #

    # dockerized-e2e-runner goodies setup
    workspace = os.environ.get('WORKSPACE', os.getcwd())
    artifacts = '%s/_artifacts' % workspace
    if not os.path.isdir(artifacts):
        os.makedirs(artifacts)

    container = '%s-%s' % (os.environ.get('JOB_NAME'), os.environ.get('BUILD_NUMBER'))
    if args.mode == 'docker':
        sudo = args.docker_in_docker or args.build
        mode = DockerMode(container, workspace, sudo, args.tag, args.mount_paths)
    elif args.mode == 'local':
        mode = LocalMode(workspace)  # pylint: disable=redefined-variable-type
    else:
        raise ValueError(args.mode)
    if args.env_file:
        mode.add_files(args.env_file)

    if args.aws:
        # Enforce aws credential/keys exists
        for path in [args.aws_ssh, args.aws_pub, args.aws_cred]:
            if not os.path.isfile(os.path.expandvars(path)):
                raise IOError(path, os.path.expandvars(path))
        mode.add_aws_cred(args.aws_ssh, args.aws_pub, args.aws_cred)

    if args.gce_ssh:
        mode.add_gce_ssh(args.gce_ssh, args.gce_pub)

    if args.service_account:
        mode.add_service_account(args.service_account)

    runner_args = []
    if args.build:
        runner_args.append('--build')
        k8s = os.getcwd()
        if not os.path.basename(k8s) == 'kubernetes':
            raise ValueError(k8s)
        mode.add_k8s(os.path.dirname(k8s), 'kubernetes', 'release')
    if args.stage:
        runner_args.append('--stage=%s' % args.stage)

    cluster = args.cluster or 'e2e-gce-%s-%s' % (
        os.environ['NODE_NAME'], os.getenv('EXECUTOR_NUMBER', 0))

    if args.kubeadm:
        # Not from Jenkins
        cluster = args.cluster or 'e2e-kubeadm-%s' % os.getenv('BUILD_NUMBER', 0)

        # This job only runs against the kubernetes repo, and bootstrap.py leaves the
        # current working directory at the repository root. Grab the SCM_REVISION so we
        # can use the .debs built during the bazel-build job that should have already
        # succeeded.
        status = re.match(
            r'STABLE_BUILD_SCM_REVISION [^\n]+',
            check_output('hack/print-workspace-status.sh')
        )
        if not status:
            raise ValueError('STABLE_BUILD_SCM_REVISION not found')

        opt = '--deployment kubernetes-anywhere ' \
            '--kubernetes-anywhere-path /workspace/kubernetes-anywhere' \
            '--kubernetes-anywhere-phase2-provider kubeadm ' \
            '--kubernetes-anywhere-cluster %s' \
            '--kubernetes-anywhere-kubeadm-version ' \
            'gs://kubernetes-release-dev/bazel/%s/build/debs/' % (cluster, status.group(1))
            # The gs:// path given here should match jobs/ci-kubernetes-bazel-build.sh
        mode.add_environment('E2E_OPT=%s' % opt)

    mode.add_environment(
      # Boilerplate envs
      # Skip gcloud update checking
      'CLOUDSDK_COMPONENT_MANAGER_DISABLE_UPDATE_CHECK=true',
      # Use default component update behavior
      'CLOUDSDK_EXPERIMENTAL_FAST_COMPONENT_UPDATE=false',
      # E2E
      'E2E_UP=%s' % args.up,
      'E2E_TEST=%s' % args.test,
      'E2E_DOWN=%s' % args.down,
      'E2E_NAME=%s' % cluster,
      # AWS
      'KUBE_AWS_INSTANCE_PREFIX=%s' % cluster,
      # GCE
      'INSTANCE_PREFIX=%s' % cluster,
      'KUBE_GCE_NETWORK=%s' % cluster,
      'KUBE_GCE_INSTANCE_PREFIX=%s' % cluster,
      # GKE
      'CLUSTER_NAME=%s' % cluster,
      'KUBE_GKE_NETWORK=%s' % cluster,
    )

    # env blacklist.
    # TODO(krzyzacy) change this to a whitelist
    docker_env_ignore = [
      'GOOGLE_APPLICATION_CREDENTIALS',
      'GOPATH',
      'GOROOT',
      'HOME',
      'PATH',
      'PWD',
      'WORKSPACE'
    ]

    # TODO(fejta): delete this
    mode.add_environment(*(
        '%s=%s' % (k, v) for (k, v) in os.environ.items()
        if k not in docker_env_ignore))

    # Overwrite JOB_NAME for soak-*-test jobs
    if args.soak_test and os.environ.get('JOB_NAME'):
        mode.add_environment('JOB_NAME=%s' % os.environ.get('JOB_NAME').replace('-test', '-deploy'))

    mode.start(runner_args)

def create_parser():
    """Create argparser."""
    parser = argparse.ArgumentParser()
    parser.add_argument(
        '--mode', default='docker', choices=['local', 'docker'])
    parser.add_argument(
        '--env-file', action="append", help='Job specific environment file')

    parser.add_argument(
        '--aws', action='store_true', help='E2E job runs in aws')
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
        '--mount-paths',
        default=[],
        nargs='*',
        help='Paths that should be mounted within the docker container in the form local:remote')
    # Assume we're upping, testing, and downing a cluster by default
    parser.add_argument(
        '--build', action='store_true', help='Build kubernetes binaries if set')
    parser.add_argument(
        '--stage', help='Stage binaries to gs:// path if set')
    parser.add_argument(
        '--cluster', default='bootstrap-e2e', help='Name of the cluster')
    parser.add_argument(
        '--docker-in-docker', action='store_true', help='Enable run docker within docker')
    parser.add_argument(
        '--down', default='true', help='If we need to set --down in e2e.go')
    parser.add_argument(
        '--kubeadm', action='store_true', help='If the test is a kubeadm job')
    parser.add_argument(
        '--soak-test', action='store_true', help='If the test is a soak test job')
    parser.add_argument(
        '--tag', default='v20170314-bb0669b0', help='Use a specific kubekins-e2e tag if set')
    parser.add_argument(
        '--test', default='true', help='If we need to set --test in e2e.go')
    parser.add_argument(
        '--up', default='true', help='If we need to set --up in e2e.go')
    return parser

if __name__ == '__main__':
    main(create_parser().parse_args())
