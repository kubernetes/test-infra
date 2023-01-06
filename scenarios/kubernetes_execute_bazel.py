#!/usr/bin/env python

# Copyright 2018 The Kubernetes Authors.
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

"""Executes a command, afterwards executes coalesce.py, preserving the return code.

Also supports configuring bazel remote caching."""

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

def call(*cmd):
    """Log and run the command, raising on errors."""
    print >>sys.stderr, 'Run:', cmd
    return subprocess.call(cmd)


def main(cmd):
    """Run script and preserve return code after running coalesce.py."""
    if not cmd:
        raise ValueError(cmd)
    # update bazel caching configuration if enabled
    # TODO(fejta): migrate all jobs to use RBE instead of this
    if os.environ.get('BAZEL_REMOTE_CACHE_ENABLED', 'false') == 'true':
        print 'Bazel remote cache is enabled, generating .bazelrcs ...'
        # TODO: consider moving this once we've migrated all users
        # of the remote cache to this script
        check(test_infra('images/bootstrap/create_bazel_cache_rcs.sh'))
    # call the user supplied command
    return_code = call(*cmd)
    # Coalesce test results into one file for upload.
    check(test_infra('hack/coalesce.py'))
    # preserve the exit code
    sys.exit(return_code)


if __name__ == '__main__':
    PARSER = argparse.ArgumentParser()
    PARSER.add_argument('cmd', nargs=1)
    PARSER.add_argument('args', nargs='*')
    ARGS = PARSER.parse_args()
    main(ARGS.cmd + ARGS.args)
