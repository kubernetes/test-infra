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

"""Runs bigquery metrics and uploads the result to GCS."""

import argparse
import glob
import json
import os
import shlex
import re
import subprocess
import sys
import time
import traceback

import requests
import ruamel.yaml as yaml

BACKFILL_DAYS = 30
DEFAULT_JQ_BIN = '/usr/bin/jq'

# Global variables for job filtering
VALID_JOBS = None  # Set of valid job names, populated if --filter-stale-jobs is used

def check(cmd, **kwargs):
    """Logs and runs the command, raising on errors."""
    print('Run:', ' '.join(shlex.quote(c) for c in cmd), end=' ', file=sys.stderr)
    if hasattr(kwargs.get('stdout'), 'name'):
        print(' > %s' % kwargs['stdout'].name, file=sys.stderr)
    else:
        print()
    # If 'stdin' keyword arg is a string run command and communicate string to stdin
    if 'stdin' in kwargs and isinstance(kwargs['stdin'], str):
        in_string = kwargs['stdin']
        kwargs['stdin'] = subprocess.PIPE
        proc = subprocess.Popen(cmd, **kwargs)
        proc.communicate(input=in_string.encode('utf-8'))
        return
    subprocess.check_call(cmd, **kwargs)


def validate_metric_name(name):
    """Raise ValueError if name is non-trivial."""
    # Regex '$' symbol matches an optional terminating new line
    # so we have to check that the name
    # doesn't have one if the regex matches.
    if not re.match(r'^[\w-]+$', name) or name[-1] == '\n':
        raise ValueError(name)


def do_jq(jq_filter, data_filename, out_filename, jq_bin=DEFAULT_JQ_BIN):
    """Executes jq on a file and outputs the results to a file."""
    with open(out_filename, 'w') as out_file:
        check([jq_bin, jq_filter, data_filename], stdout=out_file)


def _extract_job_names(job_list, prefix=None):
    """Extracts job names from a list of job configs.

    Args:
        job_list: List of job config dicts with 'name' field
        prefix: Optional prefix to add (creates additional entry with prefix)

    Returns:
        Set of job names
    """
    names = set()
    if not isinstance(job_list, list):
        return names
    for job in job_list:
        if isinstance(job, dict) and 'name' in job:
            name = job['name']
            names.add(name)
            if prefix:
                names.add(prefix + name)
    return names


def _extract_jobs_from_config(config):
    """Extracts all job names from a Prow config dict.

    Args:
        config: Parsed YAML config dict

    Returns:
        Set of job names
    """
    jobs = set()
    if not config:
        return jobs

    # Extract presubmit job names (with 'pr:' prefix for metrics)
    # Use 'or' to handle explicit null values in YAML (e.g., "presubmits: null")
    for repo_jobs in (config.get('presubmits') or {}).values():
        jobs.update(_extract_job_names(repo_jobs, prefix='pr:'))

    # Extract postsubmit job names
    for repo_jobs in (config.get('postsubmits') or {}).values():
        jobs.update(_extract_job_names(repo_jobs))

    # Extract periodic job names
    jobs.update(_extract_job_names(config.get('periodics') or []))

    return jobs


def collect_valid_jobs(jobs_dir):
    """Collects all valid job names from Prow job config YAML files.

    Args:
        jobs_dir: Path to the directory containing job configuration files
                  (e.g., config/jobs)

    Returns:
        A set of valid job names. Presubmit jobs are included both with and
        without the 'pr:' prefix to match how they appear in metrics.
    """
    valid_jobs = set()

    if not os.path.isdir(jobs_dir):
        print('Warning: jobs directory does not exist: %s' % jobs_dir, file=sys.stderr)
        return valid_jobs

    # Walk all YAML files in the jobs directory
    file_count = 0
    for root, _, files in os.walk(jobs_dir):
        for filename in files:
            if not filename.endswith('.yaml') and not filename.endswith('.yml'):
                continue

            filepath = os.path.join(root, filename)
            file_count += 1
            try:
                with open(filepath) as f:
                    config = yaml.safe_load(f)
                valid_jobs.update(_extract_jobs_from_config(config))
            except (yaml.YAMLError, OSError, TypeError, AttributeError) as e:
                print('Warning: failed to process %s: %s' % (filepath, e), file=sys.stderr)

    print('Collected %d valid job names from %d files in %s' % (
        len(valid_jobs), file_count, jobs_dir), file=sys.stderr)
    return valid_jobs


