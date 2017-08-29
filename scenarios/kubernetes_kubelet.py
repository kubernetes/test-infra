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

"""Runs node and/or kubelet tests for kubernetes/kubernetes."""

import argparse
import os
import re
import shutil
import subprocess
import sys

BRANCH_VERSION = {
    'release-1.2': 'release-1.4',
    'release-1.3': 'release-1.4',
    'master': 'release-1.7',
}

VERSION_TAG = {
    'release-1.4': '1.4-latest',
    'release-1.5': '1.5-latest',
    'release-1.6': '1.6-latest',
    'release-1.7': '1.7-latest',
}

ORIG_CWD = os.getcwd()  # Checkout changes cwd


def check(*cmd):
    """Log and run the command, raising on errors."""
    print >>sys.stderr, 'Run:', cmd
    subprocess.check_call(cmd)


def var(path):
    """Expands '${foo} interesting' to 'something interesting'."""
    return os.path.expandvars(path)


def run_docker_mode(script, properties, branch, ssh, ssh_pub, robot):
    """Test node branch by sending script specified properties and creds."""
    # If branch has 3-part version, only take first 2 parts.
    mat = re.match(r'master|release-\d+\.\d+', branch)
    if not mat:
        raise ValueError(branch)
    tag = VERSION_TAG[BRANCH_VERSION.get(mat.group(0), mat.group(0))]
    img = 'gcr.io/k8s-testimages/kubekins-node:%s' % tag
    artifacts = '%s/_artifacts' % os.environ['WORKSPACE']
    if not os.path.isdir(artifacts):
        os.makedirs(artifacts)
    k8s = os.getcwd()
    if not os.path.basename(k8s) == 'kubernetes':
        raise ValueError(k8s)

    for path in [ssh, ssh_pub, robot]:
        if not os.path.isfile(var(path)):
            raise IOError(path, var(path))
    private = '/root/.ssh/google_compute_engine'
    public = '%s.pub' % private
    service = '/service-account.json'

    os.environ['NODE_TEST_SCRIPT'] = script
    os.environ['NODE_TEST_PRORPERTIES'] = properties
    check('docker', 'pull', img)  # Update image if changed
    check(
        'docker', 'run', '--rm=true',
        '-v', '/etc/localtime:/etc/localtime:ro',
        '-v', '/var/run/docker.sock:/var/run/docker.sock',
        '-v', '%s:/go/src/k8s.io/kubernetes' % k8s,
        '-v', '%s:/workspace/_artifacts' % artifacts,
        '-v', '%s:%s:ro' % (robot, service),
        '-v', '%s:%s:ro' % (ssh, private),
        '-v', '%s:%s:ro' % (ssh_pub, public),
        '-e', 'GCE_USER=jenkins',
        '-e', 'GOOGLE_APPLICATION_CREDENTIALS=%s' % service,
        '-e', 'JENKINS_GCE_SSH_PRIVATE_KEY_FILE=%s' % private,
        '-e', 'JENKINS_GCE_SSH_PUBLIC_KEY_FILE=%s' % public,
        '-e', 'NODE_TEST_PROPERTIES=%s' % properties,
        '-e', 'NODE_TEST_SCRIPT=%s' % script,
        '-e', 'REPO_DIR=%s' % k8s,  # TODO(fejta): used?
        img,
    )

def test_infra(*paths):
    """Return path relative to root of test-infra repo."""
    return os.path.join(ORIG_CWD, os.path.dirname(__file__), '..', *paths)


def add_gce_ssh(private_key, public_key):
    """Copies priv, pub keys to /root/.ssh."""
    ssh_dir = '/root/.ssh'
    if not os.path.isdir(ssh_dir):
        os.makedirs(ssh_dir)
    gce_ssh = '%s/google_compute_engine' % ssh_dir
    gce_pub = '%s/google_compute_engine.pub' % ssh_dir
    shutil.copy(private_key, gce_ssh)
    shutil.copy(public_key, gce_pub)


def parse_env(env):
    """Returns (FOO, BAR=MORE) for FOO=BAR=MORE."""
    return env.split('=', 1)


def get_envs_from_file(env_file):
    """Returns all FOO=BAR lines from env_file."""
    envs = []
    with open(env_file) as fp:
        for line in fp:
            line = line.rstrip()
            if not line or line.startswith('#'):
                continue
            envs.append(parse_env(line))
    return dict(envs)


def run_local_mode(run_args, private_key, public_key):
    """Checkout, build and trigger the node e2e tests locally."""
    k8s = os.getcwd()
    if not os.path.basename(k8s) == 'kubernetes':
        raise ValueError(k8s)
    add_gce_ssh(private_key, public_key)
    check(
        'go', 'run', 'test/e2e_node/runner/remote/run_remote.go',
        '--logtostderr',
        '--vmodule=*=4',
        '--ssh-env=gce',
        '--results-dir=%s/_artifacts' % k8s,
        *run_args)


def main():
    if ARGS.mode == 'docker':
        run_docker_mode(
            ARGS.script,
            ARGS.properties,
            ARGS.branch,
            ARGS.gce_ssh,
            ARGS.gce_pub,
            ARGS.service_account,
        )
    elif ARGS.mode == 'local':
        run_local_mode(
            RUN_ARGS,
            ARGS.gce_ssh,
            ARGS.gce_pub,
        )
    else:
        raise ValueError(ARGS.mode)


if __name__ == '__main__':
    PARSER = argparse.ArgumentParser(
        'Runs kubelet tests with the specified script, properties and creds')
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
        '--branch', default='master', help='Branch used for testing')
    PARSER.add_argument(
        '--properties',
        default="test/e2e_node/jenkins/jenkins-ci.properties",
        help='Path to a .properties file')
    PARSER.add_argument(
        '--script',
        default='./test/e2e_node/jenkins/e2e-node-jenkins.sh',
        help='Script in kubernetes/kubernetes that runs checks')
    PARSER.add_argument(
        '--mode', default='docker', choices=['local', 'docker'])
    ARGS, RUN_ARGS = PARSER.parse_known_args()
    main()
