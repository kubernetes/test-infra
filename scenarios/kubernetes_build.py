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
import subprocess
import sys


def check(*cmd):
    """Log and run the command, raising on errors."""
    print >>sys.stderr, 'Run:', cmd
    subprocess.check_call(cmd)


def main(args):
    """Build kubernetes."""
    env = {}
    if args.kops:
        env['GCS_LOCATION'] = args.kops
    if args.unstable:
        env['DEB_CHANNEL'] = 'unstable'
    if args.suffix:
        env['KUBE_GCS_RELEASE_SUFFIX'] = args.suffix
    env['KUBE_FASTBUILD'] = 'true' if args.fast else 'false'
    if args.federation:
        env['PROJECT'] = args.federation
        env['FEDERATION'] = 'true'
        env['FEDERATION_PUSH_REPO_BASE'] = 'gcr.io/%s' % args.federation
    if args.release:
        env['KUBE_GCS_RELEASE_BUCKET'] = args.release
        env['KUBE_GCS_RELEASE_BUCKET_MIRROR'] = args.release


    for key, value in env.items():
        os.environ[key] = value

    if args.script == 'make gcs-publish-ci':
        # TODO(fejta): fix this hack
        check('make', 'gcs-publish-ci')
    else:
        check(args.script)


if __name__ == '__main__':
    PARSER = argparse.ArgumentParser(
        'Runs verification checks on the kubernetes repo')
    PARSER.add_argument(
        '--script',
        default='./hack/jenkins/build.sh',
        help='Script relative to repo which builds')
    PARSER.add_argument('--fast', action='store_true', help='Build quickly')
    PARSER.add_argument(
        '--release', help='Upload binaries to the specified gs:// path')
    PARSER.add_argument(
        '--suffix', help='Append suffix to the upload path if set')
    PARSER.add_argument(
        '--unstable',
        action='store_true',
        help='Use the unstable debian channel')
    PARSER.add_argument(
        '--federation',
        help='Enable federation with the specified project')
    PARSER.add_argument(
        '--kops', help='Upload kops to the specified gs:// path')
    ARGS = PARSER.parse_args()
    main(ARGS)