def filter_json_by_jobs(filepath, valid_jobs):
    """Filters a JSON file to only include jobs that exist in valid_jobs.

    Handles two JSON formats:
    1. Object with job names as keys: {"job-name": {...}, ...}
       Used by: failures, flakes, flakes-daily
    2. Array with "job" field: [{"job": "name", ...}, ...]
       Used by: job-health, presubmit-health

    Args:
        filepath: Path to the JSON file to filter
        valid_jobs: Set of valid job names

    Returns:
        Tuple of (original_count, filtered_count) for logging
    """
    with open(filepath) as f:
        data = json.load(f)

    original_count = 0
    filtered_count = 0

    if isinstance(data, dict):
        # Object format: filter by keys
        original_count = len(data)
        filtered_data = {k: v for k, v in data.items() if k in valid_jobs}
        filtered_count = len(filtered_data)
    elif isinstance(data, list):
        # Array format: filter by "job" field
        original_count = len(data)
        filtered_data = [item for item in data
                         if isinstance(item, dict) and item.get('job') in valid_jobs]
        filtered_count = len(filtered_data)
    else:
        # Unknown format, don't filter
        print('Warning: unknown JSON format in %s, skipping filter' % filepath,
              file=sys.stderr)
        return (0, 0)

    with open(filepath, 'w') as f:
        json.dump(filtered_data, f)

    return (original_count, filtered_count)


class BigQuerier:
    def __init__(self, project, bucket_path):
        if not project:
            raise ValueError('project', project)
        self.project = project
        if not bucket_path:
            print('Not uploading results, no bucket specified.', file=sys.stderr)
        self.prefix = bucket_path

    def do_query(self, query, out_filename):
        """Executes a bigquery query, outputting the results to a file."""
        cmd = [
            'bq', 'query', '--format=prettyjson',
            '--project_id=%s' % self.project,
            '--max_rows=1000000',  # Results may have more than 100 rows
            query,
        ]
        with open(out_filename, 'w') as out_file:
            check(cmd, stdout=out_file)
            out_file.write('\n')

    def jq_upload(self, config, data_filename):
        """Filters a data file with jq and uploads the results to GCS."""
        filtered = 'daily-%s.json' % time.strftime('%Y-%m-%d')
        latest = '%s-latest.json' % config['metric']
        do_jq(config['jqfilter'], data_filename, filtered)

        # Optionally filter out stale jobs that no longer exist in config
        if VALID_JOBS is not None:
            original, remaining = filter_json_by_jobs(filtered, VALID_JOBS)
            removed = original - remaining
            print('Filtered %s: %d -> %d jobs (%d stale jobs removed)' % (
                config['metric'], original, remaining, removed), file=sys.stderr)

        self.copy(filtered, os.path.join(config['metric'], filtered))
        self.copy(filtered, latest)

    def run_metric(self, config):
        """Runs query and filters results, uploading data to GCS."""
        raw = 'raw-%s.json' % time.strftime('%Y-%m-%d')

        self.update_query(config)
        self.do_query(config['query'], raw)
        self.copy(raw, os.path.join(config['metric'], raw))

        consumer_error = False
        for consumer in [self.jq_upload]:
            try:
                consumer(config, raw)
            except (
                    ValueError,
                    KeyError,
                    IOError,
                    requests.exceptions.ConnectionError,
                ):
                print(traceback.format_exc(), file=sys.stderr)
                consumer_error = True
        if consumer_error:
            raise ValueError('Error(s) were thrown by query result consumers.')

    def copy(self, src, dest):
        """Use gsutil to copy src to <bucket_path>/dest with minimal caching."""
        if not self.prefix:
            return  # no destination
        dest = os.path.join(self.prefix, dest)
        check(['gsutil', '-h', 'Cache-Control:max-age=60', 'cp', src, dest])

    @staticmethod
    def update_query(config):
        """Modifies config['query'] based on the metric configuration."""
        last_time = int(time.time() - (60*60*24)*BACKFILL_DAYS)
        config['query'] = config['query'].replace('<LAST_DATA_TIME>', str(last_time))


