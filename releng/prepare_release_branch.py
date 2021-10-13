#!/usr/bin/env python3

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


TEST_CONFIG_YAML = "releng/test_config.yaml"
JOB_CONFIG = "config/jobs"
BRANCH_JOB_DIR = "config/jobs/kubernetes/sig-release/release-branch-jobs"

max_config_count = 4
min_config_count = 3

class ToolError(Exception):
    pass

def get_config_files(branch_path):
    print("Retrieving config files...")
    files = glob.glob(os.path.join(branch_path, "*.yaml"))
    return files

def has_correct_amount_of_configs(branch_path):
    print("Checking config count...")
    files = get_config_files(branch_path)
    if len(files) < min_config_count or len(files) > max_config_count:
        print(
            "Expected between %s and %s yaml files in %s, but found %s" % (
                min_config_count,
                max_config_count,
                branch_path,
                len(files),
                )
            )
        return False
    return True

def check_version(branch_path):
    print("Checking latest version...")
    files = get_config_files(branch_path)
    if not has_correct_amount_of_configs(branch_path):
        raise ToolError("Incorrect release config count. Cannot continue.")
    basenames = [os.path.splitext(os.path.basename(x))[0] for x in files]
    numbers = sorted([list(map(int, x.split('.'))) for x in basenames])
    lowest = numbers[0]
    for i, num in enumerate(numbers):
        if num[1] != lowest[1] + i:
            raise ToolError("Branches are not sequential.")
    return numbers[-1]


def delete_stale_branch(branch_path, current_version):
    print("Deleting stale branch...")
    filename = '%d.%d.yaml' % (current_version[0], current_version[1] - 3)
    filepath = os.path.join(branch_path, filename)

    if os.path.exists(filepath):
        os.unlink(filepath)
    else:
        print("the branch config (%s) does not exist" % filename)


def rotate_files(rotator_bin, branch_path, current_version):
    print("Rotating files...")
    suffixes = ['beta', 'stable1', 'stable2', 'stable3']
    for i in range(0, 3):
        filename = '%d.%d.yaml' % (current_version[0], current_version[1] - i)
        from_suffix = suffixes[i]
        to_suffix = suffixes[i+1]
        sh.Command(rotator_bin)(
            old=from_suffix,
            new=to_suffix,
            config_file=os.path.join(branch_path, filename),
            _fg=True)


def fork_new_file(forker_bin, branch_path, prowjob_path, current_version):
    print("Forking new file...")
    next_version = (current_version[0], current_version[1] + 1)
    filename = '%d.%d.yaml' % (next_version[0], next_version[1])
    sh.Command(forker_bin)(
        job_config=os.path.abspath(prowjob_path),
        output=os.path.abspath(os.path.join(branch_path, filename)),
        version='%d.%d' % next_version,
        _fg=True)


def update_generated_config(path, latest_version):
    print("Updating test_config.yaml...")
    with open(path, 'r') as f:
        config = yaml.round_trip_load(f)

    v = latest_version
    suffixes = ['beta', 'stable1', 'stable2', 'stable3']
    for i, s in enumerate(suffixes):
        vs = "%d.%d" % (v[0], v[1] + 1 - i)
        markers = config['k8sVersions'][s]
        markers['version'] = vs
        for j, arg in enumerate(markers['args']):
            markers['args'][j] = re.sub(
                r'latest(-\d+\.\d+)?', 'latest-%s' % vs, arg)

        node = config['nodeK8sVersions'][s]
        for k, arg in enumerate(node['args']):
            node['args'][k] = re.sub(
                r'master|release-\d+\.\d+', 'release-%s' % vs, arg)
        node['prowImage'] = node['prowImage'].rpartition('-')[0] + '-' + vs

    with open(path, 'w') as f:
        yaml.round_trip_dump(config, f)


def regenerate_files(generate_tests_bin, test_config):
    print("Regenerating files...")
    sh.Command(generate_tests_bin)(
        yaml_config_path=test_config,
        _fg=True)


def main():
    if not os.environ.get('BUILD_WORKSPACE_DIRECTORY'):
        print("Please run me via bazel!")
        print("bazel run //releng:prepare_release_branch")
        sys.exit(1)
    rotator_bin = sys.argv[1]
    forker_bin = sys.argv[2]
    generate_tests_bin = sys.argv[3]
    d = os.environ.get('BUILD_WORKSPACE_DIRECTORY')

    branch_path = os.path.join(d, BRANCH_JOB_DIR)

    version = check_version(branch_path)
    print("Current version: %d.%d" % (version[0], version[1]))

    files = get_config_files(branch_path)
    if len(files) == 4:
        print("There should be a maximum of %s release branch configs." % max_config_count)
        print("Deleting the oldest config before rotation...")

        delete_stale_branch(branch_path, version)

    rotate_files(rotator_bin, branch_path, version)

    fork_new_file(forker_bin, branch_path,
                  os.path.join(d, JOB_CONFIG), version)

    update_generated_config(os.path.join(d, TEST_CONFIG_YAML), version)

    regenerate_files(generate_tests_bin, os.path.join(d, TEST_CONFIG_YAML))


if __name__ == "__main__":
    main()
