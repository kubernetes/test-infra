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

"""
Output jenkins job names from 'jobs_in_interest' list
"""

import os
import yaml

def loadconfig(path, suffix):
    """Load config from path with suffix, and print all the job names."""
    with open(os.path.join(os.path.dirname(__file__), path)) as fp:
        doc = yaml.safe_load(fp)

    project = None
    for item in doc:
        if not isinstance(item, dict):
            continue
        if isinstance(item.get('project'), dict):
            project = item['project']
            break
    if not project:
        raise ValueError('project not found')

    jobs = project.get(suffix)
    if not jobs or not isinstance(jobs, list):
        raise ValueError('Could not find %s list in %s' % (suffix, project))

    for job in jobs:
        # Things to check on all bootstrap jobs
        if not isinstance(job, dict):
            raise ValueError('suffix items should be dicts', jobs)
        name = job.keys()[0]
        print job[name].get('job-name')


def main():
    """Load a list of jenkins job config files."""
    jobs_in_interest = {
        'job-configs/kubernetes-jenkins/bootstrap-maintenance-ci.yaml' : 'suffix',
        'job-configs/kubernetes-jenkins/bootstrap-ci.yaml' : 'suffix',
        'job-configs/kubernetes-jenkins/bootstrap-ci-commit.yaml' : 'commit-suffix',
        'job-configs/kubernetes-jenkins/bootstrap-ci-repo.yaml' : 'repo-suffix',
        'job-configs/kubernetes-jenkins/bootstrap-ci-soak.yaml' : 'soak-suffix',
        'job-configs/kubernetes-jenkins/bootstrap-ci-dockerpush.yaml' : 'dockerpush-suffix'
    }

    for path, suffix in jobs_in_interest.iteritems():
        loadconfig(path, suffix)


if __name__ == '__main__':
    main()
