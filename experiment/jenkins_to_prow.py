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

"""Convert Jenkins config into prow config. """

import argparse
import copy
import fileinput
import json
import subprocess
import sys
import yaml


TEMPLATE = {
    'name': '',
    'interval': '2h',
    'spec': {
        'containers': [{
            'image': 'gcr.io/k8s-testimages/kubekins-e2e-prow:v20170606-e69a3df0',
            'args': [],
            'volumeMounts': [{
                'readOnly': True,
                'mountPath': '/etc/service-account',
                'name': 'service'
            }, {
                'readOnly': True,
                'mountPath': '/etc/ssh-key-secret',
                'name': 'ssh'
            }],
            'env': [{
                'name': 'GOOGLE_APPLICATION_CREDENTIALS',
                'value': '/etc/service-account/service-account.json'
            }, {
                'name': 'USER',
                'value': 'prow'
            }, {
                'name': 'JENKINS_GCE_SSH_PRIVATE_KEY_FILE',
                'value': '/etc/ssh-key-secret/ssh-private'
            }, {
                'name': 'JENKINS_GCE_SSH_PUBLIC_KEY_FILE',
                'value': '/etc/ssh-key-secret/ssh-public'
            }]
        }],
        'volumes': [{
            'secret': {
                'secretName': 'service-account'
            },
            'name': 'service'
        }, {
            'secret': {
                'defaultMode': 256,
                'secretName': 'ssh-key-secret'
            },
            'name': 'ssh'
        }]
    }
}

# pylint: disable=too-many-branches,too-many-statements,too-many-locals

def main(job, jenkins_path, suffix, prow_path, config_path, delete):
    """Convert Jenkins config to prow config."""
    with open(jenkins_path) as fp:
        doc = yaml.safe_load(fp)

    project = None
    for item in doc:
        if not isinstance(item, dict):
            continue
        if not isinstance(item.get('project'), dict):
            continue
        project = item['project']
        break
    else:
        raise ValueError('Cannot find any project from %r', jenkins_path)

    jenkins_jobs = project.get(suffix)
    dump = []
    job_names = []
    for jenkins_job in jenkins_jobs:
        name = jenkins_job.keys()[0]
        real_job = jenkins_job[name]
        if job in real_job['job-name']:
            output = copy.deepcopy(TEMPLATE)
            output['name'] = real_job['job-name']
            args = output['spec']['containers'][0]['args']
            if 'timeout' in real_job:
                args.append('--timeout=%s' % real_job['timeout'])
            if 'repo-name' not in real_job and 'branch' not in real_job:
                args.append('--bare')
            else:
                if 'repo-name' in real_job:
                    args.append('--repo=%s' % real_job['repo-name'])
                if 'branch' in real_job:
                    args.append('--branch=%s' % real_job['branch'])
            dump.append(output)
            job_names.append(real_job['job-name'])

    if prow_path:
        with open(prow_path, 'a') as fp:
            fp.write('\n')
            yaml.safe_dump(dump, fp, default_flow_style=False)
    else:
        print yaml.safe_dump(dump, default_flow_style=False)

    # delete jenkins config, try to keep format & comments
    if delete:
        deleting = False
        for line in fileinput.input(jenkins_path, inplace=True):
            if line.strip().startswith('-'):
                deleting = job in line.strip()

            if not deleting:
                sys.stdout.write(line)

    # add mode=local to config.json
    if config_path:
        with open(config_path, 'r+') as fp:
            configs = json.loads(fp.read())
            for jobn in job_names:
                if jobn in configs:
                    configs[jobn]['args'].append('--mode=local')
            fp.seek(0)
            fp.write(json.dumps(configs, sort_keys=True, indent=2))
            fp.write('\n')
            fp.truncate()

    for old_name in job_names:
        if '.' in old_name:
            new_name = old_name.replace('.', '-')
            files = ['jobs/config.json', 'testgrid/config/config.yaml', 'prow/config.yaml']
            for fname in files:
                with open(fname) as fp:
                    content = fp.read()
                content = content.replace(old_name, new_name)
                with open(fname, "w") as fp:
                    fp.write(content)
            subprocess.check_call(['git', 'mv', 'jobs/%s.env' % old_name, 'jobs/%s.env' % new_name])


if __name__ == '__main__':
    PARSER = argparse.ArgumentParser(
        description='Convert Jenkins config into prow config')
    PARSER.add_argument(
        '--config',
        help='Path to config.json',
        default=None)
    PARSER.add_argument(
        '--delete',
        action='store_true',
        help='If delete Jenkins entry')
    PARSER.add_argument(
        '--job',
        help='Job to convert, empty for all jobs in Jenkins config',
        default='')
    PARSER.add_argument(
        '--jenkins',
        help='Path to Jenkins config',
        required=True)
    PARSER.add_argument(
        '--suffix',
        help='Suffix of a jenkins job',
        default='suffix')
    PARSER.add_argument(
        '--prow',
        help='Path to output prow config, empty for stdout',
        default=None)
    ARGS = PARSER.parse_args()

    main(ARGS.job, ARGS.jenkins, ARGS.suffix, ARGS.prow, ARGS.config, ARGS.delete)
