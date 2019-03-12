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

""" This script maps prow jobs to user readable testgrid tab names
    Run this script when set up jobs for new releases
    usage:
    1. update the replace mapping in MAP
    2. bazel run //experiment:fix_testgrid_config
    3. if some testgroup name doesn't make sense, just manually edit the change.
"""

import argparse
import ruamel.yaml as yaml

MAP = {
    "-beta": "-1.13",
    "-stable1": "-1.12",
    "-stable2": "-1.11",
    "-stable3": "-1.10",

    "-k8sbeta": "-1.13",
    "-k8sstable1": "-1.12",
    "-k8sstable2": "-1.11",
    "-k8sstable3": "-1.10",

    "-1-12": "-1.12",
    "-1-11": "-1.11",
    "-1-10": "-1.10",
    "-1-9": "-1.9",

    "ci-cri-": "",
    "ci-kubernetes-": "",
    "e2e-": "",
    "periodic-kubernetes-": "periodic-",
}

DASHBOARD_PREFIX = [
    "google-aws",
    "google-gce",
    "google-gke",
    "google-unit",
    "sig-cluster-lifecycle-all",
    "sig-cluster-lifecycle-kops",
    "sig-cluster-lifecycle-upgrade-skew",
    "sig-gcp-release-1.",
    "sig-instrumentation",
    "sig-release-1.",
    "sig-network-gce",
    "sig-network-gke",
    "sig-node-cri-1.",
    "sig-node-kubelet",
    "sig-release-master-upgrade",
    "sig-scalability-gce",
]


def main(testgrid):
    """/shrug."""

    with open(testgrid) as fp:
        config = yaml.round_trip_load(fp)

    for dashboard in config['dashboards']:
        if any(prefix in dashboard['name'] for prefix in DASHBOARD_PREFIX):
            for tab in dashboard['dashboard_tab']:
                name = tab['test_group_name']
                for key, val in MAP.iteritems():
                    name = name.replace(key, val)
                tab['name'] = name

    # write out yaml
    with open(testgrid, 'w') as fp:
        yaml.dump(
            config, fp, Dumper=yaml.RoundTripDumper, width=float("inf"))
        fp.write('\n')

if __name__ == '__main__':
    PARSER = argparse.ArgumentParser(
        description='Hack testgrid configs')
    PARSER.add_argument(
        '--testgrid-config',
        default='./testgrid/config.yaml',
        help='Path to testgrid/config.yaml')
    ARGS = PARSER.parse_args()

    main(ARGS.testgrid_config)
