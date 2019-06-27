#!/usr/bin/env python

# Copyright 2019 The Kubernetes Authors.
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

# pylint can't figure out sh.bazel
# pylint: disable=no-member

import os
import sys
import glob
import re

import sh
import ruamel.yaml as yaml


TEST_CONFIG_YAML = "experiment/test_config.yaml"
JOB_CONFIG = "config/jobs"
BRANCH_JOB_DIR = "config/jobs/kubernetes/sig-release/release-branch-jobs"


class ToolError(Exception):
    pass


def check_version(branch_path):
    files = glob.glob(os.path.join(branch_path, "*.yaml"))
    if len(files) != 4:
        raise ToolError("Expected exactly four yaml files in " + branch_path)
    basenames = [os.path.splitext(os.path.basename(x))[0] for x in files]
    numbers = sorted([map(int, x.split('.')) for x in basenames])
    lowest = numbers[0]
    for i, num in enumerate(numbers):
        if num[1] != lowest[1] + i:
            raise ToolError("Branches are not sequential.")
    return numbers[-1]


def delete_dead_branch(branch_path, current_version):
    filename = '%d.%d.yaml' % (current_version[0], current_version[1] - 3)
    os.unlink(os.path.join(branch_path, filename))


def rotate_files(branch_path, current_version):
    suffixes = ['beta', 'stable1', 'stable2', 'stable3']
    for i in xrange(0, 3):
        filename = '%d.%d.yaml' % (current_version[0], current_version[1] - i)
        from_suffix = suffixes[i]
        to_suffix = suffixes[i+1]
        sh.bazel.run("//experiment/config-rotator", "--",
                     old=from_suffix,
                     new=to_suffix,
                     config_file=os.path.join(branch_path, filename),
                     _fg=True)


def fork_new_file(branch_path, prowjob_path, current_version):
    next_version = (current_version[0], current_version[1] + 1)
    filename = '%d.%d.yaml' % (next_version[0], next_version[1])
    sh.bazel.run("//experiment/config-forker", "--",
                 job_config=os.path.abspath(prowjob_path),
                 output=os.path.abspath(os.path.join(branch_path, filename)),
                 version='%d.%d' % next_version,
                 _fg=True)


def update_generated_config(latest_version):
    with open(TEST_CONFIG_YAML, 'r') as f:
        config = yaml.round_trip_load(f)

    v = latest_version
    suffixes = ['beta', 'stable1', 'stable2', 'stable3']
    for i, s in enumerate(suffixes):
        vs = "%d.%d" % (v[0], v[1] + 1 - i)
        config['k8sVersions'][s]['version'] = vs
        node = config['nodeK8sVersions'][s]
        for j, arg in enumerate(node['args']):
            node['args'][j] = re.sub(
                r'release-\d+\.\d+', 'release-%s' % vs, arg)
        node['prowImage'] = node['prowImage'].rpartition('-')[0] + '-' + vs

    with open(TEST_CONFIG_YAML, 'w') as f:
        yaml.round_trip_dump(config, f)


def regenerate_files():
    sh.bazel.run("//experiment:generate_tests", "--",
                 yaml_config_path=TEST_CONFIG_YAML,
                 _fg=True)
    sh.bazel.run("//hack:update-config", _fg=True)


def main():
    if os.environ.get('BUILD_WORKSPACE_DIRECTORY'):
        os.chdir(os.environ.get('BUILD_WORKSPACE_DIRECTORY'))
    else:
        print("Please run me via bazel!")
        print("bazel run //experiment:prepare_release_branch")
        sys.exit(1)
    version = check_version(BRANCH_JOB_DIR)
    print("Current version: %d.%d" % (version[0], version[1]))
    print("Deleting dead branch...")
    delete_dead_branch(BRANCH_JOB_DIR, version)
    print("Rotating files...")
    rotate_files(BRANCH_JOB_DIR, version)
    print("Forking new file...")
    fork_new_file(BRANCH_JOB_DIR, JOB_CONFIG, version)
    print("Updating test_config.yaml...")
    update_generated_config(version)
    print("Regenerating files...")
    regenerate_files()


if __name__ == "__main__":
    main()
