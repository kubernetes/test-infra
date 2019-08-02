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
makes it flake, but I think it's good enough. There will also be false
negatives when flakes don't show up because the PR author changed the PR.
Still, this is a good signal.

Only serial jobs are considered for the flake calculation, batch jobs are
ignored.

"""

import operator

import requests

def main(): # pylint: disable=too-many-branches
    """Run flake detector."""
    res = requests.get('https://prow.k8s.io/data.js')
    job_results = res.json()

    jobs = {} # job -> {sha -> [results...]}
    commits = {} # sha -> {job -> [results...]}
    for res in job_results:
        if res['type'] != 'presubmit':
            continue
        if res['repo'] != 'kubernetes/kubernetes':
            continue
        if res['state'] != 'success' and res['state'] != 'failure':
            continue
        # populate jobs
        if res['job'] not in jobs:
            jobs[res['job']] = {}
        if res['pull_sha'] not in jobs[res['job']]:
            jobs[res['job']][res['pull_sha']] = []
        jobs[res['job']][res['pull_sha']].append(res['state'])
        # populate commits
        if res['pull_sha'] not in commits:
            commits[res['pull_sha']] = {}
        if res['job'] not in commits[res['pull_sha']]:
            commits[res['pull_sha']][res['job']] = []
        commits[res['pull_sha']][res['job']].append(res['state'])

    job_commits = {}
    job_flakes = {}
    for job, shas in jobs.items():
        job_commits[job] = len(shas)
        job_flakes[job] = 0
        for results in shas.values():
            if 'success' in results and 'failure' in results:
                job_flakes[job] += 1

    print('Certain flakes:')
    for job, flakes in sorted(job_flakes.items(), key=operator.itemgetter(1), reverse=True):
        if job_commits[job] < 10:
            continue
        fail_chance = flakes / job_commits[job]
        print('{}/{}\t({:.0f}%)\t{}'.format(flakes, job_commits[job], 100*fail_chance, job))

    # for each commit, flaked iff exists job that flaked
    flaked = 0
    for _, job_results in commits.items():
        for job, results in job_results.items():
            if 'success' in results and 'failure' in results:
                flaked += 1
                break
    print('Commits that flaked (passed and failed some job): %d/%d  %.2f%%' %
          (flaked, len(commits), (flaked*100.0)/len(commits)))


if __name__ == '__main__':
    main()
