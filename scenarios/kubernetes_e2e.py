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
import signal
import subprocess
import sys
import tempfile
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

    @staticmethod
    def add_service_account(path):
        """Returns path."""
        return path

    def add_k8s(self, *a, **kw):
        """Add specified k8s.io repos (noop)."""
        pass

    def add_aws_profile(self, _name, _config):
        """Add aws profile envs."""
        self.add_environment('AWS_SDK_LOAD_CONFIG=true')

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

    def add_aws_runner(self):
        """Start with kops-e2e-runner.sh"""
        # TODO(Krzyzacy):retire kops-e2e-runner.sh
        self.command = '/workspace/kops-e2e-runner.sh'

    def start(self, args):
        """Starts kubetest."""
        print >>sys.stderr, 'starts with local mode'
        env = {}
        env.update(self.os_env)
        env.update(self.env_files)
        env.update(self.env)
        check_env(env, self.command, *args)


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
        self.add_env('HOME=/workspace')
        self.add_env('WORKSPACE=/workspace')

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
                self.add_env(env)

    def add_os_environment(self, *envs):
        """Adds os envs as FOO=BAR to the -e list for docker."""
        self.add_environment(*envs)

    def add_file(self, env_file):
        """Adds the file to the --env-file list."""
        self.cmd.extend(['--env-file', env_file])

    def add_env(self, env):
        """Adds a single environment variable to the -e list for docker.

        Does not check against any blacklists."""
        self.cmd.extend(['-e', env])

    def add_k8s(self, k8s, *repos):
        """Add the specified k8s.io repos into container."""
        for repo in repos:
            self.cmd.extend([
                '-v', '%s/%s:/go/src/k8s.io/%s' % (k8s, repo, repo)])

    def add_aws_profile(self, name, config):
        """Add aws profile envs."""
        self.add_environment('AWS_SDK_LOAD_CONFIG=true')
        self.cmd.extend([
          '-v', '%s:%s:ro' % (name, config),
          '-e', 'AWS_SDK_LOAD_CONFIG=true',
        ])

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

    def add_aws_runner(self):
        """Run kops_aws_runner for kops-aws jobs."""
        self.cmd.append(
          '--entrypoint=/workspace/kops-e2e-runner.sh'
        )

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
        """Mounts path at /service-account.json inside the container."""
        service = '/service-account.json'
        self.cmd.extend(['-v', '%s:%s:ro' % (path, service)])
        return service

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


# TODO(krzyzacy): Move this into kubetest
def build_kops(workspace, mode):
    """Build kops, set kops related envs."""
    if not os.path.basename(workspace) == 'kops':
        raise ValueError(workspace)
    version = 'pull-' + check_output('git', 'describe', '--always').strip()
    job = os.getenv('JOB_NAME', 'pull-kops-e2e-kubernetes-aws')
    gcs = 'gs://kops-ci/pulls/%s' % job
    gapi = 'https://storage.googleapis.com/kops-ci/pulls/%s' % job
    mode.add_environment([
        'KOPS_BASE_URL=%s/%s' % (gapi, version),
        'GCS_LOCATION=%s' % gcs
        ])
    check('make', 'gcs-publish-ci', 'VERSION=%s' % version, 'GCS_LOCATION=%s' % gcs)


