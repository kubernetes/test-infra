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

"""Garbage collect unused .env files."""

import argparse
import os
import json

ORIG_CWD = os.getcwd()  # Checkout changes cwd

def test_infra(*paths):
    """Return path relative to root of test-infra repo."""
    return os.path.join(ORIG_CWD, os.path.dirname(__file__), '..', *paths)

def find_orphans():
    with open(test_infra('jobs/config.json'), 'r') as fp:
        configs = json.loads(fp.read())

    used = {}
    unused = []
    for _, config in configs.items():
        if config['scenario'] == 'execute' or 'args' not in config:
            continue
        for arg in config['args']:
            if (arg.startswith('--env-file=') or
                    arg.startswith('--properties=')):
                used[arg.split('=')[1]] = True

    basepath = test_infra()
    for root, _, files in os.walk(test_infra('jobs')):
        for name in files:
            if name.endswith('.env'):
                path = os.path.join(root, name).replace(basepath + '/', '', 1)
                if path not in used:
                    unused.append(path)

    return unused


def deep_unlink(path):
    """Bazel symlinks to the git client, try readlink() before the unlink()."""
    try:
        under = os.path.join(os.path.dirname(path), os.readlink(path))
        os.unlink(under)
    except OSError:
        pass
    os.unlink(path)

def unlink_orphans():
    orphans = find_orphans()
    for path in orphans:
        print "Deleting unused .env file: {}".format(path)
        deep_unlink(test_infra(path))
    if orphans:
        print ('\nNote: If this is a git tree, ' +
               'use "git add -u" to stage orphan deletions')

if __name__ == '__main__':
    PARSER = argparse.ArgumentParser(
        description='Delete jobs/*.env files not in jobs/config.json')
    ARGS = PARSER.parse_args()

    unlink_orphans()
