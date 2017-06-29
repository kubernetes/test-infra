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

"""Collect & recycle obsoleted e2e jenkins projects. """

import argparse
import re
import json
import os
import subprocess

def load_projects(configs):
    """Scans the project directories for GCP projects to check."""
    filter_re = re.compile(r'^.+\.(env|sh)$')
    project_re = re.compile('^PROJECT="?([^"\r\n]+)"?$', re.MULTILINE)

    projects = set()

    for dirname, _, files in os.walk(configs):
        for path in files:
            full_path = os.path.join(dirname, path)
            if not filter_re.match(path):
                continue
            with open(full_path) as fp:
                for project in project_re.findall(fp.read()):
                    if '{' not in project:
                        projects.add(project)
                        continue
                    else:
                        raise ValueError(project)
    return projects

def main(args):
    exist = load_projects(args.jobdir)

    jobs = []
    keys = ['jkn', 'e2e', 'upg', '-gci-', 'kubekins-', 'k8s-gke', 'k8s-gce', 'gke-up']
    cmd = ['gcloud', 'alpha', 'projects', 'list', '--format=json']
    for item in json.loads(subprocess.check_output(cmd)):
        if 'projectId' not in item:
            raise ValueError('no projectId field in result???')
        if item['projectId'] in exist:
            continue
        if any(key in item['projectId'] for key in keys):
            job = {}
            job['name'] = item['projectId']
            job['type'] = 'project'
            job['state'] = 'free'
            jobs.append(job)
            print 'added project %s' % job['name']

    with open(args.config, 'r+') as fp:
        fp.seek(0)
        fp.write(json.dumps(jobs, sort_keys=True, indent=2, separators=(',', ': ')))
        fp.write('\n')
        fp.truncate()


if __name__ == '__main__':
    PARSER = argparse.ArgumentParser(
        description='Recycle not in use gcp projects')
    PARSER.add_argument(
        '--config',
        help='Path to boskos resources.json',
        default='boskos/resources.json')
    PARSER.add_argument(
        '--jobdir',
        help='Path to existing job.envs',
        default='jobs/')
    main(PARSER.parse_args())
