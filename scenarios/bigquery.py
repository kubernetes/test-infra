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

"""Runs a bigquery query, filters the results, and uploads the filtered and complete data to GCS."""

import argparse
import subprocess
import sys
import time
import yaml

DEFAULT_BUCKET = 'gs://k8s-metrics'
DEFAULT_PROJECT = 'k8s-gubernator'

def check(*cmd, **keyargs):
    """Logs and runs the command, raising on errors."""
    print >>sys.stderr, 'Run:', cmd
    # If 'stdin' keyword arg is a string run command and communicate string to stdin
    if 'stdin' in keyargs and isinstance(keyargs['stdin'], str):
        in_string = keyargs['stdin']
        keyargs['stdin'] = subprocess.PIPE
        proc = subprocess.Popen(*cmd, **keyargs)
        proc.communicate(input=in_string)
        return
    subprocess.check_call(*cmd, **keyargs)

def validate_metric_name(name):
    """Validates that a string is useable as a metric name (Can be used as GCS object name)."""
    if len(name) == 0:
        raise ValueError('metric-name cannot be an empty string.')
    if any(char in name for char in '\r\n[]*?#/\\'):
        raise ValueError('metric-name cannot contain any line feeds, carriage returns, or the '
                         'following characters: []*?#/\\')

def do_query(query, out_filename, project):
    """Executes a bigquery query, outputting the results to a file."""
    with open(out_filename, 'w') as out_file:
        check(['bq', 'query', '--format=prettyjson', '--project_id='+project], stdin=query,
              stdout=out_file)

def do_jq(jqfilter, data_filename, out_filename, jq_bin='jq'):
    """Executes a jq command on a data file and outputs the results to a file."""
    with open(out_filename, 'w') as out_file:
        check([jq_bin, jqfilter, data_filename], stdout=out_file)

def run_metric(project, bucket_path, metric, query, jqfilter):
    """Executes a query, filters the results, and uploads filtered and complete results to GCS."""
    fulldata_filename = metric + time.strftime('-%Y-%m-%d.json')
    latest_filename = metric + '-latest.json'

    do_query(query, fulldata_filename, project)
    do_jq(jqfilter, fulldata_filename, latest_filename)
    check(['gsutil', '-h', 'Cache-Control:private, max-age=0, no-transform', 'cp',
           fulldata_filename, latest_filename, bucket_path + metric + '/'])

def main(configs, project, bucket_path):
    """Loads metric config files and runs each metric."""
    if not project:
        raise ValueError('project-id cannot be an empty string.')
    if not bucket_path:
        raise ValueError('bucket cannot be an empty string.')
    if bucket_path[-1] != '/':
        bucket_path += '/'

    # the 'bq show' command is called as a hack to dodge the config prompts that bq presents
    # the first time it is run. A newline is passed to stdin to skip the prompt for default project
    # when the service account in use has access to multiple projects.
    check(['bq', 'show'], stdin='\n')

    errs = []
    for config_raw in configs:
        try:
            config = yaml.load(config_raw)
            if not config:
                raise ValueError('{} does not contain a valid yaml document.'
                                 ''.format(config_raw.name))

            validate_metric_name(config['metric'])
            run_metric(project, bucket_path, config['metric'], config['query'], config['jqfilter'])
        except Exception as exception: # pylint: disable=broad-except
            errs += exception
        finally:
            config_raw.close()

    if len(errs) > 0:
        raise Exception(*errs)


if __name__ == '__main__':
    PARSER = argparse.ArgumentParser(description=
                                     'Run BigQuery query and output filtered JSON to file.')
    PARSER.add_argument('--config',
                        required=True,
                        action='append',
                        type=argparse.FileType('r'),
                        help='A JSON config file describing a metric. This option may be listed '
                        'multiple times to run multiple metrics.')
    PARSER.add_argument('--project-id',
                        required=False,
                        type=str,
                        default=DEFAULT_PROJECT,
                        help='The GCS project id to use for the bigquery query.')
    PARSER.add_argument('--bucket',
                        required=False,
                        type=str,
                        default=DEFAULT_BUCKET,
                        help='The GCS bucket path to upload output to. Files will be uploaded to a'
                        ' subdir rooted at this path and named with the metric name. Defaults to "'
                        + DEFAULT_BUCKET + '".')

    ARGS = PARSER.parse_args()
    main(ARGS.config, ARGS.project_id, ARGS.bucket)
