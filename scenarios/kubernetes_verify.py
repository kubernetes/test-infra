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

"""Runs verify/test-go checks for kubernetes/kubernetes."""

import argparse
import os
import re
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


def main(branch, script, force):
    """Test branch using script, optionally forcing verify checks."""
    # If branch has 3-part version, only take first 2 parts.
    verify_branch = re.match(r'master|release-(\d+\.\d+)', branch)
    if not verify_branch:
        raise ValueError(branch)
    force = 'y' if force else 'n'
    k8s = os.getcwd()
    if not os.path.basename(k8s) == 'kubernetes':
        raise ValueError(k8s)

    check('mkdir', '-p', 'artifacts')
    check('ln', '-s', 'artifacts', '_artifacts')

    check(
        'bash', '-c',
        'cd /go/src/k8s.io/kubernetes && KUBE_FORCE_VERIFY_CHECKS=%s KUBE_VERIFY_GIT_BRANCH=%s %s' % (force, branch, script),
    )


if __name__ == '__main__':
    PARSER = argparse.ArgumentParser(
        'Runs verification checks on the kubernetes repo')
    PARSER.add_argument(
        '--branch', default='master', help='Upstream target repo')
    PARSER.add_argument(
        '--force', action='store_true', help='Force all verify checks')
    PARSER.add_argument(
        '--script',
        default='./hack/jenkins/test-dockerized.sh',
        help='Script in kubernetes/kubernetes that runs checks')
    ARGS = PARSER.parse_args()
    main(ARGS.branch, ARGS.script, ARGS.force)
