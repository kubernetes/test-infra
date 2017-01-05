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
import subprocess
import sys


def check_output(*cmd):
    """Log and run the command, return output, raising on errors."""
    print >>sys.stderr, 'Run:', cmd
    return subprocess.check_output(cmd)


def check(*cmd):
    """Log and run the command, raising on errors."""
    print >>sys.stderr, 'Run:', cmd
    subprocess.check_call(cmd)


def continuation_lines(fin):
    """Read in lines respect \\ in the end."""
    for line in fin:
        line = line.rstrip('\n')
        while line.endswith('\\'):
            line = line[:-1] + next(fin).rstrip('\n')
        yield line


def loadenv(envpath):
    """Load a env file into os.environ."""
    if not os.path.isfile(envpath):
        print >>sys.stderr, 'envfile %s does not exist' % envpath
        return
    with open(envpath, 'r') as envfile:
        for line in continuation_lines(envfile):
            tup = line.strip().split('=', 1)
            if len(tup) == 2:
                os.environ[tup[0].strip()] = tup[1].strip()


def main(args):
    """ set up env, call docker run, clean up """
    # pylint: disable=too-many-locals
    # pylint: disable=too-many-branches
    # pylint: disable=too-many-statements


    # platform envs
    if args.platform:
        loadenv(os.getcwd() + '/platforms/' + args.platform + '.env')

    # job envs
    if args.env:
        loadenv(os.getcwd() + '/jobs/' + args.env + '.env')

    # Boilerplate envs
    # Assume we're upping, testing, and downing a cluster
    os.environ['E2E_UP'] = os.environ.get('E2E_UP') or 'true'
    os.environ['E2E_TEST'] = os.environ.get('E2E_TEST') or 'true'
    os.environ['E2E_DOWN'] = os.environ.get('E2E_DOWN') or 'true'

    e2e_name = os.environ.get('E2E_NAME') or 'bootstrap-e2e'

    # Skip gcloud update checking
    os.environ['CLOUDSDK_COMPONENT_MANAGER_DISABLE_UPDATE_CHECK'] = 'true'
    # Use default component update behavior
    os.environ['CLOUDSDK_EXPERIMENTAL_FAST_COMPONENT_UPDATE'] = 'false'

    # AWS variables
    os.environ['KUBE_AWS_INSTANCE_PREFIX'] = e2e_name

    # GCE variables
    os.environ['INSTANCE_PREFIX'] = e2e_name
    os.environ['KUBE_GCE_NETWORK'] = e2e_name
    os.environ['KUBE_GCE_INSTANCE_PREFIX'] = e2e_name

    # GKE variables
    os.environ['CLUSTER_NAME'] = e2e_name
    os.environ['KUBE_GKE_NETWORK'] = e2e_name

    # Get golang into our PATH so we can run e2e.go
    os.environ['PATH'] += ':/usr/local/go/bin'

    # dockerized-e2e-runner goodies setup
    workspace = os.environ.get('WORKSPACE') or os.getcwd()
    repo = os.environ.get('REPO_DIR') or os.getcwd()
    artifacts = '%s/_artifacts' % workspace
    os.makedirs(artifacts)

    # TODO(ixdy): remove when all jobs are setting these vars using Jenkins credentials
    gce_key = '/var/lib/jenkins/gce_keys/google_compute_engine'
    if os.path.isfile(gce_key):
        os.environ['JENKINS_GCE_SSH_PRIVATE_KEY_FILE'] = gce_key

    gce_key_pub = '/var/lib/jenkins/gce_keys/google_compute_engine.pub'
    if os.path.isfile(gce_key_pub):
        os.environ['JENKINS_GCE_SSH_PUBLIC_KEY_FILE'] = gce_key_pub


    e2e_image_tag = 'v20170104-9031f1d'
    e2e_image_tag_override = '%s/hack/jenkins/.kubekins_e2e_image_tag' % workspace
    if os.path.isfile(e2e_image_tag_override):
        with open(e2e_image_tag_override, 'r') as tag:
            e2e_image_tag = tag.read()

    docker_env_ignore = [
      'GOOGLE_APPLICATION_CREDENTIALS',
      'GOROOT',
      'HOME',
      'PATH',
      'PWD',
      'WORKSPACE'
    ]

    docker_extra_args = []
    if (os.environ.get('JENKINS_ENABLE_DOCKER_IN_DOCKER') and
            os.environ.get('JENKINS_ENABLE_DOCKER_IN_DOCKER').lower() == 'y'):
        docker_extra_args.extend([
          '-v', '/var/run/docker.sock:/var/run/docker.sock',
          '-v', '%s:/go/src/k8s.io/kubernetes' % repo,
          '-e', 'REPO_DIR=%s' % repo,
          '-e', 'HOST_ARTIFACTS_DIR=%s' % artifacts
        ])

    if (os.environ.get('JENKINS_USE_LOCAL_BINARIES') and
            os.environ.get('JENKINS_USE_LOCAL_BINARIES').lower() == 'y'):
        docker_extra_args.extend([
          '-v', '%s/_output":/workspace/_output:ro' % workspace
        ])

    if 'KUBE_E2E_RUNNER' in os.environ:
        docker_extra_args.extend([
          '--entrypoint=%s', os.environ.get('KUBE_E2E_RUNNER')
        ])

    # exec
    container = '%s-%s' % (os.environ.get('JOB_NAME'), os.environ.get('BUILD_NUMBER'))

    print 'Starting...'

    cmd = ['docker', 'run', '--rm',
      '--name=%s' % container,
      '-v', '%s/_artifacts":/workspace/_artifacts' % workspace,
      '-v', '/etc/localtime:/etc/localtime:ro'
    ]

    if os.environ.get('JENKINS_GCE_SSH_PRIVATE_KEY_FILE'):
        cmd.extend(['-v', '%s:/workspace/.ssh/google_compute_engine:ro'
            % os.environ['JENKINS_GCE_SSH_PRIVATE_KEY_FILE']])

    if os.environ.get('JENKINS_GCE_SSH_PUBLIC_KEY_FILE'):
        cmd.extend(['-v', '%s:/workspace/.ssh/google_compute_engine.pub:ro'
            % os.environ['JENKINS_GCE_SSH_PUBLIC_KEY_FILE']])

    if os.environ.get('JENKINS_AWS_SSH_PRIVATE_KEY_FILE'):
        cmd.extend(['-v', '%s:/workspace/.ssh/kube_aws_rsa:ro'
            % os.environ['JENKINS_AWS_SSH_PRIVATE_KEY_FILE']])

    if os.environ.get('JENKINS_AWS_SSH_PUBLIC_KEY_FILE'):
        cmd.extend(['-v', '%s:/workspace/.ssh/kube_aws_rsa.pub:ro'
            % os.environ['JENKINS_AWS_SSH_PUBLIC_KEY_FILE']])

    if os.environ.get('JENKINS_AWS_CREDENTIALS_FILE'):
        cmd.extend(['-v', '%s:/workspace/.aws/credentials:ro'
            % os.environ['JENKINS_AWS_CREDENTIALS_FILE']])

    if os.environ.get('GOOGLE_APPLICATION_CREDENTIALS'):
        cmd.extend(['-v', '%s:/service-account.json:ro'
            % os.environ['GOOGLE_APPLICATION_CREDENTIALS'],
                    '-e', 'GOOGLE_APPLICATION_CREDENTIALS=/service-account.json'])

    for key, value in os.environ.items():
        if key not in docker_env_ignore:
            cmd.extend(['-e', '%s=%s' % (key, value)])

    cmd.extend(docker_extra_args)

    cmd.extend([
      '-e', 'HOME=/workspace',
      '-e', 'WORKSPACE=/workspace',
      'gcr.io/k8s-testimages/kubekins-e2e:%s' % e2e_image_tag
    ])

    try:
        check(*cmd)
        print 'Finished Successfully'
    except subprocess.CalledProcessError as exc:
        print 'Exiting with code: %d' % exc.returncode


if __name__ == '__main__':

    PARSER = argparse.ArgumentParser(
        'Runs e2e jobs on the kubernetes repo')
    PARSER.add_argument(
        '--branch', default='master', help='Upstream target repo')
    PARSER.add_argument(
        '--platform', help='Platform test runs on')
    PARSER.add_argument(
        '--env', help='Job specific environment variables')
    ARGS = PARSER.parse_args()

    main(ARGS)
