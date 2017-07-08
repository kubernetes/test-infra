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
import traceback

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
    """Runs e2e tests by calling kubetest."""
    def __init__(self, workspace, artifacts):
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

    def add_k8s(self, *a, **kw):
        """Add specified k8s.io repos (noop)."""
        pass

    def use_latest_image(self, image_family, image_project):
        """Gets the latest image from the image_family in the image_project."""
        out = check_output(
            'gcloud', 'compute', 'images', 'describe-from-family',
            image_family, '--project=%s' % image_project)
        latest_image = next(
            (line[6:].strip() for line in out.split('\n') if (
                line.startswith('name: '))),
            None)
        if not latest_image:
            raise ValueError(
                'Failed to get the latest image from family %s in project %s' % (
                    image_family, image_project))
        # TODO(yguo0905): Support this in GKE.
        self.add_environment(
            'KUBE_GCE_NODE_IMAGE=%s' % latest_image,
            'KUBE_GCE_NODE_PROJECT=%s' % image_project)
        print >>sys.stderr, 'Set KUBE_GCE_NODE_IMAGE=%s' % latest_image
        print >>sys.stderr, 'Set KUBE_GCE_NODE_PROJECT=%s' % image_project

    def start(self, args):
        """Starts kubetest."""
        print >>sys.stderr, 'starts with local mode'
        env = {}
        env.update(self.os_env)
        env.update(self.env_files)
        env.update(self.env)
        # Do not interfere with the local project
        project = env.get('PROJECT')
        if project:
            try:
                check('gcloud', 'config', 'set', 'project', env['PROJECT'])
            except subprocess.CalledProcessError:
                print >>sys.stderr, 'Fail to set project %r', project
        else:
            print >>sys.stderr, 'PROJECT not set in job, will use local project'
        check_env(env, 'kubetest', *args)


class DockerMode(object):
    """Runs e2e tests via docker run kubekins-e2e."""
    def __init__(self, container, artifacts, sudo, tag, mount_paths):
        self.tag = tag
        try:  # Pull a newer version if one exists
            check('docker', 'pull', kubekins(tag))
        except subprocess.CalledProcessError:
            pass

        print 'Starting %s...' % container

        self.container = container
        self.local_artifacts = artifacts
        self.artifacts = '/workspace/_artifacts'
        self.cmd = [
            'docker', 'run', '--rm',
            '--name=%s' % container,
            '-v', '%s:%s' % (artifacts, self.artifacts),
            '-v', '/etc/localtime:/etc/localtime:ro',
        ]
        for path in mount_paths or []:
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
        """Runs kubetest inside a docker container."""
        print >>sys.stderr, 'starts with docker mode'
        cmd = list(self.cmd)
        cmd.append(kubekins(self.tag))
        cmd.extend(args)
        signal.signal(signal.SIGTERM, self.sig_handler)
        signal.signal(signal.SIGINT, self.sig_handler)
        try:
            check(*cmd)
        finally:  # Ensure docker files are readable by bootstrap
            if not os.path.isdir(self.local_artifacts):  # May not exist
                pass
            try:
                check('sudo', 'chmod', '-R', 'o+r', self.local_artifacts)
            except subprocess.CalledProcessError:  # fails outside CI
                traceback.print_exc()

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
        mode = DockerMode(container, artifacts, sudo, args.tag, args.mount_paths)
    elif args.mode == 'local':
        mode = LocalMode(workspace, artifacts)  # pylint: disable=bad-option-value
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

    # TODO(fejta): remove after next image push
    mode.add_environment('KUBETEST_MANUAL_DUMP=y')
    runner_args = [
        '-v',
        '--dump=%s' % mode.artifacts,
    ]

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

    # TODO(fejta): move these out of this file
    if args.up == 'true':
        runner_args.append('--up')
    if args.down == 'true':
        runner_args.append('--down')
    if args.test == 'true':
        runner_args.append('--test')

    cluster = cluster_name(args.cluster, os.getenv('BUILD_NUMBER', 0))
    runner_args.extend(args.kubetest_args)

    if args.logexporter:
        # TODO(fejta): Take the below value through a flag instead of env var.
        runner_args.append('--logexporter-gcs-path=%s' % os.environ.get('GCS_ARTIFACTS_DIR', ''))

    if args.kubeadm:
        version = kubeadm_version(args.kubeadm)
        runner_args.extend([
            '--kubernetes-anywhere-path=/workspace/kubernetes-anywhere',
            '--kubernetes-anywhere-phase2-provider=kubeadm',
            '--kubernetes-anywhere-cluster=%s' % cluster,
            '--kubernetes-anywhere-kubeadm-version=%s' % version,
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

    if args and args.image_family and args.image_project:
        mode.use_latest_image(args.image_family, args.image_project)

    mode.start(runner_args)

def create_parser():
    """Create argparser."""
    parser = argparse.ArgumentParser()
    parser.add_argument(
        '--mode', default='local', choices=['local', 'docker'])
    parser.add_argument(
        '--env-file', action="append", help='Job specific environment file')
    parser.add_argument(
        '--image-family',
        help='The image family from which to fetch the latest image')
    parser.add_argument(
        '--image-project',
        help='The image project from which to fetch the test images')
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
        action='append',
        help='Paths that should be mounted within the docker container in the form local:remote')
    parser.add_argument(
        '--build', nargs='?', default=None, const='',
        help='Build kubernetes binaries if set, optionally specifying strategy')
    parser.add_argument(
        '--cluster', default='bootstrap-e2e', help='Name of the cluster')
    parser.add_argument(
        '--docker-in-docker', action='store_true', help='Enable run docker within docker')
    parser.add_argument(
        '--kubeadm', choices=['ci', 'periodic', 'pull'])
    parser.add_argument(
        '--tag', default='v20170714-94e76415', help='Use a specific kubekins-e2e tag if set')
    parser.add_argument(
        '--test', default='true', help='If we need to run any actual test within kubetest')
    parser.add_argument(
        '--down', default='true', help='If we need to tear down the e2e cluster')
    parser.add_argument(
        '--up', default='true', help='If we need to bring up a e2e cluster')
    parser.add_argument(
        '--logexporter',
        action='store_true',
        help='If we need to use logexporter tool to upload logs from nodes to GCS directly')
    parser.add_argument(
        '--kubetest_args',
        action='append',
        default=[],
        help='Send unrecognized args directly to kubetest')
    return parser


def parse_args(args=None):
    """Return args, adding unrecognized args to kubetest_args."""
    parser = create_parser()
    args, extra = parser.parse_known_args(args)
    args.kubetest_args += extra

    if (args.image_family or args.image_project) and args.mode == 'docker':
        raise ValueError(
            '--image-family / --image-project is not supported in docker mode')
    if bool(args.image_family) != bool(args.image_project):
        raise ValueError(
            '--image-family and --image-project must be both set or unset')
    return args


if __name__ == '__main__':
    main(parse_args())