def set_up_aws(args, mode, cluster, runner_args):
    """Set up aws related envs."""
    for path in [args.aws_ssh, args.aws_pub, args.aws_cred]:
        if not os.path.isfile(os.path.expandvars(path)):
            raise IOError(path, os.path.expandvars(path))
    mode.add_aws_cred(args.aws_ssh, args.aws_pub, args.aws_cred)

    aws_config = '/workspace/.aws/config'
    aws_ssh = '/workspace/.ssh/kube_aws_rsa'
    profile = args.aws_profile
    if args.aws_role_arn:
        with tempfile.NamedTemporaryFile(prefix='aws-config', delete=False) as cfg:
            cfg.write(
                '[profile jenkins-assumed-role]\nrole_arn = %s\nsource_profile = %s\n' % (
                    args.aws_role_arn, profile))
            mode.add_aws_profile(cfg.name, aws_config)
    profile = 'jenkins-assumed-role'

    zones = args.kops_zones or random.choice([
        'us-west-1a',
        'us-west-1c',
        'us-west-2a',
        'us-west-2b',
        'us-east-1a',
        'us-east-1d',
        'us-east-2a',
        'us-east-2b',
    ])
    regions = ','.join([zone[:-1] for zone in zones.split(',')])

    mode.add_environment(
      'AWS_PROFILE=%s' % profile,
      'AWS_DEFAULT_PROFILE=%s' % profile,
      'KOPS_REGIONS=%s' % regions,
    )

    if args.aws_cluster_domain:
        cluster = '%s.%s' % (cluster, args.aws_cluster_domain)

    runner_args.extend([
        '--kops-cluster=%s' % cluster,
        '--kops-zones=%s ' % zones,
        '--kops-state=%s' % args.kops_state,
        '--kops-nodes=%s' % args.kops_nodes,
        '--kops-ssh-key=%s' % aws_ssh,
    ])
    # TODO(krzyzacy):Remove after retire kops-e2e-runner.sh
    mode.add_aws_runner()

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

    # Set up workspace/artifacts dir
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

    for env_file in args.env_file:
        mode.add_file(test_infra(env_file))
    for env in args.env:
        mode.add_env(env)
    if args.gce_ssh:
        mode.add_gce_ssh(args.gce_ssh, args.gce_pub)

    # TODO(fejta): remove after next image push
    mode.add_environment('KUBETEST_MANUAL_DUMP=y')
    runner_args = [
        '-v',
        '--dump=%s' % mode.artifacts,
    ]

    if args.service_account:
        runner_args.append(
            '--gcp-service-account=%s' % mode.add_service_account(args.service_account))

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

    if args.kops_build:
        build_kops(workspace, mode)

    # TODO(fejta): move these out of this file
    if args.up == 'true':
        runner_args.append('--up')
    if args.down == 'true':
        runner_args.append('--down')
    if args.test == 'true':
        runner_args.append('--test')

    cluster = cluster_name(args.cluster, os.getenv('BUILD_NUMBER', 0))
    runner_args.append('--cluster=%s' % cluster)
    runner_args.append('--gcp-network=%s' % cluster)
    runner_args.extend(args.kubetest_args)

    if args.use_logexporter:
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

    if args.aws:
        set_up_aws(args, mode, cluster, runner_args)

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

    if args and args.image_family and args.image_project:
        mode.use_latest_image(args.image_family, args.image_project)

    mode.start(runner_args)

def create_parser():
    """Create argparser."""
    parser = argparse.ArgumentParser()
    parser.add_argument(
        '--mode', default='local', choices=['local', 'docker'])
    parser.add_argument(
        '--env-file', default=[], action="append",
        help='Job specific environment file')
    parser.add_argument(
        '--env', default=[], action="append",
        help='Job specific environment setting ' +
        '(usage: "--env=VAR=SETTING" will set VAR to SETTING).')
    parser.add_argument(
        '--image-family',
        help='The image family from which to fetch the latest image')
    parser.add_argument(
        '--image-project',
        help='The image project from which to fetch the test images')
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
        '--tag', default='v20170728-faff708c', help='Use a specific kubekins-e2e tag if set')
    parser.add_argument(
        '--test', default='true', help='If we need to run any actual test within kubetest')
    parser.add_argument(
        '--down', default='true', help='If we need to tear down the e2e cluster')
    parser.add_argument(
        '--up', default='true', help='If we need to bring up a e2e cluster')
    parser.add_argument(
        '--use-logexporter',
        action='store_true',
        help='If we need to use logexporter tool to upload logs from nodes to GCS directly')
    parser.add_argument(
        '--kubetest_args',
        action='append',
        default=[],
        help='Send unrecognized args directly to kubetest')

    # aws
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
        '--kops-state', default='s3://k8s-kops-jenkins/',
        help='Name of the aws state storage')
    parser.add_argument(
        '--kops-zones', help='Comma-separated list of zones else random choice')
    parser.add_argument(
        '--kops-build', action='store_true', help='If we need to build kops locally')

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

    if args.aws:
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
