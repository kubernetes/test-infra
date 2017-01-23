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


def sig_handler(_signo, _frame):
    """Stops container upon receive signal.SIGTERM and signal.SIGINT."""
    print >>sys.stderr, 'signo = %s, frame = %s' % (_signo, _frame)
    check(['docker', 'stop', CONTAINER])


def main(args):
    """Set up env, start kubekins-e2e, handle termination. """
    # pylint: disable=too-many-locals

    # dockerized-e2e-runner goodies setup
    workspace = os.environ.get('WORKSPACE', os.getcwd())
    artifacts = '%s/_artifacts' % workspace
    if not os.path.isdir(artifacts):
        os.makedirs(artifacts)

    e2e_image_tag = 'v20170104-9031f1d'
    e2e_image_tag_override = '%s/hack/jenkins/.kubekins_e2e_image_tag' % workspace
    if os.path.isfile(e2e_image_tag_override):
        with open(e2e_image_tag_override) as tag:
            e2e_image_tag = tag.read()

    # exec

    print 'Starting %s...' % CONTAINER

    cmd = [
      'docker', 'run', '--rm',
      '--name=%s' % CONTAINER,
      '-v', '%s/_artifacts":/workspace/_artifacts' % workspace,
      '-v', '/etc/localtime:/etc/localtime:ro'
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

    gce_ssh = '/workspace/.ssh/google_compute_engine'
    gce_pub = '%s.pub' % gce_ssh
    service = '/service-account.json'

    cmd.extend([
      '-v', '%s:%s:ro' % (args.gce_ssh, gce_ssh),
      '-v', '%s:%s:ro' % (args.gce_pub, gce_pub),
      '-v', '%s:%s:ro' % (args.service_account, service),
      '-e', 'JENKINS_GCE_SSH_PRIVATE_KEY_FILE=%s' % gce_ssh,
      '-e', 'JENKINS_GCE_SSH_PUBLIC_KEY_FILE=%s' % gce_pub,
      '-e', 'GOOGLE_APPLICATION_CREDENTIALS=%s' % service,
      # Boilerplate envs
      # Skip gcloud update checking
      '-e', 'CLOUDSDK_COMPONENT_MANAGER_DISABLE_UPDATE_CHECK=\'true\'',
      # Use default component update behavior
      '-e', 'CLOUDSDK_EXPERIMENTAL_FAST_COMPONENT_UPDATE=\'false\'',
      # E2E
      '-e', 'E2E_UP=%s' % args.up,
      '-e', 'E2E_TEST=%s' % args.test,
      '-e', 'E2E_DOWN=%s' % args.down,
      '-e', 'E2E_NAME=%s' % args.cluster,
      # AWS
      '-e', 'KUBE_AWS_INSTANCE_PREFIX=%s' % args.cluster,
      # GCE
      '-e', 'INSTANCE_PREFIX=%s' % args.cluster,
      '-e', 'KUBE_GCE_NETWORK=%s' % args.cluster,
      '-e', 'KUBE_GCE_INSTANCE_PREFIX=%s' % args.cluster,
      # GKE
      '-e', 'CLUSTER_NAME=%s' % args.cluster,
      '-e', 'KUBE_GKE_NETWORK=%s' % args.cluster,
      # Workspace
      '-e', 'HOME=/workspace',
      '-e', 'WORKSPACE=/workspace'])

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

    cmd.append('gcr.io/k8s-testimages/kubekins-e2e:%s' % e2e_image_tag)

    signal.signal(signal.SIGTERM, sig_handler)
    signal.signal(signal.SIGINT, sig_handler)

    check(*cmd)


if __name__ == '__main__':

    PARSER = argparse.ArgumentParser()
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

    # Assume we're upping, testing, and downing a cluster by default
    PARSER.add_argument(
        '--up', default='true', help='If we need to set --up in e2e.go')
    PARSER.add_argument(
        '--test', default='true', help='If we need to set --test in e2e.go')
    PARSER.add_argument(
        '--down', default='true', help='If we need to set --down in e2e.go')
    PARSER.add_argument(
        '--cluster', default='bootstrap-e2e', help='Name of the cluster')
    ARGS = PARSER.parse_args()

    CONTAINER = '%s-%s' % (os.environ.get('JOB_NAME'), os.environ.get('BUILD_NUMBER'))

    main(ARGS)
