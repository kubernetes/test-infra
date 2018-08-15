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

"""Updates the Gubernator configuration from the Prow configuration."""

import argparse
import yaml

def main(prow_config, gubernator_config):
    with open(prow_config) as prow_file:
        prow_data = yaml.load(prow_file)

    default_presubmits = []
    for job in prow_data['presubmits']['kubernetes/kubernetes']:
        if job.get('always_run'):
            default_presubmits.append(job['name'])

    with open(gubernator_config) as gubernator_file:
        gubernator_data = yaml.load(gubernator_file)

    gubernator_data['jobs']['kubernetes-jenkins/pr-logs/directory/'] = default_presubmits

    with open(gubernator_config, 'w+') as gubernator_file:
        yaml.dump(gubernator_data, gubernator_file, default_flow_style=False, explicit_start=True)

if __name__ == '__main__':
    PARSER = argparse.ArgumentParser()
    PARSER.add_argument('prow_config', help="Path to Prow configuration YAML.")
    PARSER.add_argument('gubernator_config', help="Path to Gubernator configuration YAML.")
    ARGS = PARSER.parse_args()
    main(ARGS.prow_config, ARGS.gubernator_config)
