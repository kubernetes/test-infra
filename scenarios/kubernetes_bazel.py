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

"""Runs bazel build/test for current repo."""

import argparse
import os
import subprocess
import sys

ORIG_CWD = os.getcwd()

def test_infra(*paths):
    """Return path relative to root of test-infra repo."""
    return os.path.join(ORIG_CWD, os.path.dirname(__file__), '..', *paths)

def check(*cmd):
    """Log and run the command, raising on errors."""
    print >>sys.stderr, 'Run:', cmd
    subprocess.check_call(cmd)


def echo_result(res):
    """echo error message bazed on value of res"""
    echo_map = {
        0:'Success',
        1:'Build failed',
        2:'Bad environment or flags',
        3:'Build passed, tests failed or timed out',
        4:'Build passed, no tests found',
        5:'Interrupted'
    }
    print echo_map.get(res, 'Unknown exit code : %s' % res)

def get_version():
    """Return kubernetes version"""
    with open('bazel-genfiles/version') as fp:
        return fp.read.strip()

def main(args):
    """Trigger a bazel build/test run, and upload results."""
    check('bazel', 'clean', '--expunge')
    res = 0
    try:
        if args.build:
            check('bazel', 'build', *args.build.split(' '))
        if args.release:
            check('bazel', 'build', *args.release.split(' '))
        if args.test:
            check('bazel', 'test', *args.test.split(' '))
    except subprocess.CalledProcessError as exp:
        res = exp.returncode

    if args.release and res == 0:
        version = get_version()
        if not version:
            print 'Kubernetes version missing; not uploading ci artifacts.'
            res = 1
        else:
            try:
                check(
                    'bazel', 'run', '//:push-build', '--',
                    '%s/%s' % (args.gcs, version)
                    )
            except subprocess.CalledProcessError as exp:
                res = exp.returncode

    # Coalesce test results into one file for upload.
    check(test_infra('images/pull_kubernetes_bazel/coalesce.py'))

    echo_result(res)
    exit(res)

def create_parser():
    """Create argparser."""
    parser = argparse.ArgumentParser()
    parser.add_argument(
        '--build', default=None, help='Bazel build')
    parser.add_argument(
        '--release', default=None, help='Bazel release')
    parser.add_argument(
        '--test', default=None, help='Bazel test')
    parser.add_argument(
        '--gcs',
        default='gs://kubernetes-release-dev/bazel',
        help='GCS path for where to push build')
    return parser

def parse_args(args=None):
    """Return parsed args."""
    parser = create_parser()
    return parser.parse_args(args)

if __name__ == '__main__':
    main(parse_args())
