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
import subprocess
import sys

BRANCH_VERSION = {
    'release-1.2': 'release-1.4',
    'release-1.3': 'release-1.4',
    'master': 'release-1.6',
}

VERSION_TAG = {
    'release-1.4': '1.4-latest',
    'release-1.5': '1.5-latest',
    'release-1.6': '1.6-latest',
}


def check(*cmd):
    """Log and run the command, raising on errors."""
    print >>sys.stderr, 'Run:', cmd
    subprocess.check_call(cmd)


def var(path):
    """Expands '${foo} interesting' to 'something interesting'."""
    return os.path.expandvars(path)


def main(script, properties, branch, ssh, ssh_pub, robot, skip):
    """Test node branch by sending script specified properties and creds."""
    # pylint: disable=too-many-locals
    if skip and os.environ.get('PULL_BASE_REF'):
        if os.environ.get('PULL_BASE_REF') in skip:
            print >>sys.stderr, 'Test Skipped'
            return
    mat = re.match(r'master|release-\d+\.\d+', branch)
    if not mat:
        raise ValueError(branch)
    tag = VERSION_TAG[BRANCH_VERSION.get(branch, branch)]
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
        '--skip-release',
        help='Legacy branches that PR node e2e job should disabled for.')
    ARGS = PARSER.parse_args()
    main(
        ARGS.script,
        ARGS.properties,
        ARGS.branch,
        ARGS.gce_ssh,
        ARGS.gce_pub,
        ARGS.service_account,
        ARGS.skip_release,
    )
