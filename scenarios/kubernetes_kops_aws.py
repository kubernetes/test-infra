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
import random
import signal
import string
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


def setup_signal_handlers(container):
    """Establish a signal handler to kill 'container'."""
    def sig_handler(_signo, _frame):
        """Stops container upon receive signal.SIGTERM and signal.SIGINT."""
        print >>sys.stderr, 'signo = %s, frame = %s' % (_signo, _frame)
        check('docker', 'stop', container)

    signal.signal(signal.SIGTERM, sig_handler)
    signal.signal(signal.SIGINT, sig_handler)


def kubekins(tag):
    """Return full path to kubekins-e2e:tag."""
    return 'gcr.io/k8s-testimages/kubekins-e2e:%s' % tag


def main(args):
    """Set up env, start kops-runner, handle termination. """
    # pylint: disable=too-many-locals

    job_name = (os.environ.get('JOB_NAME') or
                os.environ.get('USER') or
                'kops-aws-test')
    build_number = (os.environ.get('BUILD_NUMBER') or
                    ''.join(
                        random.choice(string.ascii_lowercase + string.digits)
                        for _ in range(8)))
    container = '%s-%s' % (job_name, build_number)

    # dockerized-e2e-runner goodies setup
    workspace = os.environ.get('WORKSPACE', os.getcwd())
    artifacts = '%s/_artifacts' % workspace
    if not os.path.isdir(artifacts):
        os.makedirs(artifacts)

    for path in [args.aws_ssh, args.aws_pub, args.aws_cred]:
        if not os.path.isfile(os.path.expandvars(path)):
            raise IOError(path, os.path.expandvars(path))

    try:  # Pull a newer version if one exists
        check('docker', 'pull', kubekins(args.tag))
    except subprocess.CalledProcessError:
        pass

    print 'Starting %s...' % container

    cmd = [
      'docker', 'run', '--rm',
      '--name=%s' % container,
      '-v', '%s/_artifacts:/workspace/_artifacts' % workspace,
      '-v', '/etc/localtime:/etc/localtime:ro',
      '--entrypoint=/workspace/kops-e2e-runner.sh'
    ]

    # Rules for env var priority here in docker:
    # -e FOO=a -e FOO=b -> FOO=b
    # --env-file FOO=a --env-file FOO=b -> FOO=b
    # -e FOO=a --env-file FOO=b -> FOO=a(!!!!)
    # --env-file FOO=a -e FOO=b -> FOO=b
    #
    # So if you overwrite FOO=c for a local run it will take precedence.
    #

    if args.env_file:
        for env in args.env_file:
            cmd.extend(['--env-file', test_infra(env)])

    # Enforce to be always present
    aws_ssh = '/workspace/.ssh/kube_aws_rsa'
    aws_pub = '%s.pub' % aws_ssh
    aws_cred = '/workspace/.aws/credentials'

    cmd.extend([
      '-v', '%s:%s:ro' % (args.aws_ssh, aws_ssh),
      '-v', '%s:%s:ro' % (args.aws_pub, aws_pub),
      '-v', '%s:%s:ro' % (args.aws_cred, aws_cred),
      ])
    if args.service_account:
        service = '/service-account.json'
        cmd.extend(['-v', '%s:%s:ro' % (args.service_account, service),
                    '-e', 'GOOGLE_APPLICATION_CREDENTIALS=%s' % service])

    zones = args.zones
    if not zones:
        zones = random.choice([
            'us-west-1a',
            'us-west-1c',
            'us-west-2a',
            'us-west-2b',
            'us-east-1a',
            'us-east-1d',
            #'us-east-2a',
            #'us-east-2b',
        ])
    regions = ','.join([zone[:-1] for zone in zones.split(',')])

    e2e_opt = ('--kops-cluster %s --kops-zones %s '
               '--kops-state %s --kops-nodes=%s --kops-ssh-key=%s' %
               (args.cluster, zones, args.state, args.nodes, aws_ssh))
    if args.image:
        e2e_opt += ' --kops-image=%s' % args.image

    cmd.extend([
      # Boilerplate envs
      # Jenkins required variables
      '-e', 'JOB_NAME=%s' % job_name,
      '-e', 'BUILD_NUMBER=%s' % build_number,
      # KOPS_REGIONS is needed by log dump hook in kops-e2e-runner.sh
      '-e', 'KOPS_REGIONS=%s' % regions,
      # E2E
      '-e', 'E2E_UP=%s' % args.up,
      '-e', 'E2E_TEST=%s' % args.test,
      '-e', 'E2E_DOWN=%s' % args.down,
      # Kops
      '-e', 'E2E_OPT=%s' % e2e_opt,
    ])

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

    for key, value in os.environ.items():
        if key not in docker_env_ignore:
            cmd.extend(['-e', '%s=%s' % (key, value)])

    cmd.append(kubekins(args.tag))

    setup_signal_handlers(container)

    check(*cmd)


