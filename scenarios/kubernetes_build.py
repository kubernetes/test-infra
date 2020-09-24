#!/usr/bin/env python

# Copyright 2016 The Kubernetes Authors.
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

"""Builds kubernetes with specified config"""

import argparse
import os
import re
import subprocess
import sys


def check(*cmd):
    """Log and run the command, raising on errors."""
    print >>sys.stderr, 'Run:', cmd
    subprocess.check_call(cmd)

def check_no_stdout(*cmd):
    """Log and run the command, suppress stdout & stderr, raising on errors."""
    print >>sys.stderr, 'Run:', cmd
    null = open(os.devnull, 'w')
    subprocess.check_call(cmd, stdout=null, stderr=null)

def check_output(*cmd):
    """Log and run the command, raising on errors, return output"""
    print >>sys.stderr, 'Run:', cmd
    return subprocess.check_output(cmd)

def check_build_exists(gcs, suffix, fast):
    """ check if a k8s build with same version
        already exists in remote path
    """
    if not os.path.exists('hack/print-workspace-status.sh'):
        print >>sys.stderr, 'hack/print-workspace-status.sh not found, continue'
        return False

    version = ''
    try:
        match = re.search(
            r'gitVersion ([^\n]+)',
            check_output('hack/print-workspace-status.sh')
        )
        if match:
            version = match.group(1)
    except subprocess.CalledProcessError as exc:
        # fallback with doing a real build
        print >>sys.stderr, 'Failed to get k8s version, continue: %s' % exc
        return False

    if version:
        if not gcs:
            gcs = 'kubernetes-release-dev'
        gcs = 'gs://' + gcs
        mode = 'ci'
        if fast:
            mode += '/fast'
        if suffix:
            mode += suffix
        gcs = os.path.join(gcs, mode, version)
        try:
            check_no_stdout('gsutil', 'ls', gcs)
            check_no_stdout('gsutil', 'ls', gcs + "/kubernetes.tar.gz")
            check_no_stdout('gsutil', 'ls', gcs + "/bin")
            return True
        except subprocess.CalledProcessError as exc:
            print >>sys.stderr, (
                'gcs path %s (or some files under it) does not exist yet, continue' % gcs)
    return False


def main(args):
    # pylint: disable=too-many-branches
    """Build and push kubernetes.

    This is a python port of the kubernetes/hack/jenkins/build.sh script.
    """
    if os.path.split(os.getcwd())[-1] != 'kubernetes':
        print >>sys.stderr, (
            'Scenario should only run from either kubernetes directory!')
        sys.exit(1)

    # pre-check if target build exists in gcs bucket or not
    # if so, don't make duplicated builds
    if check_build_exists(args.release, args.suffix, args.fast):
        print >>sys.stderr, 'build already exists, exit'
        sys.exit(0)

    env = {
        # Skip gcloud update checking; do we still need this?
        'CLOUDSDK_COMPONENT_MANAGER_DISABLE_UPDATE_CHECK': 'true',
        # Don't run any unit/integration tests when building
        'KUBE_RELEASE_RUN_TESTS': 'n',
    }
    push_build_args = ['--nomock', '--verbose', '--ci']
    if args.suffix:
        push_build_args.append('--gcs-suffix=%s' % args.suffix)
    if args.release:
        push_build_args.append('--bucket=%s' % args.release)
    if args.registry:
        push_build_args.append('--docker-registry=%s' % args.registry)
    if args.extra_publish_file:
        push_build_args.append('--extra-publish-file=%s' % args.extra_publish_file)
    if args.extra_version_markers:
        push_build_args.append('--extra-version-markers=%s' % args.extra_version_markers)
    if args.fast:
        push_build_args.append('--fast')
    if args.allow_dup:
        push_build_args.append('--allow-dup')
    if args.skip_update_latest:
        push_build_args.append('--noupdatelatest')
    if args.register_gcloud_helper:
        # Configure docker client for gcr.io authentication to allow communication
        # with non-public registries.
        check_no_stdout('gcloud', 'auth', 'configure-docker')

    for key, value in env.items():
        os.environ[key] = value
    check('make', 'clean')
    if args.fast:
        check('make', 'quick-release')
    else:
        check('make', 'release')
    output = check_output(args.push_build_script, *push_build_args)
    print >>sys.stderr, 'Push build result: ', output

if __name__ == '__main__':
    PARSER = argparse.ArgumentParser(
        'Build and push.')
    PARSER.add_argument(
        '--release', help='Upload binaries to the specified gs:// path')
    PARSER.add_argument(
        '--suffix', help='Append suffix to the upload path if set')
    PARSER.add_argument(
        '--registry', help='Push images to the specified docker registry')
    PARSER.add_argument(
        '--extra-publish-file', help='Additional version file uploads to')
    PARSER.add_argument(
        '--extra-version-markers', help='Additional version file uploads to')
    PARSER.add_argument(
        '--fast', action='store_true', help='Specifies a fast build')
    PARSER.add_argument(
        '--allow-dup', action='store_true', help='Allow overwriting if the build exists on gcs')
    PARSER.add_argument(
        '--skip-update-latest', action='store_true', help='Do not update the latest file')
    PARSER.add_argument(
        '--push-build-script', default='../release/push-build.sh', help='location of push-build.sh')
    PARSER.add_argument(
        '--register-gcloud-helper', action='store_true',
        help='Register gcloud as docker credentials helper')
    ARGS = PARSER.parse_args()
    main(ARGS)
