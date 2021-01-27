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
import os
import pipes
import re
import subprocess
import sys
import time
import traceback

import requests
import ruamel.yaml as yaml

BACKFILL_DAYS = 30

def check(cmd, **kwargs):
    """Logs and runs the command, raising on errors."""
    print('Run:', ' '.join(pipes.quote(c) for c in cmd), end=' ', file=sys.stderr)
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


def do_jq(jq_filter, data_filename, out_filename, jq_bin='jq'):
    """Executes jq on a file and outputs the results to a file."""
    with open(out_filename, 'w') as out_file:
        check([jq_bin, jq_filter, data_filename], stdout=out_file)


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
        default='k8s-gubernator',
        help='Charge the specified account for bigquery usage.')
    PARSER.add_argument(
        '--bucket',
        help='Upload results to the specified gcs bucket.')

    ARGS = PARSER.parse_args()
    main(ARGS.config, ARGS.project, ARGS.bucket)