if __name__ == '__main__':

    PARSER = argparse.ArgumentParser()
    PARSER.add_argument(
        '--env-file', action="append", help='Job specific environment file')

    PARSER.add_argument(
        '--aws-ssh',
        default=os.environ.get('JENKINS_AWS_SSH_PRIVATE_KEY_FILE'),
        help='Path to private aws ssh keys')
    PARSER.add_argument(
        '--aws-pub',
        default=os.environ.get('JENKINS_AWS_SSH_PUBLIC_KEY_FILE'),
        help='Path to pub aws ssh key')
    PARSER.add_argument(
        '--aws-cred',
        default=os.environ.get('JENKINS_AWS_CREDENTIALS_FILE'),
        help='Path to aws credential file')
    PARSER.add_argument(
        '--service-account',
        default=os.environ.get('GOOGLE_APPLICATION_CREDENTIALS'),
        help='Path to service-account.json')

    # Assume we're upping, testing, and downing a cluster by default
    PARSER.add_argument(
        '--cluster', help='Name of the aws cluster (required)')
    PARSER.add_argument(
        '--down', default='true', help='If we need to set --down in e2e.go')
    PARSER.add_argument(
        '--nodes', default=4, type=int, help='Number of nodes to start')
    PARSER.add_argument(
        '--state', default='s3://k8s-kops-jenkins/',
        help='Name of the aws state storage')
    PARSER.add_argument(
        '--tag', default='v20170207-9bbd5f41',
        help='Use a specific kubekins-e2e tag if set')
    PARSER.add_argument(
        '--test', default='true', help='If we need to set --test in e2e.go')
    PARSER.add_argument(
        '--up', default='true', help='If we need to set --up in e2e.go')
    PARSER.add_argument(
        '--zones', default=None,
        help='Availability zones to start the cluster in. '
        'Defaults to a random zone.')
    PARSER.add_argument(
        '--image', default='',
        help='Image (AMI) for nodes to use. Defaults to kops default.')
    ARGS = PARSER.parse_args()

    if not ARGS.cluster:
        raise ValueError('--cluster must be provided')

    # If aws keys are missing, try to fetch from HOME dir
    if not (ARGS.aws_ssh or ARGS.aws_pub or ARGS.aws_cred):
        HOME = os.environ.get('HOME')
        if not HOME:
            raise ValueError('HOME dir not set!')
        if not ARGS.aws_ssh:
            ARGS.aws_ssh = '%s/.ssh/kube_aws_rsa' % HOME
            print >>sys.stderr, 'AWS ssh key not found. Try to fetch from %s' % ARGS.aws_ssh
        if not ARGS.aws_pub:
            ARGS.aws_pub = '%s/.ssh/kube_aws_rsa.pub' % HOME
            print >>sys.stderr, 'AWS pub key not found. Try to fetch from %s' % ARGS.aws_pub
        if not ARGS.aws_cred:
            ARGS.aws_cred = '%s/.aws/credentials' % HOME
            print >>sys.stderr, 'AWS cred not found. Try to fetch from %s' % ARGS.aws_cred

    main(ARGS)
