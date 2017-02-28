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
import shutil
import signal
import subprocess
import sys

ORIG_CWD = os.getcwd()  # Checkout changes cwd

def test_infra(*paths):
    """Return path relative to root of test-infra repo."""
    return os.path.join(ORIG_CWD, os.path.dirname(__file__), '..', *paths)


def check_output(*cmd):
    """Log and run the command, return output, raising on errors."""
    print >>sys.stderr, 'Run:', cmd
    return subprocess.check_output(cmd)


def check(*cmd):
    """Log and run the command, raising on errors."""
    print >>sys.stderr, 'Run:', cmd
    subprocess.check_call(cmd)


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
          '%s/e2e-runner.sh' % self.workspace,
          test_infra('jenkins/e2e-image/e2e-runner.sh')
        ]
        for path in options:
            if os.path.isfile(path):
                return path
        raise ValueError('Cannot find e2e-runner at any of %s' % ', '.join(options))


    def install_prerequisites(self):
        """Copies upload-to-gcs and kubetest if needed."""
        parent = os.path.dirname(self.runner)
        if not os.path.isfile(os.path.join(parent, 'kubetest')):
            check('go', 'install', 'k8s.io/test-infra/kubetest')
            shutil.copy(
                os.path.expandvars('${GOPATH}/bin/kubetest'),
                os.path.join(parent, 'kubetest'))

    def start(self):
        """Runs e2e-runner.sh after setting env and installing prereqs."""
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
        check_env(env, self.runner)


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


    def start(self):
        """Runs kubekins."""
        self.cmd.append(kubekins(self.tag))
        signal.signal(signal.SIGTERM, self.sig_handler)
        signal.signal(signal.SIGINT, self.sig_handler)
        check(*self.cmd)

    def sig_handler(self, _signo, _frame):
        """Stops container upon receive signal.SIGTERM and signal.SIGINT."""
        print >>sys.stderr, 'docker stop (signo=%s, frame=%s)' % (_signo, _frame)
        check('docker', 'stop', self.container)


def main(args):
    """Set up env, start kubekins-e2e, handle termination. """
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
        mode = DockerMode(container, workspace, args.docker_in_docker, args.tag, args.mount_paths)
    elif args.mode == 'local':
        mode = LocalMode(workspace)  # pylint: disable=redefined-variable-type
    else:
        raise ValueError(args.mode)
    if args.env_file:
        mode.add_files(args.env_file)

    if args.gce_ssh:
        mode.add_gce_ssh(args.gce_ssh, args.gce_pub)

    if args.service_account:
        mode.add_service_account(args.service_account)

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
      'E2E_NAME=%s' % args.cluster,
      # AWS
      'KUBE_AWS_INSTANCE_PREFIX=%s' % args.cluster,
      # GCE
      'INSTANCE_PREFIX=%s' % args.cluster,
      'KUBE_GCE_NETWORK=%s' % args.cluster,
      'KUBE_GCE_INSTANCE_PREFIX=%s' % args.cluster,
      # GKE
      'CLUSTER_NAME=%s' % args.cluster,
      'KUBE_GKE_NETWORK=%s' % args.cluster,
    )

    # env blacklist.
    # TODO(krzyzacy) change this to a whitelist
    docker_env_ignore = [
      'GOOGLE_APPLICATION_CREDENTIALS',
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

    mode.start()



if __name__ == '__main__':

    PARSER = argparse.ArgumentParser()

    PARSER.add_argument(
        '--mode', default='docker', choices=['local', 'docker'])
    PARSER.add_argument(
        '--env-file', action="append", help='Job specific environment file')

    PARSER.add_argument(
        '--gce-ssh',
        default=os.environ.get('JENKINS_GCE_SSH_PRIVATE_KEY_FILE'),
        help='Path to .ssh/google_compute_engine keys')
    PARSER.add_argument(
        '--gce-pub',
        default=os.environ.get('JENKINS_GCE_SSH_PUBLIC_KEY_FILE'),
        help='Path to pub gce ssh key')
    PARSER.add_argument(
        '--service-account',
        default=os.environ.get('GOOGLE_APPLICATION_CREDENTIALS'),
        help='Path to service-account.json')
    PARSER.add_argument(
        '--mount-paths',
        default=[],
        nargs='*',
        help='Paths that should be mounted within the docker container in the form local:remote')

    # Assume we're upping, testing, and downing a cluster by default
    PARSER.add_argument(
        '--cluster', default='bootstrap-e2e', help='Name of the cluster')
    PARSER.add_argument(
        '--docker-in-docker', action='store_true', help='Enable run docker within docker')
    PARSER.add_argument(
        '--down', default='true', help='If we need to set --down in e2e.go')
    PARSER.add_argument(
        '--soak-test', action='store_true', help='If the test is a soak test job')
    PARSER.add_argument(
        '--tag', default='v20170228-ebc41180', help='Use a specific kubekins-e2e tag if set')
    PARSER.add_argument(
        '--test', default='true', help='If we need to set --test in e2e.go')
    PARSER.add_argument(
        '--up', default='true', help='If we need to set --up in e2e.go')
    ARGS = PARSER.parse_args()

    main(ARGS)
