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

"""Executes a command."""

import argparse
import os
import subprocess
import sys

def check_with_log(*cmd):
    """Log and run the command, raising on errors."""
    print >>sys.stderr, 'Run:', cmd
    print >>sys.stderr, subprocess.check_call(cmd)

def check_no_log(*cmd):
    """Run the command, raising on errors, no logs"""
    try:
        subprocess.check_call(cmd)
    except:
        raise subprocess.CalledProcessError(cmd='subprocess.check_call', returncode=1)

def check_output(*cmd):
    """Log and run the command, return output, raising on errors."""
    print >>sys.stderr, 'Run:', cmd
    return subprocess.check_output(cmd)


def main(target, buildfile):
    """Build & push to canary."""
    check_with_log(
      'docker', 'build', '-t', target, '--no-cache=true',
      '--pull=true', '--file=%s' % buildfile, '.'
    )
    check_with_log('docker', 'inspect', target)

    user = None
    if os.path.exists(os.environ.get('DOCKER_USER')):
        with open(os.environ.get('DOCKER_USER'), 'r') as content_file:
            user = content_file.read()

    pwd = None
    if os.path.exists(os.environ.get('DOCKER_PASSWORD')):
        with open(os.environ.get('DOCKER_PASSWORD'), 'r') as content_file:
            pwd = content_file.read()

    if not user or not pwd:
        print >>sys.stderr, 'Logging info not exist'
        sys.exit(1)
    print >>sys.stderr, 'Logging in as %r' % user
    check_no_log('docker', 'login', '--username=%s' % user, '--password=%s' % pwd)

    os.environ.pop('DOCKER_USER', None)
    os.environ.pop('DOCKER_PASSWORD', None)

    check_with_log('docker', 'push', target)
    check_with_log('docker', 'logout')


if __name__ == '__main__':
    PARSER = argparse.ArgumentParser()
    PARSER.add_argument(
        '--owner', help='Owner of the job')
    PARSER.add_argument(
        '--target', help='Build target')
    PARSER.add_argument(
        '--file', help='Build files')
    ARGS = PARSER.parse_args()
    if not ARGS.target or not ARGS.file:
        raise ValueError('--target and --file must be set!')
    if ARGS.owner:
        os.environ['OWNER'] = ARGS.owner
    main(ARGS.target, ARGS.file)
