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

import requests

TEST_CONFIG_YAML = "test_config.yaml"
JOB_CONFIG = "../config/jobs"
BRANCH_JOB_DIR = "../config/jobs/kubernetes/sig-release/release-branch-jobs"

max_config_count = 5
min_config_count = 3

suffixes = ['beta', 'stable1', 'stable2', 'stable3', 'stable4']

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
    numbers = []
    for x in basenames:
        # Matching only x.y.yaml file names
        if re.match(r'^\d+\.\d+$', x):
            numbers.append(list(map(int, x.split('.'))))
        else:
            print(f"Skipped invalid basename: '{x}'")  # Log skipped files

    numbers = sorted(numbers)

    if not numbers:
        raise ToolError(f"No valid version numbers found. Check config {branch_path} for valid files.")

    # Check if versions are sequential
    lowest = numbers[0]
    for i, num in enumerate(numbers):
        expected_version = lowest[1] + i  # Calculate the expected minor version
        if num[1] != expected_version:
            print(f"Non-sequential branch found: Expected 1.{expected_version}, but got 1.{num[1]}")
            raise ToolError("Branches are not sequential.")

    print(f"Valid version numbers found: {numbers}")
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
    for i in range(max_config_count - 1):
        filename = '%d.%d.yaml' % (current_version[0], current_version[1] - i)
        from_suffix = suffixes[i]
        to_suffix = suffixes[i+1]
        sh.Command(rotator_bin)(
            old=from_suffix,
            new=to_suffix,
            config_file=os.path.join(branch_path, filename),
            _fg=True)


def fork_new_file(forker_bin, branch_path, prowjob_path, current_version, go_version):
    print("Forking new file...")
    next_version = (current_version[0], current_version[1] + 1)
    filename = '%d.%d.yaml' % (next_version[0], next_version[1])
    sh.Command(forker_bin)(
        job_config=os.path.abspath(prowjob_path),
        output=os.path.abspath(os.path.join(branch_path, filename)),
        version='%d.%d' % next_version,
        go_version=go_version,
        _fg=True)


def update_generated_config(path, latest_version):
    print("Updating test_config.yaml...")
    with open(path, 'r') as f:
        config = yaml.round_trip_load(f)

    v = latest_version
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

def go_version_kubernetes_master():
    resp = requests.get(
        'https://raw.githubusercontent.com/kubernetes/kubernetes/master/.go-version')
    resp.raise_for_status()
    data = resp.content.decode("utf-8")
    return data

def main():
    if os.environ.get('BUILD_WORKSPACE_DIRECTORY'):
        print("Please run me via make rule!")
        print("make -C releng prepare-release-branch")
        sys.exit(1)
    rotator_bin = sys.argv[1]
    forker_bin = sys.argv[2]
    generate_tests_bin = sys.argv[3]

    cur_file_dir = os.path.dirname(__file__)
    branch_job_dir_abs = os.path.join(cur_file_dir, BRANCH_JOB_DIR)

    version = check_version(branch_job_dir_abs)
    print("Current version: %d.%d" % (version[0], version[1]))

    go_version = go_version_kubernetes_master()
    print("Current Go Version: %s" % go_version)

    files = get_config_files(branch_job_dir_abs)
    if len(files) > max_config_count:
        print("There should be a maximum of %s release branch configs." % max_config_count)
        print("Deleting the oldest config before rotation...")

        delete_stale_branch(branch_job_dir_abs, version)

    rotate_files(rotator_bin, branch_job_dir_abs, version)

    fork_new_file(forker_bin, branch_job_dir_abs,
                  os.path.join(cur_file_dir, JOB_CONFIG), version, go_version)

    update_generated_config(os.path.join(cur_file_dir, TEST_CONFIG_YAML), version)

    regenerate_files(generate_tests_bin, os.path.join(cur_file_dir, TEST_CONFIG_YAML))


if __name__ == "__main__":
    main()
