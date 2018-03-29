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

# Need to figure out why this only fails on travis
# pylint: disable=bad-continuation

"""Prepares for nested docker, and executes a command."""
# TODO(Q-Lee): check the necessity of this once MountPropagation is available in
# prow: https://github.com/kubernetes/kubernetes/pull/59252

import argparse
import os
import subprocess
import sys

def check(*cmd):
    """Log and run the command, raising on errors."""
    print >>sys.stderr, 'Run:', cmd
    subprocess.check_call(cmd)


def main(envs, cmd):
    """Make important mounts r-shared, then run script and verify it exits 0."""
    check("mount", "--make-rshared", "/lib/modules")
    check("mount", "--make-rshared", "/sys")
    check("mount", "--make-rshared", "/")

    for env in envs:
        key, val = env.split('=', 1)
        print >>sys.stderr, '%s=%s' % (key, val)
        os.environ[key] = val
    if not cmd:
        raise ValueError(cmd)
    check(*cmd)


if __name__ == '__main__':
    PARSER = argparse.ArgumentParser()
    PARSER.add_argument('--env', default=[], action='append')
    PARSER.add_argument('cmd', nargs=1)
    PARSER.add_argument('args', nargs='*')
    ARGS = PARSER.parse_args()
    main(ARGS.env, ARGS.cmd + ARGS.args)
