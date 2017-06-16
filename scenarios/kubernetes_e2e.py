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

def parse_env(env):
    """Returns (FOO, BAR=MORE) for FOO=BAR=MORE."""
    return env.split('=', 1)

def kubeadm_version(mode):
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

        # Work-around for release-1.6 jobs, which still upload debs to an older
        # location (without os/arch prefixes).
        # TODO(pipejakob): remove this when we no longer support 1.6.x.
        if version.startswith("v1.6."):
            return 'gs://kubernetes-release-dev/bazel/%s/build/debs/' % version

    elif mode == 'pull':
        version = '%s/%s' % (os.environ['PULL_NUMBER'], os.getenv('PULL_REFS'))

    else:
        raise ValueError("Unknown kubeadm mode given: %s" % mode)

    # The path given here should match jobs/ci-kubernetes-bazel-build.sh
    return 'gs://kubernetes-release-dev/bazel/%s/bin/linux/amd64/' % version


class LocalMode(object):
    """Runs e2e tests by calling e2e-runner.sh."""
    def __init__(self, workspace):
        self.workspace = workspace
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
          '/workspace/e2e-runner.sh',
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
        env.update(self.os_env)
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
        self._add_env_var('HOME=/workspace')
        self._add_env_var('WORKSPACE=/workspace')

    def add_environment(self, *envs):
        """Adds FOO=BAR to the -e list for docker.

        Host-specific environment variables are ignored."""
        # TODO(krzyzacy) change this to a whitelist?
        docker_env_ignore = [
            'GOOGLE_APPLICATION_CREDENTIALS',
            'GOPATH',
            'GOROOT',
            'HOME',
            'PATH',
            'PWD',
            'WORKSPACE'
        ]
        for env in envs:
            key, _value = parse_env(env)
            if key in docker_env_ignore:
                print >>sys.stderr, 'Skipping environment variable %s' % env
            else:
                self._add_env_var(env)

    def add_os_environment(self, *envs):
        """Adds os envs as FOO=BAR to the -e list for docker."""
        self.add_environment(*envs)

    def _add_env_var(self, env):
        """Adds a single environment variable to the -e list for docker.

        Does not check against any blacklists."""
        self.cmd.extend(['-e', env])

    def add_file(self, env_file):
        """Adds the file to the --env-file list."""
        self.cmd.extend(['--env-file', env_file])

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


def cluster_name(cluster, build):
    """Return or select a cluster name."""
    if cluster:
        return cluster
    if len(build) < 20:
        return 'e2e-%s' % build
    return 'e2e-%s' % hashlib.md5(build).hexdigest()[:10]


