#!/usr/bin/env python3

# Copyright 2026 The Kubernetes Authors.
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

"""Generate job_repos.json mapping Prow job names to org/repo."""

import json
import os
import sys

import yaml


def generate_job_repos(jobs_dir):
    """Parse Prow job configs and return a dict mapping job name to org/repo."""
    if not os.path.isdir(jobs_dir):
        raise FileNotFoundError(f'jobs directory not found: {jobs_dir}')

    mapping = {}

    for root, _, files in os.walk(jobs_dir):
        for fname in files:
            if not fname.endswith(('.yaml', '.yml')):
                continue
            filepath = os.path.join(root, fname)
            with open(filepath) as f:
                try:
                    data = yaml.safe_load(f)
                except Exception:
                    continue
                if not data:
                    continue

            # Presubmits: repo is the YAML key (already org/repo format)
            for repo, jobs in (data.get('presubmits') or {}).items():
                for job in (jobs or []):
                    if isinstance(job, dict) and 'name' in job:
                        mapping[job['name']] = repo

            # Postsubmits: same structure as presubmits
            for repo, jobs in (data.get('postsubmits') or {}).items():
                for job in (jobs or []):
                    if isinstance(job, dict) and 'name' in job:
                        mapping[job['name']] = repo

            # Periodics: repo from first extra_refs entry
            for job in (data.get('periodics') or []):
                if not isinstance(job, dict) or 'name' not in job:
                    continue
                extra_refs = job.get('extra_refs', [])
                if extra_refs:
                    ref = extra_refs[0]
                    org = ref.get('org', '')
                    repo = ref.get('repo', '')
                    if org and repo:
                        mapping[job['name']] = f'{org}/{repo}'

    return mapping


def main():
    jobs_dir = sys.argv[1] if len(sys.argv) > 1 else 'config/jobs'
    output = sys.argv[2] if len(sys.argv) > 2 else 'job_repos.json'

    mapping = generate_job_repos(jobs_dir)

    with open(output, 'w') as f:
        json.dump(mapping, f, sort_keys=True)

    print(f'Generated {output} with {len(mapping)} job mappings')


if __name__ == '__main__':
    main()
