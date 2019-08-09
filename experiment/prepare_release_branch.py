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


TEST_CONFIG_YAML = "experiment/test_config.yaml"
JOB_CONFIG = "config/jobs"
PROW_CONFIG = "prow/config.yaml"
BRANCH_JOB_DIR = "config/jobs/kubernetes/sig-release/release-branch-jobs"
SECURITY_JOBS = "config/jobs/kubernetes-security/generated-security-jobs.yaml"


class ToolError(Exception):
    pass


def check_version(branch_path):
    files = glob.glob(os.path.join(branch_path, "*.yaml"))
    if len(files) != 4:
        raise ToolError("Expected exactly four yaml files in " + branch_path)
    basenames = [os.path.splitext(os.path.basename(x))[0] for x in files]
    numbers = sorted([list(map(int, x.split('.'))) for x in basenames])
    lowest = numbers[0]
    for i, num in enumerate(numbers):
        if num[1] != lowest[1] + i:
            raise ToolError("Branches are not sequential.")
    return numbers[-1]


def delete_dead_branch(branch_path, current_version):
    filename = '%d.%d.yaml' % (current_version[0], current_version[1] - 3)
    os.unlink(os.path.join(branch_path, filename))


def rotate_files(rotator_bin, branch_path, current_version):
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
    next_version = (current_version[0], current_version[1] + 1)
    filename = '%d.%d.yaml' % (next_version[0], next_version[1])
    sh.Command(forker_bin)(
        job_config=os.path.abspath(prowjob_path),
        output=os.path.abspath(os.path.join(branch_path, filename)),
        version='%d.%d' % next_version,
        _fg=True)


def update_generated_config(path, latest_version):
    with open(path, 'r') as f:
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

    with open(path, 'w') as f:
        yaml.round_trip_dump(config, f)


def regenerate_files(generate_tests_bin, generate_security_bin, test_config,
                     prow_config, job_dir, security_config):
    sh.Command(generate_tests_bin)(
        yaml_config_path=test_config,
        _fg=True)
    sh.Command(generate_security_bin)(
        config=prow_config,
        jobs=job_dir,
        output=security_config,
        _fg=True)


def main():
    if not os.environ.get('BUILD_WORKSPACE_DIRECTORY'):
        print("Please run me via bazel!")
        print("bazel run //experiment:prepare_release_branch")
        sys.exit(1)
    rotator_bin = sys.argv[1]
    forker_bin = sys.argv[2]
    generate_tests_bin = sys.argv[3]
    generate_security_bin = sys.argv[4]
    d = os.environ.get('BUILD_WORKSPACE_DIRECTORY')
    version = check_version(os.path.join(d, BRANCH_JOB_DIR))
    print("Current version: %d.%d" % (version[0], version[1]))
    print("Deleting dead branch...")
    delete_dead_branch(os.path.join(d, BRANCH_JOB_DIR), version)
    print("Rotating files...")
    rotate_files(rotator_bin, os.path.join(d, BRANCH_JOB_DIR), version)
    print("Forking new file...")
    fork_new_file(forker_bin, os.path.join(d, BRANCH_JOB_DIR),
                  os.path.join(d, JOB_CONFIG), version)
    print("Updating test_config.yaml...")
    update_generated_config(os.path.join(d, TEST_CONFIG_YAML), version)
    print("Regenerating files...")
    regenerate_files(generate_tests_bin, generate_security_bin,
                     os.path.join(d, TEST_CONFIG_YAML),
                     os.path.join(d, PROW_CONFIG),
                     os.path.join(d, JOB_CONFIG),
                     os.path.join(d, SECURITY_JOBS))


if __name__ == "__main__":
    main()
