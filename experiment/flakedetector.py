#!/usr/bin/env python3

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


"""Counts the number of flakes in PRs using data from prow.

A flake is counted if a job passes and fails for the same pull commit. This
isn't a perfect signal, since something might have happened on master that
makes it flake, but I think it's good enough.

There will also be false negatives: flakes that don't show up here because the
PR author changed the PR. Still, this is a good signal.
"""

import operator

import requests


r = requests.get('https://prow.k8s.io/data.js')
data = r.json()

jobs = {}
for job in data:
    if job['type'] != 'pr':
        continue
    if job['repo'] != 'kubernetes/kubernetes':
        continue
    if job['state'] != 'success' and job['state'] != 'failure':
        continue
    if job['job'] not in jobs:
        jobs[job['job']] = {}
    if job['pull_sha'] not in jobs[job['job']]:
        jobs[job['job']][job['pull_sha']] = []
    jobs[job['job']][job['pull_sha']].append(job['state'])

job_flakes = {}
for job, commits in jobs.items():
    job_flakes[job] = 0
    for results in commits.values():
        if 'success' in results and 'failure' in results:
            job_flakes[job] += 1

print('Certain flakes from the last day:')
for job, flakes in sorted(job_flakes.items(), key=operator.itemgetter(1), reverse=True):
    print('{}\t{}'.format(flakes, job))