def main(args):
    """Set up env, start kubekins-e2e, handle termination. """
    # pylint: disable=too-many-branches,too-many-statements

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
        sudo = args.docker_in_docker or args.build is not None
        mode = DockerMode(container, workspace, sudo, args.tag, args.mount_paths)
    elif args.mode == 'local':
        mode = LocalMode(workspace)  # pylint: disable=bad-option-value
    else:
        raise ValueError(args.mode)

    if args.env_file:
        for env_file in args.env_file:
            mode.add_file(test_infra(env_file))

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
    if args.build is not None:
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
    if args.stage:
        runner_args.append('--stage=%s' % args.stage)
    if args.stage_suffix:
        runner_args.append('--stage-suffix=%s' % args.stage_suffix)
    if args.multiple_federations:
        runner_args.append('--multiple-federations')
    if args.perf_tests:
        runner_args.append('--perf-tests')
    if args.charts_tests:
        runner_args.append('--charts')
    if args.kubemark:
        runner_args.append('--kubemark')
    if args.up == 'true':
        runner_args.append('--up')
    if args.down == 'true':
        runner_args.append('--down')
    if args.federation:
        runner_args.append('--federation')
    if args.deployment:
        runner_args.append('--deployment=%s' % args.deployment)
    if args.save:
        runner_args.append('--save=%s' % args.save)
    if args.publish:
        runner_args.append('--publish=%s' % args.publish)
    if args.timeout:
        runner_args.append('--timeout=%s' % args.timeout)
    if args.skew:
        runner_args.append('--skew')
    if args.upgrade_args:
        runner_args.append('--upgrade_args=%s' % args.upgrade_args)

    for ext in args.extract or []:
        runner_args.append('--extract=%s' % ext)
    cluster = cluster_name(args.cluster, os.getenv('BUILD_NUMBER', 0))
    # TODO(fejta): remove this add_environment after pushing new kubetest image
    mode.add_environment('FAIL_ON_GCP_RESOURCE_LEAK=false')
    runner_args.append('--check-leaked-resources=%s' % args.check_leaked_resources)


    if args.kubeadm:
        version = kubeadm_version(args.kubeadm)
        runner_args.extend([
            ' --kubernetes-anywhere-path=/workspace/kubernetes-anywhere',
            ' --kubernetes-anywhere-phase2-provider=kubeadm',
            ' --kubernetes-anywhere-cluster=%s' % cluster,
            ' --kubernetes-anywhere-kubeadm-version=%s' % version,
        ])

    # TODO(fejta): delete this?
    mode.add_os_environment(*(
        '%s=%s' % (k, v) for (k, v) in os.environ.items()))

    mode.add_environment(
      # Boilerplate envs
      # Skip gcloud update checking
      'CLOUDSDK_COMPONENT_MANAGER_DISABLE_UPDATE_CHECK=true',
      # Use default component update behavior
      'CLOUDSDK_EXPERIMENTAL_FAST_COMPONENT_UPDATE=false',
      # E2E
      'E2E_TEST=%s' % args.test,
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
    parser.add_argument('--publish', help='Upload binaries to gs://path if set')
    parser.add_argument(
        '--build', nargs='?', default=None, const='',
        help='Build kubernetes binaries if set, optionally specifying strategy')
    parser.add_argument(
        '--stage', help='Stage binaries to gs:// path if set')
    parser.add_argument(
        '--stage-suffix', help='Append suffix to staged version if set')
    parser.add_argument(
        '--charts-tests', action='store_true', help='If the test is a charts test job')
    parser.add_argument(
        '--extract', action="append", help='Pass --extract flag(s) to kubetest')
    parser.add_argument(
        '--cluster', default='bootstrap-e2e', help='Name of the cluster')
    parser.add_argument(
        '--deployment', default='bash', choices=['none', 'bash', 'kops', 'kubernetes-anywhere'])
    parser.add_argument(
        '--docker-in-docker', action='store_true', help='Enable run docker within docker')
    parser.add_argument(
        '--down', default='true', help='If we need to tear down the e2e cluster')
    parser.add_argument(
        '--federation', action='store_true', help='If kubetest will have --federation flag')
    parser.add_argument(
        '--kubeadm', choices=['ci', 'periodic', 'pull'])
    parser.add_argument(
        '--kubemark', action='store_true', help='If the test uses kubemark')
    parser.add_argument(
        '--perf-tests', action='store_true', help='If the test need to run k8s/perf-test e2e test')
    parser.add_argument(
        '--save', default=None,
        help='Save credentials to gs:// path on --up if set (or load from there if not --up)')
    parser.add_argument(
        '--skew', action='store_true',
        help='If we need to run skew tests, pass --skew to kubetest.')
    parser.add_argument(
        '--tag', default='v20170605-ed5d94ed', help='Use a specific kubekins-e2e tag if set')
    parser.add_argument(
        '--test', default='true', help='If we need to run any actual test within kubetest')
    parser.add_argument(
        '--up', default='true', help='If we need to bring up a e2e cluster')
    parser.add_argument(
        '--timeout', help='Terminate testing after this golang duration (eg --timeout=100m).')
    parser.add_argument(
        '--multiple-federations', action='store_true',
        help='Run federation control planes in parallel')
    parser.add_argument(
        '--check-leaked-resources',
        nargs='?', default='false', const='true',
        help='Send --check-leaked-resources to kubetest')
    parser.add_argument(
        '--upgrade_args', help='Send --upgrade_args to kubetest')
    # TODO(fejta): allow sending arbitrary args to kubetest, remove flags that
    # otherwise do nothing aside from pass value to kubetest
    return parser

if __name__ == '__main__':
    main(create_parser().parse_args())
