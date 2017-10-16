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

"""Runs heapster tests for kubernetes/heapster."""

import argparse
import os
import shutil
import subprocess
import sys
import tempfile


def check(*cmd):
    """Log and run the command, raising on errors."""
    print >>sys.stderr, 'Run:', cmd
    subprocess.check_call(cmd)

HEAPSTER_IMAGE_VERSION = '0.8'

def main(ssh, ssh_pub, robot, project):
    """Run unit/integration heapster test against master in docker"""

    img = 'gcr.io/k8s-testimages/heapster-test:%s' % HEAPSTER_IMAGE_VERSION
    artifacts = '%s/_artifacts' % os.environ['WORKSPACE']
    if not os.path.isdir(artifacts):
        os.makedirs(artifacts)
    heapster = os.getcwd()
    if not os.path.basename(heapster) == 'heapster':
        raise ValueError(heapster)

    for path in [ssh, ssh_pub, robot]:
        if not os.path.isfile(os.path.expandvars(path)):
            raise IOError(path, os.path.expandvars(path))
    private = '/root/.ssh/google_compute_engine'
    public = '%s.pub' % private
    service = '/service-account.json'

    temp = tempfile.mkdtemp(prefix='heapster-')
    try:
        check(
            'docker', 'run', '--rm=true',
            '-v', '/etc/localtime:/etc/localtime:ro',
            '-v', '/var/run/docker.sock:/var/run/docker.sock',
            '-v', '%s:/go/src/k8s.io/heapster' % heapster,
            '-v', '%s:%s' % (temp, temp),
            '-v', '%s:/workspace/_artifacts' % artifacts,
            '-v', '%s:%s:ro' % (robot, service),
            '-v', '%s:%s:ro' % (ssh, private),
            '-v', '%s:%s:ro' % (ssh_pub, public),
            '-e', 'GOOGLE_APPLICATION_CREDENTIALS=%s' % service,
            '-e', 'JENKINS_GCE_SSH_PRIVATE_KEY_FILE=%s' % private,
            '-e', 'JENKINS_GCE_SSH_PUBLIC_KEY_FILE=%s' % public,
            '-e', 'REPO_DIR=%s' % heapster, # Used in heapster/Makefile
            '-e', 'TEMP_DIR=%s' % temp,
            '-e', 'PROJECT=%s' % project,
            img,
        )
        shutil.rmtree(temp)
    except subprocess.CalledProcessError:
        shutil.rmtree(temp)
        raise

if __name__ == '__main__':
    PARSER = argparse.ArgumentParser(
        'Runs heapster tests with the specified creds')
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
    # TODO(https://github.com/kubernetes/heapster/issues/1501): Move this to heapster
    PARSER.add_argument(
        '--project',
        default='kubernetes-jenkins-pull',
        help='GCP project where heapster test runs from')
    ARGS = PARSER.parse_args()
    main(
        ARGS.gce_ssh,
        ARGS.gce_pub,
        ARGS.service_account,
        ARGS.project
    )