def all_configs(search='**.yaml'):
    """Returns config files in the metrics dir."""
    return glob.glob(os.path.join(
        os.path.dirname(__file__), 'configs', search))


def ints_to_floats(point):
    for key, val in point.items():
        if key == 'time':
            continue
        if isinstance(val, int):
            point[key] = float(val)
        elif isinstance(val, dict):
            point[key] = ints_to_floats(val)
    return point


def main(configs, project, bucket_path):
    """Loads metric config files and runs each metric."""
    queryer = BigQuerier(project, bucket_path)

    # authenticate as the given service account if our environment is providing one
    if 'GOOGLE_APPLICATION_CREDENTIALS' in os.environ:
        keyfile = os.environ['GOOGLE_APPLICATION_CREDENTIALS']
        check(['gcloud', 'auth', 'activate-service-account', f'--key-file={keyfile}'])

    # the 'bq show' command is called as a hack to dodge the config prompts that bq presents
    # the first time it is run. A newline is passed to stdin to skip the prompt for default project
    # when the service account in use has access to multiple projects.
    check(['bq', 'show'], stdin='\n')

    errs = []
    for path in configs or all_configs():
        try:
            with open(path) as config_raw:
                config = yaml.safe_load(config_raw)
            if not config:
                raise ValueError('invalid yaml: %s.' % path)
            config['metric'] = config['metric'].strip()
            validate_metric_name(config['metric'])
            queryer.run_metric(config)
        except (
                ValueError,
                KeyError,
                IOError,
                subprocess.CalledProcessError,
            ):
            print(traceback.format_exc(), file=sys.stderr)
            errs.append(path)

    if errs:
        print('Failed %d configs: %s' % (len(errs), ', '.join(errs)))
        sys.exit(1)


if __name__ == '__main__':
    PARSER = argparse.ArgumentParser()
    PARSER.add_argument(
        '--config', action='append', help='YAML file describing a metric.')
    PARSER.add_argument(
        '--project',
        default='kubernetes-public',
        help='Charge the specified account for bigquery usage.')
    PARSER.add_argument(
        '--bucket',
        help='Upload results to the specified gcs bucket.')
    PARSER.add_argument(
        '--jq',
        help='path to jq binary')
    PARSER.add_argument(
        '--jobs-dir',
        help='Path to Prow job config directory (e.g., config/jobs). '
             'Used with --filter-stale-jobs to determine valid job names.')
    PARSER.add_argument(
        '--filter-stale-jobs',
        action='store_true',
        help='Filter out jobs that no longer exist in the job configs. '
             'Requires --jobs-dir to be specified.')

    ARGS = PARSER.parse_args()
    if ARGS.jq:
        DEFAULT_JQ_BIN = ARGS.jq

    # Initialize job filtering if requested
    if ARGS.filter_stale_jobs:
        if not ARGS.jobs_dir:
            print('Error: --filter-stale-jobs requires --jobs-dir', file=sys.stderr)
            sys.exit(1)
        VALID_JOBS = collect_valid_jobs(ARGS.jobs_dir)
        if not VALID_JOBS:
            print('Warning: no valid jobs found, filtering will remove all jobs',
                  file=sys.stderr)

    main(ARGS.config, ARGS.project, ARGS.bucket)
