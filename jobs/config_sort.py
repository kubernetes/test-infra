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

"""Sort current config.json and prow/config.yaml alphabetically. """

import argparse
import cStringIO
import json
import os

import ruamel.yaml as yaml

ORIG_CWD = os.getcwd()  # Checkout changes cwd

def test_infra(*paths):
    """Return path relative to root of test-infra repo."""
    return os.path.join(ORIG_CWD, os.path.dirname(__file__), '..', *paths)


def sorted_seq(jobs):
    return yaml.comments.CommentedSeq(
        sorted(jobs, key=lambda job: job['name']))


def sorted_map(jobs):
    for key, value in jobs.items():
        jobs[key] = sorted_seq(value)
    return jobs


def sort_job_config():
    """Sort config.json alphabetically."""
    with open(test_infra('jobs/config.json'), 'r+') as fp:
        configs = json.loads(fp.read())
        fp.seek(0)
        fp.write(json.dumps(configs, sort_keys=True, indent=2, separators=(',', ': ')))
        fp.write('\n')
        fp.truncate()


def sorted_boskos_config():
    """Get the sorted boskos configuration."""
    with open(test_infra('boskos/resources.json'), 'r') as fp:
        configs = json.loads(fp.read())
    for rtype in configs:
        rtype["names"] = sorted(rtype["names"])
    output = cStringIO.StringIO()
    json.dump(
        configs, output, sort_keys=True, indent=2, separators=(',', ': '))
    output.write('\n')
    return output

def sort_boskos_config():
    """Sort boskos/resources.json alphabetically."""
    output = sorted_boskos_config()
    with open(test_infra('boskos/resources.json'), 'w+') as fp:
        fp.write(output.getvalue())
    output.close()


def sorted_prow_config():
    """Get the sorted Prow configuration."""
    with open(test_infra('prow/config.yaml'), 'r') as fp:
        configs = yaml.round_trip_load(fp, preserve_quotes=True)
    configs['periodics'] = sorted_seq(configs['periodics'])
    configs['presubmits'] = sorted_map(configs['presubmits'])
    configs['postsubmits'] = sorted_map(configs['postsubmits'])
    output = cStringIO.StringIO()
    yaml.round_trip_dump(
        configs, output, default_flow_style=False, width=float("inf"))
    return output


def sort_prow_config():
    """Sort test jobs in Prow configuration alphabetically."""
    output = sorted_prow_config()
    with open(test_infra('prow/config.yaml'), 'w+') as fp:
        fp.write(output.getvalue())
    output.close()


if __name__ == '__main__':
    PARSER = argparse.ArgumentParser(
        description='Sort config.json and prow/config.yaml alphabetically')
    ARGS = PARSER.parse_args()

    sort_job_config()
    sort_prow_config()
    sort_boskos_config()
